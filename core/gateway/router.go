// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// ModelBackend represents a deployed model and its backend service.
// Capability and Endpoints are best-effort: populated by querying the
// model's own /v1/models endpoint at refresh time. When empty, the
// gateway forwards every request to the backend as-is (legacy behavior),
// so older model servers that don't expose capability info keep working.
type ModelBackend struct {
	Name       string   // e.g., "mistral-7b"
	URL        string   // e.g., "http://localhost:8000"
	Port       int      // e.g., 8000
	Type       string   // "cpu" or "gpu"
	Status     string   // "running", "stopped", "error"
	Capability string   // e.g., "generative", "encoder", "classification" (empty if unknown)
	Endpoints  []string // e.g., ["/v1/chat/completions"] — what the model can serve (nil if unknown)
}

// ModelRouter discovers and maintains a registry of available models
type ModelRouter struct {
	mu              sync.RWMutex
	registry        map[string]*ModelBackend // modelName → backend
	managementURL   string
	httpClient      *http.Client
	refreshInterval time.Duration
	done            chan struct{}
	stopOnce        sync.Once
}

// managementModelsResponse is the JSON response from the management service's
// /api/v1/models/running endpoint. The model name comes back as "name"
// (NOT "model_name" — that older tag silently produced an empty registry,
// which made every model-routed request fall through to the default
// backend with a 404).
type managementModelsResponse struct {
	Models []struct {
		ModelName   string `json:"name"`
		ContainerID string `json:"container_id"`
		Type        string `json:"type"`
		Status      string `json:"status"`
		Port        int    `json:"port"`
	} `json:"models"`
}

// NewModelRouter creates a new model router
func NewModelRouter(managementURL string) *ModelRouter {
	return &ModelRouter{
		registry:        make(map[string]*ModelBackend),
		managementURL:   managementURL,
		httpClient:      &http.Client{Timeout: 5 * time.Second},
		refreshInterval: 30 * time.Second,
		done:            make(chan struct{}),
	}
}

// StartDiscovery begins the background model discovery process
func (r *ModelRouter) StartDiscovery() {
	// Perform initial discovery
	_ = r.refresh()

	// Start periodic refresh
	go func() {
		ticker := time.NewTicker(r.refreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-r.done:
				return
			case <-ticker.C:
				_ = r.refresh()
			}
		}
	}()
}

// Stop terminates the discovery process
func (r *ModelRouter) Stop() {
	r.stopOnce.Do(func() {
		close(r.done)
	})
}

// refresh queries the management service for current models
func (r *ModelRouter) refresh() error {
	// Construct URL to management service
	modelsURL := strings.TrimSuffix(r.managementURL, "/") + "/api/v1/models/running"

	resp, err := r.httpClient.Get(modelsURL)
	if err != nil {
		// Log error but don't fail - keep existing registry
		return fmt.Errorf("failed to fetch models from %s: %w", modelsURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("management service returned %d", resp.StatusCode)
	}

	var response managementModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to decode models response: %w", err)
	}

	// Build the new registry first WITHOUT the lock, since fetching each
	// backend's /v1/models can take up to 2 seconds per model and we don't
	// want to block reads on the registry that long. The lock is held only
	// during the swap at the end.
	newRegistry := make(map[string]*ModelBackend)
	for _, m := range response.Models {
		if m.Status != "running" {
			continue
		}
		backend := &ModelBackend{
			Name:   m.ModelName,
			Port:   m.Port,
			URL:    fmt.Sprintf("http://localhost:%d", m.Port),
			Type:   m.Type,
			Status: m.Status,
		}
		// Best-effort: enrich with capability + endpoint list from the
		// model's own /v1/models. Failure leaves Capability/Endpoints
		// empty, and the proxy will forward every request to that
		// backend without the friendly-error check (preserves the
		// behavior expected by older model servers).
		r.fetchCapability(backend)
		newRegistry[m.ModelName] = backend
	}

	r.mu.Lock()
	r.registry = newRegistry
	r.mu.Unlock()
	return nil
}

// modelCapabilityResponse mirrors the subset of /v1/models we read.
// The model server (see internal/docker/cpu_inference.go) reports its
// detected capability and the OpenAI-compat endpoints it actually
// registered for that capability.
type modelCapabilityResponse struct {
	Data []struct {
		Capability string   `json:"capability"`
		Endpoints  []string `json:"endpoints"`
	} `json:"data"`
}

// fetchCapability populates backend.Capability and backend.Endpoints
// from <backend.URL>/v1/models. Best-effort with a 2s timeout — any
// failure (network, non-200, malformed JSON, missing fields) leaves
// the fields empty so the proxy treats the model as legacy and skips
// the friendly-error check.
func (r *ModelRouter) fetchCapability(backend *ModelBackend) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(backend.URL + "/v1/models")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var body modelCapabilityResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return
	}
	if len(body.Data) == 0 {
		return
	}
	backend.Capability = body.Data[0].Capability
	backend.Endpoints = append([]string(nil), body.Data[0].Endpoints...)
}

// IsPathSupported answers the friendly-error check for a request that
// was already routed to backend (backend != nil). Returns ok=true when
// the gateway should forward the request, ok=false when the model
// definitely cannot serve this path.
//
// Conservative: when capability info is unknown (legacy model server
// that doesn't expose Capability/Endpoints), returns ok=true. The proxy
// then forwards as-is and the worst case is the model's own 404 — same
// behavior as before this method existed.
func (b *ModelBackend) IsPathSupported(strippedPath string) (ok bool) {
	if len(b.Endpoints) == 0 {
		return true
	}
	return slices.Contains(b.Endpoints, strippedPath)
}

// GetBackend returns the backend for a given model name
func (r *ModelRouter) GetBackend(modelName string) (*ModelBackend, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	backend, exists := r.registry[modelName]
	return backend, exists
}

// ListModels returns all registered models
func (r *ModelRouter) ListModels() []*ModelBackend {
	r.mu.RLock()
	defer r.mu.RUnlock()

	models := make([]*ModelBackend, 0, len(r.registry))
	for _, backend := range r.registry {
		models = append(models, backend)
	}
	return models
}

// GetModelCount returns the number of registered models
func (r *ModelRouter) GetModelCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.registry)
}
