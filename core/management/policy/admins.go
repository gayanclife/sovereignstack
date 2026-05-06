// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package policy

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"
)

// NamedAdmins maps an admin actor name (e.g. "alice", "ci-bot") to the
// Bearer token they present. This is the Phase C4 replacement for the
// single shared AdminKey: every successful admin call is attributable to
// a specific named principal so audit trails show "who did what."
//
// The single-key AdminKey on Service is preserved for backward compat —
// when set, it acts as a fallback principal named "admin".
type NamedAdmins map[string]string

// authenticateAdmin returns the actor name matching the request's Bearer
// token, or "" when none match. Constant-time compare per token to avoid
// leaking which tokens exist via response timing.
func (s *Service) authenticateAdmin(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		// No Bearer present; fall through to OIDC session check.
		return s.actorFromOIDCSession(r)
	}
	token := auth[len("Bearer "):]

	// Single-key fallback: the legacy AdminKey is treated as actor "admin".
	if s.AdminKey != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.AdminKey)) == 1 {
		return "admin"
	}

	for name, expected := range s.NamedAdmins {
		if expected == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1 {
			return name
		}
	}
	return s.actorFromOIDCSession(r)
}

// audit invokes the AdminAudit hook if set. nil-safe.
func (s *Service) audit(actor, action string, r *http.Request) {
	if s.AdminAudit == nil {
		return
	}
	if actor == "" {
		actor = "unknown"
	}
	s.AdminAudit(actor, action, r)
}

// actorFromOIDCSession returns the OIDC subject when a valid session
// cookie is present, the session has not expired, and its role is admin;
// otherwise "".
func (s *Service) actorFromOIDCSession(r *http.Request) string {
	if s.oidc == nil {
		return ""
	}
	cookie, err := r.Cookie("sovstack_session")
	if err != nil {
		return ""
	}
	sid, ok := verifySession(cookie.Value, s.oidc.cfg.SessionSecret)
	if !ok {
		return ""
	}
	s.oidc.mu.RLock()
	defer s.oidc.mu.RUnlock()
	sess, ok := s.oidc.sessions[sid]
	if !ok || sess.role != "admin" {
		return ""
	}
	if time.Now().After(sess.expires) {
		return ""
	}
	return sess.subject
}
