// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package policy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gayanclife/sovereignstack/core/keys"
)

func newTestStore(t *testing.T) *keys.KeyStore {
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

func TestPolicy_ListUsers_RequiresAdmin(t *testing.T) {
	svc := New(newTestStore(t), "secret-admin")
	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without admin header, got %d", w.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req2.Header.Set("Authorization", "Bearer secret-admin")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200 with valid admin, got %d", w2.Code)
	}
}

func TestPolicy_ReadProfile_PublicNoAuth(t *testing.T) {
	svc := New(newTestStore(t), "secret-admin")
	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/alice", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("read profile: %d", w.Code)
	}
	var prof map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &prof)
	if prof["id"] != "alice" {
		t.Errorf("body: %v", prof)
	}
}

func TestPolicy_GrantModel(t *testing.T) {
	store := newTestStore(t)
	svc := New(store, "secret-admin")
	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/alice/models/mistral-7b", nil)
	req.Header.Set("Authorization", "Bearer secret-admin")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("grant: %d %s", w.Code, w.Body.String())
	}
	user, _ := store.GetByID("alice")
	if !contains(user.AllowedModels, "mistral-7b") {
		t.Errorf("alice should now have mistral-7b: %v", user.AllowedModels)
	}
}

func TestPolicy_RevokeModel(t *testing.T) {
	store := newTestStore(t)
	_ = store.GrantModelAccess("alice", "mistral-7b")

	svc := New(store, "secret-admin")
	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/alice/models/mistral-7b", nil)
	req.Header.Set("Authorization", "Bearer secret-admin")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("revoke: %d", w.Code)
	}
	user, _ := store.GetByID("alice")
	if contains(user.AllowedModels, "mistral-7b") {
		t.Errorf("alice should not have mistral-7b after revoke: %v", user.AllowedModels)
	}
}

func TestPolicy_SetQuota(t *testing.T) {
	store := newTestStore(t)
	svc := New(store, "secret-admin")
	mux := http.NewServeMux()
	svc.Register(mux)

	body := strings.NewReader(`{"max_tokens_per_day":500000,"max_tokens_per_month":10000000}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/alice/quota", body)
	req.Header.Set("Authorization", "Bearer secret-admin")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("set quota: %d %s", w.Code, w.Body.String())
	}
	user, _ := store.GetByID("alice")
	if user.MaxTokensPerDay != 500000 {
		t.Errorf("daily quota not applied: %d", user.MaxTokensPerDay)
	}
}

func TestPolicy_AccessCheck_Allowed(t *testing.T) {
	store := newTestStore(t)
	_ = store.GrantModelAccess("alice", "mistral-7b")

	svc := New(store, "")
	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/access/check?user=alice&model=mistral-7b", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for allowed model, got %d %s", w.Code, w.Body.String())
	}
}

func TestPolicy_AccessCheck_Denied(t *testing.T) {
	svc := New(newTestStore(t), "")
	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/access/check?user=alice&model=forbidden", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for denied model, got %d", w.Code)
	}
}

func TestPolicy_NoAdminKey_AdminAuthBypassed(t *testing.T) {
	svc := New(newTestStore(t), "") // empty admin key = bypass for dev
	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected dev-mode bypass to permit access, got %d", w.Code)
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
