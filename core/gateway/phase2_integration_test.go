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
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gayanclife/sovereignstack/core/audit"
	"github.com/gayanclife/sovereignstack/core/keys"
)

// Phase 2 Integration Test: Full request flow with access control

func TestPhase2_FullRequestFlow_AllowedAccess(t *testing.T) {
	// Setup: Create KeyStore with user
	tmpFile := filepath.Join(t.TempDir(), "keys.json")
	ks, _ := keys.LoadKeyStore(tmpFile)

	alice := &keys.UserProfile{
		ID:            "alice",
		Key:           "sk_alice_123",
		AllowedModels: []string{"mistral-7b"},
		CreatedAt:     time.Now(),
	}
	ks.AddUser(alice)

	// Setup: Create gateway with access control
	authProvider := &mockAuthProvider{
		store: ks,
	}
	accessController := NewKeyStoreAccessController(ks)
	auditLogger := audit.NewLogger(100)

	gw, err := NewGateway(GatewayConfig{
		TargetURL:        "http://localhost:8000",
		AuthProvider:     authProvider,
		AccessController: accessController,
		AuditLogger:      auditLogger,
		RequestsPerMin:   100,
		APIKeyHeader:     "X-API-Key",
	})
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}

	// Test: Request with allowed model (model in URL path for extraction)
	req := httptest.NewRequest("POST", "/v1/models/mistral-7b/chat/completions", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-API-Key", "sk_alice_123")

	w := httptest.NewRecorder()
	gw.ServeHTTP(w, req)

	// The request should be allowed to proceed (won't reach backend, but won't be denied by access control)
	// Status will be non-403 (either forwarded and fails with 502, or something else)
	if w.Code == http.StatusForbidden {
		t.Errorf("Expected request to proceed, got 403 Forbidden")
	}
}

func TestPhase2_FullRequestFlow_DeniedAccess(t *testing.T) {
	// Setup: Create KeyStore with user
	tmpFile := filepath.Join(t.TempDir(), "keys.json")
	ks, _ := keys.LoadKeyStore(tmpFile)

	alice := &keys.UserProfile{
		ID:            "alice",
		Key:           "sk_alice_123",
		AllowedModels: []string{"mistral-7b"}, // Only mistral-7b
		CreatedAt:     time.Now(),
	}
	ks.AddUser(alice)

	// Setup: Create gateway with access control
	authProvider := &mockAuthProvider{store: ks}
	accessController := NewKeyStoreAccessController(ks)
	auditLogger := audit.NewLogger(100)

	gw, err := NewGateway(GatewayConfig{
		TargetURL:        "http://localhost:8000",
		AuthProvider:     authProvider,
		AccessController: accessController,
		AuditLogger:      auditLogger,
		RequestsPerMin:   100,
		APIKeyHeader:     "X-API-Key",
	})
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}

	// Test: Request with denied model (model in URL path for extraction)
	req := httptest.NewRequest("POST", "/v1/models/proprietary-model/chat/completions", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-API-Key", "sk_alice_123")

	w := httptest.NewRecorder()
	gw.ServeHTTP(w, req)

	// Should get 403 Forbidden
	if w.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden, got %d", w.Code)
	}

	// Check response body contains error message
	body, _ := io.ReadAll(w.Body)
	if !bytes.Contains(body, []byte("access denied")) {
		t.Errorf("Expected 'access denied' in response, got: %s", string(body))
	}
}

func TestPhase2_AdminWildcardAccess(t *testing.T) {
	// Setup: Create KeyStore with admin user
	tmpFile := filepath.Join(t.TempDir(), "keys.json")
	ks, _ := keys.LoadKeyStore(tmpFile)

	admin := &keys.UserProfile{
		ID:            "admin",
		Key:           "sk_admin_456",
		AllowedModels: []string{"*"}, // Wildcard
		CreatedAt:     time.Now(),
	}
	ks.AddUser(admin)

	// Setup: Create access controller
	accessController := NewKeyStoreAccessController(ks)

	// Test: Admin can access any model
	testModels := []string{"mistral-7b", "proprietary-model", "custom-model", "unknown"}
	for _, model := range testModels {
		if !accessController.CanAccess("admin", model) {
			t.Errorf("Admin should have access to %s (wildcard)", model)
		}
	}
}

func TestPhase2_RequestWithoutAccessControl(t *testing.T) {
	// Setup: Create gateway WITHOUT access controller (backward compatibility)
	authProvider := &mockAuthProvider{store: nil}
	auditLogger := audit.NewLogger(100)

	gw, err := NewGateway(GatewayConfig{
		TargetURL:        "http://localhost:8000",
		AuthProvider:     authProvider,
		AccessController: nil, // No access control
		AuditLogger:      auditLogger,
		RequestsPerMin:   100,
		APIKeyHeader:     "X-API-Key",
	})
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}

	// Test: Any model should be allowed (no access controller to deny)
	req := httptest.NewRequest("POST", "/v1/models/anything/chat/completions", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-API-Key", "sk_any_key")

	w := httptest.NewRecorder()
	gw.ServeHTTP(w, req)

	// Should NOT be 403 (access control not enabled)
	if w.Code == http.StatusForbidden {
		t.Errorf("Expected no 403 when access control is disabled")
	}
}

// Mock AuthProvider for testing
type mockAuthProvider struct {
	store *keys.KeyStore
}

func (p *mockAuthProvider) ValidateToken(token string) (string, error) {
	if p.store == nil {
		// For backward compat test, accept any token
		return "test-user", nil
	}

	user, _ := p.store.GetByKey(token)
	if user == nil {
		return "", ErrInvalidToken
	}
	return user.ID, nil
}

var ErrInvalidToken = &tokenError{message: "invalid token"}

type tokenError struct {
	message string
}

func (e *tokenError) Error() string {
	return e.message
}

// Benchmark: Access control check performance
func BenchmarkAccessControl(b *testing.B) {
	// Setup
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}

	// Create user with 10 allowed models
	models := make([]string, 10)
	for i := 0; i < 10; i++ {
		models[i] = "model-" + string(rune(65+i)) // model-A, model-B, etc
	}

	profile := &keys.UserProfile{
		ID:            "testuser",
		Key:           "sk_test",
		AllowedModels: models,
		CreatedAt:     time.Now(),
	}
	ks.Users["testuser"] = profile

	ac := NewKeyStoreAccessController(ks)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ac.CanAccess("testuser", "model-E")
	}
}
