// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package policy

import (
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gayanclife/sovereignstack/core/keys"
)

func newTestStoreF1(t *testing.T) *keys.KeyStore {
	t.Helper()
	dir := t.TempDir()
	ks, err := keys.LoadKeyStore(filepath.Join(dir, "keys.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ks.AddUser(&keys.UserProfile{ID: "alice", Key: "sk_alice"}); err != nil {
		t.Fatal(err)
	}
	return ks
}

func TestOIDC_Disabled_LoginReturns503(t *testing.T) {
	svc := New(newTestStoreF1(t), "")
	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when OIDC disabled, got %d", w.Code)
	}
}

func TestOIDC_Disabled_CallbackReturns503(t *testing.T) {
	svc := New(newTestStoreF1(t), "")
	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when OIDC disabled, got %d", w.Code)
	}
}

func TestSessionSign_RoundTrip(t *testing.T) {
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)

	signed := signSession("session-id-xyz", secret)
	sid, ok := verifySession(signed, secret)
	if !ok || sid != "session-id-xyz" {
		t.Errorf("round-trip failed: ok=%v sid=%s", ok, sid)
	}
}

func TestSessionSign_RejectsTampered(t *testing.T) {
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)

	signed := signSession("alice", secret)
	tampered := signed[:len(signed)-3] + "AAA"
	if _, ok := verifySession(tampered, secret); ok {
		t.Error("tampered cookie should not verify")
	}
}

func TestSessionSign_RejectsWrongSecret(t *testing.T) {
	a, b := make([]byte, 32), make([]byte, 32)
	_, _ = rand.Read(a)
	_, _ = rand.Read(b)

	signed := signSession("alice", a)
	if _, ok := verifySession(signed, b); ok {
		t.Error("cookie signed with secret A should not verify under secret B")
	}
}

func TestSessionSign_RejectsMalformed(t *testing.T) {
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)

	for _, in := range []string{"", "no-dot", "."} {
		if _, ok := verifySession(in, secret); ok {
			t.Errorf("malformed input %q should be rejected", in)
		}
	}
}

// CheckAdminAuth is the integration point. When OIDC is configured but
// the request has no session cookie, AdminKey should still gate access.
func TestCheckAdminAuth_BearerStillWorksWhenOIDCConfigured(t *testing.T) {
	svc := New(newTestStoreF1(t), "secret-admin")
	// Stub OIDC presence by attaching a non-nil OIDC with no sessions.
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	svc.oidc = &OIDC{
		cfg:      OIDCConfig{SessionTTL: time.Hour, SessionSecret: secret, AdminClaim: "role"},
		sessions: map[string]oidcSession{},
		states:   map[string]time.Time{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer secret-admin")
	if !svc.checkAdminAuth(req) {
		t.Error("Bearer admin auth should still pass when OIDC is also configured")
	}
}

func TestCheckAdminAuth_SessionGrantsAdmin(t *testing.T) {
	svc := New(newTestStoreF1(t), "")
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	svc.oidc = &OIDC{
		cfg:      OIDCConfig{SessionTTL: time.Hour, SessionSecret: secret, AdminClaim: "role"},
		sessions: map[string]oidcSession{
			"sid-admin": {subject: "alice@oidc", role: keys.RoleAdmin, expires: time.Now().Add(time.Hour)},
		},
		states: map[string]time.Time{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.AddCookie(&http.Cookie{Name: "sovstack_session", Value: signSession("sid-admin", secret)})
	if !svc.checkAdminAuth(req) {
		t.Error("session with admin role should pass checkAdminAuth")
	}
}

func TestCheckAdminAuth_NonAdminSessionRejected(t *testing.T) {
	svc := New(newTestStoreF1(t), "")
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	svc.oidc = &OIDC{
		cfg:      OIDCConfig{SessionTTL: time.Hour, SessionSecret: secret, AdminClaim: "role"},
		sessions: map[string]oidcSession{
			"sid-user": {subject: "bob@oidc", role: keys.RoleUser, expires: time.Now().Add(time.Hour)},
		},
		states: map[string]time.Time{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.AddCookie(&http.Cookie{Name: "sovstack_session", Value: signSession("sid-user", secret)})
	if svc.checkAdminAuth(req) {
		t.Error("non-admin session should NOT pass checkAdminAuth")
	}
}

func TestCheckAdminAuth_ExpiredSessionRejected(t *testing.T) {
	svc := New(newTestStoreF1(t), "")
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	svc.oidc = &OIDC{
		cfg:      OIDCConfig{SessionTTL: time.Hour, SessionSecret: secret, AdminClaim: "role"},
		sessions: map[string]oidcSession{
			"sid-expired": {subject: "alice", role: keys.RoleAdmin, expires: time.Now().Add(-time.Hour)},
		},
		states: map[string]time.Time{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.AddCookie(&http.Cookie{Name: "sovstack_session", Value: signSession("sid-expired", secret)})
	if svc.checkAdminAuth(req) {
		t.Error("expired session should NOT pass checkAdminAuth")
	}
}
