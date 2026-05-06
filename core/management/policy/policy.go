// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package policy hosts the user-policy HTTP handlers (CRUD for users,
// model access grants, quota updates, access checks). Phase E split this
// out so it can run as its own binary on a separate port (8888 by
// default), with admin auth required and exclusive write access to the
// keys.json file.
//
// Endpoints owned by this package:
//
//	GET    /api/v1/users                        (admin)
//	GET    /api/v1/users/{id}                   (no auth)
//	POST   /api/v1/users/{id}/models/{model}    (admin)
//	DELETE /api/v1/users/{id}/models/{model}    (admin)
//	PATCH  /api/v1/users/{id}/quota             (admin)
//	GET    /api/v1/access/check?user=&model=    (no auth)
package policy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gayanclife/sovereignstack/core/keys"
)

// Service bundles the policy handlers with their dependencies.
type Service struct {
	Store    *keys.KeyStore
	AdminKey string // empty means admin auth is disabled (dev only)

	// NamedAdmins holds optional Phase C4 named-actor admin tokens. Every
	// admin mutation logs which named principal performed it, giving the
	// audit trail per-human attribution rather than just "the admin key".
	// AdminKey, when set, also works (recorded as actor "admin").
	NamedAdmins NamedAdmins

	// AdminAudit, if set, is invoked after every successful admin
	// mutation. The Service injects the matched actor name (from
	// NamedAdmins, OIDC session subject, or "admin" for the AdminKey
	// fallback) so callers can log to whatever audit sink they prefer.
	// nil-safe: when unset, no audit happens.
	AdminAudit func(actor, action string, r *http.Request)

	oidc *OIDC // optional; populated by EnableOIDC (Phase F1)
}

// New returns a Service backed by the given KeyStore + admin key.
func New(store *keys.KeyStore, adminKey string) *Service {
	return &Service{Store: store, AdminKey: adminKey}
}

// Register attaches policy handlers to mux. OIDC routes are registered
// always, but they 503 unless EnableOIDC has been called.
func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/users", s.handleUsers)
	mux.HandleFunc("/api/v1/users/", s.handleUsers) // prefix match for /{id}/...
	mux.HandleFunc("/api/v1/access/check", s.handleAccessCheck)
	s.registerOIDCRoutes(mux)
}

// checkAdminAuth returns true if the request carries an admin credential.
// Three paths are accepted, in order of preference for actor attribution:
//
//  1. Bearer token in NamedAdmins (Phase C4) → actor = the matched name
//  2. Bearer token equal to AdminKey (legacy/M2M) → actor = "admin"
//  3. Valid OIDC session cookie with role=admin (Phase F1) → actor = subject
//
// When all three are unconfigured (AdminKey empty, NamedAdmins empty,
// no OIDC), every request passes — useful for dev/tests but never for
// production. The Service.AdminAudit hook (if set) sees the matched
// actor so callers can record per-human audit trails.
func (s *Service) checkAdminAuth(r *http.Request) bool {
	if s.AdminKey == "" && len(s.NamedAdmins) == 0 && s.oidc == nil {
		return true
	}
	return s.authenticateAdmin(r) != ""
}

// handleUsers routes all /api/v1/users* requests.
//
// Path layout:
//
//	/api/v1/users                                  → list (admin)
//	/api/v1/users/{id}                             → read profile (public)
//	/api/v1/users/{id}/models/{model}              → grant/revoke (admin)
//	/api/v1/users/{id}/quota                       → set quotas (admin)
func (s *Service) handleUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	parts := strings.Split(r.URL.Path, "/")
	// Split("/api/v1/users") == ["", "api", "v1", "users"] (len 4).
	if len(parts) < 4 {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}

	// Bare /api/v1/users (no id) — list-all.
	if len(parts) == 4 || (len(parts) == 5 && parts[4] == "") {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if !s.checkAdminAuth(r) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		users := s.Store.ListUsers()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"users": users,
			"count": len(users),
		})
		return
	}

	userID := parts[4]
	user, _ := s.Store.GetByID(userID)
	if user == nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}

	// /api/v1/users/{id}/models/{model} — admin
	if len(parts) >= 7 && parts[5] == "models" {
		if !s.checkAdminAuth(r) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		model := parts[6]
		actor := s.authenticateAdmin(r) // populated because checkAdminAuth passed
		switch r.Method {
		case http.MethodPost:
			if err := s.Store.GrantModelAccess(userID, model); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
				return
			}
			s.audit(actor, fmt.Sprintf("grant_model user=%s model=%s", userID, model), r)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "action": "granted", "model": model})
		case http.MethodDelete:
			if err := s.Store.RevokeModelAccess(userID, model); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
				return
			}
			s.audit(actor, fmt.Sprintf("revoke_model user=%s model=%s", userID, model), r)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "action": "revoked", "model": model})
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
		return
	}

	// /api/v1/users/{id}/quota — admin
	if len(parts) >= 6 && parts[5] == "quota" {
		if r.Method != http.MethodPatch {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if !s.checkAdminAuth(r) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		var req struct {
			MaxTokensPerDay   int64 `json:"max_tokens_per_day"`
			MaxTokensPerMonth int64 `json:"max_tokens_per_month"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		if err := s.Store.SetQuota(userID, req.MaxTokensPerDay, req.MaxTokensPerMonth); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		actor := s.authenticateAdmin(r)
		s.audit(actor, fmt.Sprintf("set_quota user=%s daily=%d monthly=%d", userID, req.MaxTokensPerDay, req.MaxTokensPerMonth), r)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":               "ok",
			"max_tokens_per_day":   req.MaxTokensPerDay,
			"max_tokens_per_month": req.MaxTokensPerMonth,
		})
		return
	}

	// /api/v1/users/{id} — read profile (public)
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	_ = json.NewEncoder(w).Encode(user)
}

// handleAccessCheck answers GET /api/v1/access/check?user=&model= with
// {allowed: bool}. No auth — this is a public pre-flight tool.
func (s *Service) handleAccessCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	user := r.URL.Query().Get("user")
	model := r.URL.Query().Get("model")
	if user == "" || model == "" {
		http.Error(w, `{"error":"user and model query params required"}`, http.StatusBadRequest)
		return
	}
	allowed := s.Store.CanAccess(user, model)
	code := http.StatusOK
	if !allowed {
		code = http.StatusForbidden
	}
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"user":    user,
		"model":   model,
		"allowed": allowed,
	})
}
