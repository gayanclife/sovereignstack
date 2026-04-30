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

package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleModelEndpoints_InvalidPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/models/", nil)
	w := httptest.NewRecorder()

	handleModelEndpoints(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid path, got %d", w.Code)
	}
}

func TestHandleModelEndpoints_UnknownEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/models/mistral-7b/unknown", nil)
	w := httptest.NewRecorder()

	handleModelEndpoints(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown endpoint, got %d", w.Code)
	}
}

func TestHandleModelMetrics_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/models/mistral-7b/metrics", nil)
	w := httptest.NewRecorder()

	handleModelMetrics(w, req, "mistral-7b")

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST method, got %d", w.Code)
	}
}

// TestHandleModelMetrics_ModelNotRunning tests behavior when model is not in
// the running list. This requires Docker to be available; if Docker isn't
// reachable, the test should still get a 5xx (not a 404), so we accept either.
func TestHandleModelMetrics_ModelNotRunning(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/models/nonexistent-model-xyz-12345/metrics", nil)
	w := httptest.NewRecorder()

	handleModelMetrics(w, req, "nonexistent-model-xyz-12345")

	// Expect 404 (model not found in running list) or 500 (Docker unavailable)
	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("expected 404 or 500, got %d", w.Code)
	}
}
