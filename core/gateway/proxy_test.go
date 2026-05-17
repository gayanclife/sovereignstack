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

// TestProxy_CapabilityCheck_AllowsModelsIntrospection confirms that
// GET /models/{name}/v1/models is always passed through regardless of
// capability, so clients can discover what a model supports before
// sending an inference request.
func TestProxy_CapabilityCheck_AllowsModelsIntrospection(t *testing.T) {
	gw := newTestGatewayWithRouter(t, map[string]*ModelBackend{
		"distilbert": {
			Name:       "distilbert",
			URL:        "http://127.0.0.1:9",
			Capability: "encoder",
			Endpoints:  []string{"/v1/embeddings"},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/models/distilbert/v1/models", nil)
	req.Header.Set("X-API-Key", "sk_test")
	w := httptest.NewRecorder()
	gw.ServeHTTP(w, req)

	// Must NOT be blocked by the capability gate.
	if w.Code == http.StatusNotFound &&
		strings.Contains(w.Body.String(), "endpoint_not_supported_for_model") {
		t.Errorf("/v1/models introspection blocked by capability gate: %s", w.Body.String())
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

// TestProxy_V1ModelsPrefixStripped verifies that requests using the
// /v1/models/{name}/... path format are routed to the correct backend
// and that the /models/{name} segment is stripped before forwarding,
// so the backend receives /v1/chat/completions (not the full original path).
func TestProxy_V1ModelsPrefixStripped(t *testing.T) {
	var receivedPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	auth := NewAPIKeyAuthProvider()
	auth.AddKey("sk_test", "test-user")

	router := NewModelRouter("http://management-not-used")
	router.mu.Lock()
	router.registry["mistral-7b"] = &ModelBackend{
		Name:       "mistral-7b",
		URL:        backend.URL,
		Capability: "generative",
		Endpoints:  []string{"/v1/chat/completions"},
	}
	router.mu.Unlock()

	gw, err := NewGateway(GatewayConfig{
		TargetURL:    backend.URL,
		AuthProvider: auth,
		ModelRouter:  router,
		AuditLogger:  audit.NewLogger(10),
		APIKeyHeader: "X-API-Key",
	})
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost,
		"/v1/models/mistral-7b/chat/completions",
		bytes.NewReader([]byte(`{"messages":[]}`)))
	req.Header.Set("X-API-Key", "sk_test")
	w := httptest.NewRecorder()
	gw.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	if receivedPath != "/v1/chat/completions" {
		t.Errorf("backend received path %q, want /v1/chat/completions", receivedPath)
	}
}

// TestProxy_UnknownModel_PathStrippedBeforeDefaultBackend verifies that when
// a /models/{name}/... request arrives and the named model is absent from the
// registry (race: removed between ServeHTTP check and director), the director
// still strips the routing prefix so the default backend receives
// /v1/chat/completions rather than the full unrecognised path.
func TestProxy_UnknownModel_PathStrippedBeforeDefaultBackend(t *testing.T) {
	// Empty registry — model not deployed.
	auth := NewAPIKeyAuthProvider()
	auth.AddKey("sk_test", "test-user")
	router := NewModelRouter("http://management-not-used")

	gw, err := NewGateway(GatewayConfig{
		TargetURL:    "http://127.0.0.1:0",
		AuthProvider: auth,
		ModelRouter:  router,
		AuditLogger:  audit.NewLogger(10),
		APIKeyHeader: "X-API-Key",
	})
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}

	// Call director directly, simulating the race where the model was evicted
	// after the ServeHTTP capability gate passed.
	req, _ := http.NewRequest(http.MethodPost,
		"/models/microsoft-Phi/v1/chat/completions",
		bytes.NewReader([]byte(`{}`)))
	gw.director(req)

	if req.URL.Path != "/v1/chat/completions" {
		t.Errorf("director left path unstripped: got %q, want /v1/chat/completions", req.URL.Path)
	}
}

// TestProxy_ModelNotDeployed_Returns404 confirms that a /models/{name}/...
// request where the named model is absent from the router registry is
// rejected with a clear 404 rather than forwarded (with the unstripped
// path) to the default backend.
func TestProxy_ModelNotDeployed_Returns404(t *testing.T) {
	gw := newTestGatewayWithRouter(t, map[string]*ModelBackend{
		"other-model": {
			Name:       "other-model",
			URL:        "http://127.0.0.1:9",
			Capability: "generative",
			Endpoints:  []string{"/v1/chat/completions"},
		},
	})

	req := httptest.NewRequest(http.MethodPost,
		"/models/mistral-7b/v1/chat/completions",
		bytes.NewReader([]byte(`{"messages":[]}`)))
	req.Header.Set("X-API-Key", "sk_test")
	w := httptest.NewRecorder()
	gw.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (body: %s)", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`"model_not_deployed"`, `"mistral-7b"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\nfull body: %s", want, body)
		}
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

func TestParseTokenUsage_JSON(t *testing.T) {
	body := []byte(`{"id":"chatcmpl-1","choices":[{"message":{"content":"hi"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
	in, out := parseTokenUsage(body, "application/json")
	if in != 10 || out != 5 {
		t.Errorf("expected (10,5) got (%d,%d)", in, out)
	}
}

func TestParseTokenUsage_SSE(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"he\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"llo\"}}],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":3}}\n\ndata: [DONE]\n\n")
	in, out := parseTokenUsage(body, "text/event-stream")
	if in != 8 || out != 3 {
		t.Errorf("expected (8,3) got (%d,%d)", in, out)
	}
}

func TestParseTokenUsage_NoUsage(t *testing.T) {
	body := []byte(`{"id":"chatcmpl-1","choices":[]}`)
	in, out := parseTokenUsage(body, "application/json")
	if in != 0 || out != 0 {
		t.Errorf("expected (0,0) got (%d,%d)", in, out)
	}
}
