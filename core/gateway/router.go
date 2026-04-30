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
	"strings"
	"sync"
	"time"
)

// ModelBackend represents a deployed model and its backend service
type ModelBackend struct {
	Name   string // e.g., "mistral-7b"
	URL    string // e.g., "http://localhost:8000"
	Port   int    // e.g., 8000
	Type   string // "cpu" or "gpu"
	Status string // "running", "stopped", "error"
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

// managementModelsResponse is the JSON response from management service
type managementModelsResponse struct {
	Models []struct {
		ModelName   string `json:"model_name"`
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
	modelsURL := strings.TrimSuffix(r.managementURL, "/") + "/api/models/running"

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

	// Update registry
	r.mu.Lock()
	defer r.mu.Unlock()

	newRegistry := make(map[string]*ModelBackend)
	for _, m := range response.Models {
		if m.Status == "running" {
			backend := &ModelBackend{
				Name:   m.ModelName,
				Port:   m.Port,
				URL:    fmt.Sprintf("http://localhost:%d", m.Port),
				Type:   m.Type,
				Status: m.Status,
			}
			newRegistry[m.ModelName] = backend
		}
	}

	r.registry = newRegistry
	return nil
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
