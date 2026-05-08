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
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gayanclife/sovereignstack/core/audit"
)

// newTestGatewayWithRouter spins up a gateway wired with a hardcoded
// model router whose registry the caller pre-populates. Useful for
// tests that need to exercise the capability check without spinning a
// real management API.
func newTestGatewayWithRouter(t *testing.T, registry map[string]*ModelBackend) *Gateway {
	t.Helper()
	auth := NewAPIKeyAuthProvider()
	auth.AddKey("sk_test", "test-user")

	router := NewModelRouter("http://management-not-used")
	router.mu.Lock()
	router.registry = registry
	router.mu.Unlock()

	gw, err := NewGateway(GatewayConfig{
		TargetURL:    "http://127.0.0.1:0", // unreachable; capability check should fire first
		AuthProvider: auth,
		ModelRouter:  router,
		AuditLogger:  audit.NewLogger(10),
		APIKeyHeader: "X-API-Key",
	})
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}
	return gw
}

// TestProxy_CapabilityCheck_ShortCircuitsOnMismatch covers the friendly
// error case: caller hits /v1/chat/completions on an encoder model. The
// gateway should respond 404 with a helpful body before forwarding to
// the model container (which would otherwise return its own bare
// {"detail":"Not Found"} that doesn't tell the caller why).
func TestProxy_CapabilityCheck_ShortCircuitsOnMismatch(t *testing.T) {
	gw := newTestGatewayWithRouter(t, map[string]*ModelBackend{
		"distilbert": {
			Name:       "distilbert",
			URL:        "http://127.0.0.1:9", // unreachable; should never be called
			Capability: "encoder",
			Endpoints:  []string{"/v1/embeddings"},
		},
	})

	req := httptest.NewRequest(http.MethodPost,
		"/models/distilbert/v1/chat/completions",
		bytes.NewReader([]byte(`{"messages":[]}`)))
	req.Header.Set("X-API-Key", "sk_test")
	w := httptest.NewRecorder()
	gw.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (body: %s)", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		`"endpoint_not_supported_for_model"`,
		`"distilbert"`,
		`"encoder"`,
		`/v1/chat/completions`,
		`/v1/embeddings`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\nfull body: %s", want, body)
		}
	}
	if got := w.Header().Get("X-Supported-Endpoints"); got != "/v1/embeddings" {
		t.Errorf("X-Supported-Endpoints: got %q, want /v1/embeddings", got)
	}
}

// TestProxy_CapabilityCheck_AllowsSupportedEndpoint confirms the check
// doesn't false-positive: a request to a supported endpoint passes the
// capability gate and proceeds toward the (unreachable) backend, where
// it'll fail with 502 / similar — but NOT 404 from the capability check.
func TestProxy_CapabilityCheck_AllowsSupportedEndpoint(t *testing.T) {
	gw := newTestGatewayWithRouter(t, map[string]*ModelBackend{
		"tinyllama": {
			Name:       "tinyllama",
			URL:        "http://127.0.0.1:9",
			Capability: "generative",
			Endpoints:  []string{"/v1/chat/completions", "/v1/completions"},
		},
	})

	req := httptest.NewRequest(http.MethodPost,
		"/models/tinyllama/v1/chat/completions",
		bytes.NewReader([]byte(`{"messages":[]}`)))
	req.Header.Set("X-API-Key", "sk_test")
	w := httptest.NewRecorder()
	gw.ServeHTTP(w, req)

	// Whatever happens at the backend, we MUST NOT get the 404 with the
	// capability-mismatch body — that would mean the gate misclassified.
	if w.Code == http.StatusNotFound &&
		strings.Contains(w.Body.String(), "endpoint_not_supported_for_model") {
		t.Errorf("supported endpoint blocked by capability gate: %s", w.Body.String())
	}
}

// TestProxy_CapabilityCheck_LegacyModelPassesThrough confirms backwards
// compat: a router entry with no Capability/Endpoints info (older model
// server, or first poll hasn't completed) doesn't get blocked. The
// gateway forwards as-is and any 404 comes from the backend itself.
func TestProxy_CapabilityCheck_LegacyModelPassesThrough(t *testing.T) {
	gw := newTestGatewayWithRouter(t, map[string]*ModelBackend{
		"unknown-cap": {
			Name: "unknown-cap",
			URL:  "http://127.0.0.1:9",
			// Capability / Endpoints intentionally unset.
		},
	})

	req := httptest.NewRequest(http.MethodPost,
		"/models/unknown-cap/v1/anything",
		bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-API-Key", "sk_test")
	w := httptest.NewRecorder()
	gw.ServeHTTP(w, req)

	// The capability gate must NOT short-circuit on legacy models.
	if w.Code == http.StatusNotFound &&
		strings.Contains(w.Body.String(), "endpoint_not_supported_for_model") {
		t.Errorf("legacy model wrongly blocked by capability gate: %s",
			w.Body.String())
	}
}
