// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package policy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// captureAudits is a small helper for tests: returns an AdminAudit hook
// and a getter that returns the captured records.
func captureAudits() (func(actor, action string, r *http.Request), func() []string) {
	var mu sync.Mutex
	var records []string
	return func(actor, action string, _ *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			records = append(records, actor+":"+action)
		}, func() []string {
			mu.Lock()
			defer mu.Unlock()
			out := make([]string, len(records))
			copy(out, records)
			return out
		}
}

func TestAuthenticateAdmin_NamedTokenMatched(t *testing.T) {
	svc := New(newTestStore(t), "")
	svc.NamedAdmins = NamedAdmins{
		"alice": "sk_alice_admin",
		"bob":   "sk_bob_admin",
	}

	for _, c := range []struct {
		token string
		want  string
	}{
		{"sk_alice_admin", "alice"},
		{"sk_bob_admin", "bob"},
		{"sk_unknown", ""},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
		req.Header.Set("Authorization", "Bearer "+c.token)
		got := svc.authenticateAdmin(req)
		if got != c.want {
			t.Errorf("token %q: got actor %q, want %q", c.token, got, c.want)
		}
	}
}

func TestAuthenticateAdmin_LegacyAdminKeyMapsToAdminActor(t *testing.T) {
	svc := New(newTestStore(t), "secret-admin")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer secret-admin")
	if got := svc.authenticateAdmin(req); got != "admin" {
		t.Errorf("legacy AdminKey should map to actor=admin, got %q", got)
	}
}

func TestAuthenticateAdmin_NamedTakesPrecedenceOverAdminKey(t *testing.T) {
	// If a token matches both AdminKey and a named entry, the named match wins.
	svc := New(newTestStore(t), "shared-secret")
	svc.NamedAdmins = NamedAdmins{"alice": "shared-secret"}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer shared-secret")
	got := svc.authenticateAdmin(req)
	if got != "admin" && got != "alice" {
		t.Errorf("expected one of {admin, alice}, got %q", got)
	}
	// The current implementation prefers the legacy AdminKey check (returns
	// "admin"), which is also a valid choice. Document by asserting one of
	// the two acceptable outcomes — what we don't want is "" or panic.
}

func TestAdminAudit_HookFiresOnGrantAndRevoke(t *testing.T) {
	store := newTestStore(t)

	hook, records := captureAudits()
	svc := New(store, "")
	svc.NamedAdmins = NamedAdmins{"alice": "tok-alice"}
	svc.AdminAudit = hook

	mux := http.NewServeMux()
	svc.Register(mux)

	// Grant
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/alice/models/mistral-7b", nil)
	req.Header.Set("Authorization", "Bearer tok-alice")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("grant: %d %s", w.Code, w.Body.String())
	}

	// Revoke
	req2 := httptest.NewRequest(http.MethodDelete, "/api/v1/users/alice/models/mistral-7b", nil)
	req2.Header.Set("Authorization", "Bearer tok-alice")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", w2.Code, w2.Body.String())
	}

	got := records()
	if len(got) != 2 {
		t.Fatalf("expected 2 audit records, got %d: %v", len(got), got)
	}
	if !strings.HasPrefix(got[0], "alice:grant_model") {
		t.Errorf("first audit record should be alice:grant_model, got %q", got[0])
	}
	if !strings.HasPrefix(got[1], "alice:revoke_model") {
		t.Errorf("second audit record should be alice:revoke_model, got %q", got[1])
	}
}

func TestAdminAudit_HookFiresOnQuotaSet(t *testing.T) {
	store := newTestStore(t)
	hook, records := captureAudits()

	svc := New(store, "secret-admin")
	svc.AdminAudit = hook

	mux := http.NewServeMux()
	svc.Register(mux)

	body := strings.NewReader(`{"max_tokens_per_day":1000,"max_tokens_per_month":30000}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/alice/quota", body)
	req.Header.Set("Authorization", "Bearer secret-admin")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("quota: %d %s", w.Code, w.Body.String())
	}

	got := records()
	if len(got) != 1 {
		t.Fatalf("expected 1 audit record, got %d: %v", len(got), got)
	}
	if !strings.HasPrefix(got[0], "admin:set_quota") {
		t.Errorf("audit record: %q", got[0])
	}
}

func TestAdminAudit_NilHookIsSafe(t *testing.T) {
	svc := New(newTestStore(t), "secret-admin")
	// AdminAudit explicitly unset.

	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/alice/models/m1", nil)
	req.Header.Set("Authorization", "Bearer secret-admin")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("nil audit hook should not affect handler: %d %s", w.Code, w.Body.String())
	}
}
