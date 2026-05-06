// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package policy

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCConfig is what an operator passes to enable Phase F1 sign-in.
// All four fields must be non-empty to activate OIDC; otherwise the OIDC
// endpoints stay registered but reject with 503.
type OIDCConfig struct {
	IssuerURL    string // e.g. "https://keycloak.example.com/realms/sovstack"
	ClientID     string
	ClientSecret string
	RedirectURL  string // e.g. "https://policy.example.com/api/v1/auth/callback"

	// AdminClaim is the OIDC ID-token claim whose value (a string) is
	// taken as the user's role. Defaults to "role". A user with role==
	// "admin" passes Service.checkAdminAuth via session cookie.
	AdminClaim string

	// SessionTTL is how long a successful sign-in remains valid.
	// Defaults to 8h.
	SessionTTL time.Duration

	// SessionSecret signs the session cookie. If empty, a random secret
	// is generated at process start (cookies invalidate on restart).
	SessionSecret []byte
}

// OIDC is the optional OpenID Connect handler bundle. Wired into Service
// when EnableOIDC is called. The Service tracks whether OIDC is active so
// checkAdminAuth can fall back to Bearer-token mode for legacy callers.
type OIDC struct {
	cfg      OIDCConfig
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config

	mu       sync.RWMutex
	sessions map[string]oidcSession // sessionID → session
	states   map[string]time.Time   // OAuth `state` → expiry
}

type oidcSession struct {
	subject string
	role    string
	expires time.Time
}

// EnableOIDC turns on the /api/v1/auth/* endpoints on this Service. If
// cfg has missing required fields, the endpoints respond 503; the Service
// keeps the legacy admin-key Bearer behaviour so existing callers don't
// break.
func (s *Service) EnableOIDC(ctx context.Context, cfg OIDCConfig) error {
	if cfg.IssuerURL == "" || cfg.ClientID == "" || cfg.RedirectURL == "" {
		return fmt.Errorf("OIDC requires IssuerURL, ClientID, and RedirectURL")
	}
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 8 * time.Hour
	}
	if cfg.AdminClaim == "" {
		cfg.AdminClaim = "role"
	}
	if len(cfg.SessionSecret) == 0 {
		cfg.SessionSecret = make([]byte, 32)
		if _, err := rand.Read(cfg.SessionSecret); err != nil {
			return fmt.Errorf("generate session secret: %w", err)
		}
	}

	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return fmt.Errorf("OIDC discover: %w", err)
	}

	s.oidc = &OIDC{
		cfg:      cfg,
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		oauth: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  cfg.RedirectURL,
			Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
		},
		sessions: make(map[string]oidcSession),
		states:   make(map[string]time.Time),
	}
	return nil
}

// registerOIDCRoutes is called from Register if OIDC is enabled. The three
// routes are always registered; if OIDC is nil they 503.
func (s *Service) registerOIDCRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("/api/v1/auth/callback", s.handleCallback)
	mux.HandleFunc("/api/v1/auth/logout", s.handleLogout)
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		http.Error(w, `{"error":"oidc not configured"}`, http.StatusServiceUnavailable)
		return
	}
	state := randB64(16)
	s.oidc.mu.Lock()
	s.oidc.states[state] = time.Now().Add(10 * time.Minute)
	s.oidc.mu.Unlock()

	url := s.oidc.oauth.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusFound)
}

func (s *Service) handleCallback(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		http.Error(w, `{"error":"oidc not configured"}`, http.StatusServiceUnavailable)
		return
	}
	ctx := r.Context()

	state := r.URL.Query().Get("state")
	s.oidc.mu.Lock()
	expiry, ok := s.oidc.states[state]
	delete(s.oidc.states, state)
	s.oidc.mu.Unlock()
	if !ok || time.Now().After(expiry) {
		http.Error(w, `{"error":"invalid state"}`, http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, `{"error":"missing code"}`, http.StatusBadRequest)
		return
	}

	tok, err := s.oidc.oauth.Exchange(ctx, code)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"oidc exchange: %s"}`, err.Error()), http.StatusBadGateway)
		return
	}
	rawID, _ := tok.Extra("id_token").(string)
	if rawID == "" {
		http.Error(w, `{"error":"no id_token in oidc response"}`, http.StatusBadGateway)
		return
	}
	idToken, err := s.oidc.verifier.Verify(ctx, rawID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"id_token verify: %s"}`, err.Error()), http.StatusUnauthorized)
		return
	}

	var claims map[string]any
	_ = idToken.Claims(&claims)
	role, _ := claims[s.oidc.cfg.AdminClaim].(string)
	subject := idToken.Subject

	sid := randB64(24)
	s.oidc.mu.Lock()
	s.oidc.sessions[sid] = oidcSession{
		subject: subject,
		role:    role,
		expires: time.Now().Add(s.oidc.cfg.SessionTTL),
	}
	s.oidc.mu.Unlock()

	signed := signSession(sid, s.oidc.cfg.SessionSecret)
	http.SetCookie(w, &http.Cookie{
		Name:     "sovstack_session",
		Value:    signed,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(s.oidc.cfg.SessionTTL),
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"subject": subject,
		"role":    role,
		"expires": time.Now().Add(s.oidc.cfg.SessionTTL).Format(time.RFC3339),
	})
}

func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		http.Error(w, `{"error":"oidc not configured"}`, http.StatusServiceUnavailable)
		return
	}
	cookie, err := r.Cookie("sovstack_session")
	if err == nil {
		if sid, ok := verifySession(cookie.Value, s.oidc.cfg.SessionSecret); ok {
			s.oidc.mu.Lock()
			delete(s.oidc.sessions, sid)
			s.oidc.mu.Unlock()
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:    "sovstack_session",
		Value:   "",
		Path:    "/",
		MaxAge:  -1,
		Expires: time.Unix(0, 0),
	})
	w.WriteHeader(http.StatusNoContent)
}

// SessionRoleFromRequest returns ("admin", true) when the request carries
// a valid session cookie whose claim value matches "admin". Otherwise
// ("", false). Used by checkAdminAuth as a session-based alternative to
// Bearer admin keys.
func (s *Service) sessionRoleFromRequest(r *http.Request) (string, bool) {
	if s.oidc == nil {
		return "", false
	}
	cookie, err := r.Cookie("sovstack_session")
	if err != nil {
		return "", false
	}
	sid, ok := verifySession(cookie.Value, s.oidc.cfg.SessionSecret)
	if !ok {
		return "", false
	}
	s.oidc.mu.RLock()
	sess, ok := s.oidc.sessions[sid]
	s.oidc.mu.RUnlock()
	if !ok || time.Now().After(sess.expires) {
		return "", false
	}
	return sess.role, true
}

// signSession returns sid|HMAC-SHA256(secret, sid), base64-encoded.
func signSession(sid string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(sid))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return sid + "." + sig
}

// verifySession parses signed session cookie; returns sid + ok.
func verifySession(value string, secret []byte) (string, bool) {
	dot := strings.LastIndex(value, ".")
	if dot < 0 {
		return "", false
	}
	sid, sig := value[:dot], value[dot+1:]
	expected, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(sid))
	if !hmac.Equal(mac.Sum(nil), expected) {
		return "", false
	}
	return sid, true
}

func randB64(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
