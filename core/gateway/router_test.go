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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestModelRouter_NewModelRouter(t *testing.T) {
	router := NewModelRouter("http://localhost:8888")
	if router == nil {
		t.Errorf("NewModelRouter should create router")
	}
	if router.managementURL != "http://localhost:8888" {
		t.Errorf("Expected management URL http://localhost:8888, got %s", router.managementURL)
	}
}

func TestModelRouter_GetBackend_NotFound(t *testing.T) {
	router := NewModelRouter("http://localhost:8888")
	_, exists := router.GetBackend("nonexistent-model")
	if exists {
		t.Errorf("GetBackend should return false for nonexistent model")
	}
}

func TestModelRouter_GetBackend_Found(t *testing.T) {
	router := NewModelRouter("http://localhost:8888")

	// Manually add a model to registry
	backend := &ModelBackend{
		Name:   "mistral-7b",
		Port:   8000,
		URL:    "http://localhost:8000",
		Type:   "gpu",
		Status: "running",
	}
	router.mu.Lock()
	router.registry["mistral-7b"] = backend
	router.mu.Unlock()

	result, exists := router.GetBackend("mistral-7b")
	if !exists {
		t.Errorf("GetBackend should find mistral-7b")
	}
	if result.Port != 8000 {
		t.Errorf("Expected port 8000, got %d", result.Port)
	}
}

func TestModelRouter_ListModels_Empty(t *testing.T) {
	router := NewModelRouter("http://localhost:8888")
	models := router.ListModels()
	if len(models) != 0 {
		t.Errorf("Expected 0 models, got %d", len(models))
	}
}

func TestModelRouter_ListModels_Multiple(t *testing.T) {
	router := NewModelRouter("http://localhost:8888")

	router.mu.Lock()
	router.registry["mistral-7b"] = &ModelBackend{Name: "mistral-7b", Port: 8000}
	router.registry["llama-3-8b"] = &ModelBackend{Name: "llama-3-8b", Port: 8001}
	router.registry["phi-3"] = &ModelBackend{Name: "phi-3", Port: 8002}
	router.mu.Unlock()

	models := router.ListModels()
	if len(models) != 3 {
		t.Errorf("Expected 3 models, got %d", len(models))
	}
}

func TestModelRouter_GetModelCount(t *testing.T) {
	router := NewModelRouter("http://localhost:8888")

	if router.GetModelCount() != 0 {
		t.Errorf("Expected count 0, got %d", router.GetModelCount())
	}

	router.mu.Lock()
	router.registry["mistral-7b"] = &ModelBackend{Name: "mistral-7b"}
	router.registry["llama-3-8b"] = &ModelBackend{Name: "llama-3-8b"}
	router.mu.Unlock()

	if router.GetModelCount() != 2 {
		t.Errorf("Expected count 2, got %d", router.GetModelCount())
	}
}

func TestModelRouter_Refresh_Success(t *testing.T) {
	// Create mock management service
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := managementModelsResponse{
			Models: []struct {
				ModelName   string `json:"model_name"`
				ContainerID string `json:"container_id"`
				Type        string `json:"type"`
				Status      string `json:"status"`
				Port        int    `json:"port"`
			}{
				{
					ModelName:   "mistral-7b",
					ContainerID: "container1",
					Type:        "gpu",
					Status:      "running",
					Port:        8000,
				},
				{
					ModelName:   "llama-3-8b",
					ContainerID: "container2",
					Type:        "gpu",
					Status:      "running",
					Port:        8001,
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	router := NewModelRouter(server.URL)
	err := router.refresh()
	if err != nil {
		t.Errorf("Refresh failed: %v", err)
	}

	if router.GetModelCount() != 2 {
		t.Errorf("Expected 2 models after refresh, got %d", router.GetModelCount())
	}

	backend, exists := router.GetBackend("mistral-7b")
	if !exists {
		t.Errorf("mistral-7b should exist after refresh")
	}
	if backend.Port != 8000 {
		t.Errorf("Expected port 8000, got %d", backend.Port)
	}
}

func TestModelRouter_Refresh_IgnoresStopped(t *testing.T) {
	// Create mock management service that returns stopped models
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := managementModelsResponse{
			Models: []struct {
				ModelName   string `json:"model_name"`
				ContainerID string `json:"container_id"`
				Type        string `json:"type"`
				Status      string `json:"status"`
				Port        int    `json:"port"`
			}{
				{
					ModelName: "running-model",
					Status:    "running",
					Port:      8000,
				},
				{
					ModelName: "stopped-model",
					Status:    "stopped",
					Port:      8001,
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	router := NewModelRouter(server.URL)
	router.refresh()

	// Should only have running model
	if router.GetModelCount() != 1 {
		t.Errorf("Expected 1 running model, got %d", router.GetModelCount())
	}

	_, exists := router.GetBackend("running-model")
	if !exists {
		t.Errorf("running-model should exist")
	}

	_, exists = router.GetBackend("stopped-model")
	if exists {
		t.Errorf("stopped-model should not exist")
	}
}

func TestModelRouter_Refresh_ConnectionError(t *testing.T) {
	// Use invalid URL to trigger connection error
	router := NewModelRouter("http://invalid-host:9999")
	err := router.refresh()

	if err == nil {
		t.Errorf("Refresh should return error for invalid host")
	}
}

func TestModelRouter_Stop(t *testing.T) {
	router := NewModelRouter("http://localhost:8888")
	router.Stop()

	// Stop should close the done channel
	select {
	case <-router.done:
		// Success - channel is closed
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Stop should close done channel")
	}
}

func TestModelRouter_ConcurrentAccess(t *testing.T) {
	router := NewModelRouter("http://localhost:8888")

	// Add initial models
	router.mu.Lock()
	router.registry["mistral-7b"] = &ModelBackend{Name: "mistral-7b", Port: 8000}
	router.mu.Unlock()

	// Concurrent reads
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, _ = router.GetBackend("mistral-7b")
			_ = router.ListModels()
			_ = router.GetModelCount()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	if router.GetModelCount() != 1 {
		t.Errorf("Model count should still be 1 after concurrent access")
	}
}

// Helper function tests
func TestExtractModelNameFromPath_Valid(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/models/mistral-7b/v1/chat/completions", "mistral-7b"},
		{"/models/llama-3-8b/v1/embeddings", "llama-3-8b"},
		{"/models/phi-3/v1/completions", "phi-3"},
	}

	for _, test := range tests {
		result := extractModelNameFromPath(test.path)
		if result != test.expected {
			t.Errorf("extractModelNameFromPath(%s) = %s, expected %s", test.path, result, test.expected)
		}
	}
}

func TestExtractModelNameFromPath_Invalid(t *testing.T) {
	tests := []string{
		"/v1/models",
		"/v1/chat/completions",
		"/api/health",
		"/models/",
	}

	for _, path := range tests {
		result := extractModelNameFromPath(path)
		if result != "" {
			t.Errorf("extractModelNameFromPath(%s) should return empty string, got %s", path, result)
		}
	}
}

func TestStripModelPrefixFromPath(t *testing.T) {
	tests := []struct {
		path      string
		model     string
		expected  string
	}{
		{"/models/mistral-7b/v1/chat/completions", "mistral-7b", "/v1/chat/completions"},
		{"/models/llama-3-8b/v1/embeddings", "llama-3-8b", "/v1/embeddings"},
		{"/models/phi-3/v1/completions", "phi-3", "/v1/completions"},
	}

	for _, test := range tests {
		result := stripModelPrefixFromPath(test.path, test.model)
		if result != test.expected {
			t.Errorf("stripModelPrefixFromPath(%s, %s) = %s, expected %s",
				test.path, test.model, result, test.expected)
		}
	}
}

func TestStripModelPrefixFromPath_NoMatch(t *testing.T) {
	path := "/models/mistral-7b/v1/chat/completions"
	model := "llama-3-8b"

	result := stripModelPrefixFromPath(path, model)
	if result != path {
		t.Errorf("stripModelPrefixFromPath should return original path when model doesn't match, got %s", result)
	}
}

// Benchmark tests
func BenchmarkModelRouter_GetBackend(b *testing.B) {
	router := NewModelRouter("http://localhost:8888")

	// Add many models
	router.mu.Lock()
	for i := 0; i < 100; i++ {
		name := "model-" + string(rune(65+i%26))
		router.registry[name] = &ModelBackend{Name: name, Port: 8000 + i}
	}
	router.mu.Unlock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.GetBackend("model-A")
	}
}

func BenchmarkExtractModelNameFromPath(b *testing.B) {
	path := "/models/mistral-7b/v1/chat/completions"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		extractModelNameFromPath(path)
	}
}

func BenchmarkStripModelPrefixFromPath(b *testing.B) {
	path := "/models/mistral-7b/v1/chat/completions"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stripModelPrefixFromPath(path, "mistral-7b")
	}
}
