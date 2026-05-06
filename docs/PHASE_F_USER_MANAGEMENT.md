# Phase F — User management

The OSS stack now ships with two complementary auth surfaces:

| Surface | What it auths | How |
|---------|---------------|-----|
| **API key (existing)** | `/v1/*` request path on the gateway | argon2id-hashed bearer; validated per-request |
| **OIDC session (new)** | Admin actions on the policy service | `/api/v1/auth/login` → IdP → cookie |

Plus a new **role model** that's enforced per-key:

| Role | Behaviour |
|------|-----------|
| `user` | Default. Subject to model allowlist + quotas. |
| `admin` | Can also pass policy-service admin checks (Bearer or OIDC session). |
| `viewer` | Read-only — enforced by the commercial dashboard, not the gateway. |
| `service` | Machine-to-machine. Subject to `IPAllowlist` if non-empty (Phase F2). |

## F1 — OIDC for OSS admin sign-in

The `policy` service now exposes:

```
GET /api/v1/auth/login      → 302 Found to IdP authorize URL
GET /api/v1/auth/callback   → exchange code, set sovstack_session cookie
GET /api/v1/auth/logout     → invalidate session, clear cookie
```

When configured, an admin signs in via the IdP. The IdP returns an
`id_token`; the policy service verifies it, reads the `role` claim
(configurable via `admin_claim`), and issues a signed session cookie.
Subsequent admin requests with the cookie pass `checkAdminAuth`.

Both auth paths coexist. The Bearer admin key keeps working for M2M
callers (e.g. the visibility backend's outbound calls). OIDC is for
human admins.

### Configuration (YAML)

```yaml
management:
  port: 8888
  keys_file: ~/.sovereignstack/keys.json
  admin_key: "..."              # legacy/M2M; can stay alongside OIDC
  oidc:
    issuer_url:    "https://keycloak.example.com/realms/sovstack"
    client_id:     "sovstack-policy"
    client_secret: "<oidc-client-secret>"
    redirect_url:  "https://policy.example.com/api/v1/auth/callback"
    admin_claim:   "role"         # default; the claim that carries 'admin'
```

### Environment variables (alternative to YAML)

```
SOVSTACK_OIDC_ISSUER_URL=...
SOVSTACK_OIDC_CLIENT_ID=...
SOVSTACK_OIDC_CLIENT_SECRET=...
SOVSTACK_OIDC_REDIRECT_URL=...
SOVSTACK_OIDC_ADMIN_CLAIM=role
```

### Keycloak example walkthrough

1. **Create a realm** in Keycloak: `sovstack`.
2. **Add a client** in the realm:
   - Client ID: `sovstack-policy`
   - Client authentication: ON
   - Valid redirect URIs: `https://policy.example.com/api/v1/auth/callback`
3. **Add a `role` mapper** that includes `realm_access.roles[0]` (or any
   single-value source) into the ID token claim named `role`.
4. **Create realm role `admin`** and assign it to the operator users who
   should be able to mutate `keys.json` via the dashboard.
5. **Save the client secret** from the Credentials tab into your
   `sovstack.yaml` (or as `SOVSTACK_OIDC_CLIENT_SECRET`).
6. Start the policy service. Visit
   `https://policy.example.com/api/v1/auth/login` — it redirects to
   Keycloak; on successful sign-in you land back at `/callback` with a
   `sovstack_session` cookie. `/api/v1/users` (admin) now accepts that
   cookie.

### Security model

- **Session cookie** is HMAC-SHA256-signed with a per-process secret
  (random on each start unless `session_secret` is configured). Tampering
  → 401. Cookie is `HttpOnly; Secure; SameSite=Lax; Path=/`.
- **State parameter** is required on `/callback`; mismatched or expired
  state (>10 minutes) → 400.
- **ID-token verification** uses the IdP's published JWKS (the `go-oidc`
  library refreshes them on rotation).
- **No refresh tokens** are stored. When the access cookie expires, the
  user signs in again — the policy service is admin-only and not a
  long-running browser session.
- **AdminKey Bearer remains valid** even when OIDC is on. Production
  deployments can rotate AdminKey to a high-entropy machine-only secret
  while routing humans through OIDC.

## F2 — Service accounts with IP allowlist

A new `service` role plus an `ip_allowlist` field on `UserProfile`:

```bash
sovstack keys add ci-runner \
  --role service \
  --ip-allowlist 10.0.0.0/8,172.16.0.0/12,203.0.113.5
```

The gateway, after authenticating an API key, looks up the profile. If
`role == "service"` and the source IP is **not** in the allowlist, the
request is rejected with 403:

```json
{"error": "source IP not allowed for this service account"}
```

For non-service roles the allowlist is silently ignored — humans behind
ever-changing NAT shouldn't fight an IP rule.

### Allowlist entry forms

- Single IP (v4 or v6): `203.0.113.5`, `2001:db8::1`
- CIDR: `10.0.0.0/8`, `fd00::/8`
- Source IP can be plain or `host:port`; the port is stripped before matching.

### Test coverage

```
core/keys (IP allowlist):                   9/9   ✓
core/management/policy (OIDC + admin):     18/18  ✓ (8 prior + 10 new for OIDC)
─────────────────────────────────────────
Phase F: 19 new tests passing.
```

## What's NOT in Phase F (deferred)

- **Native username + password** — for the OSS path, OIDC is the right
  delegation. Username/password is a commercial-layer feature
  (`sovereignstack-web` will gain Auth.js + bcrypt when the time comes).
- **Google SSO out of the box** — Google is just an OIDC provider. OSS
  users can wire `https://accounts.google.com` as the issuer, configure
  client credentials in Google Cloud Console, and the existing F1 flow
  handles the rest. The convenience bundle ("click Google logo to sign
  in") is a commercial UX feature, not a security feature.
- **Per-actor audit attribution for OIDC sessions** — admin actions
  performed via OIDC currently log the AdminKey Bearer's actor as
  empty. Threading the OIDC subject through to AuditLog is a follow-up
  alongside C4.

## Files added / modified

| File | Action | Phase |
|------|--------|-------|
| `core/keys/store.go` | MODIFY — added `Role` constants, `IPAllowlist` field | F2 |
| `core/keys/ipallowlist.go` | CREATE — `IsIPAllowed` method | F2 |
| `core/keys/ipallowlist_test.go` | CREATE — 9 tests | F2 |
| `core/gateway/access.go` | MODIFY — `IsSourceIPAllowed` on KeyStoreAccessController | F2 |
| `core/gateway/proxy.go` | MODIFY — IP allowlist enforced before access check | F2 |
| `cmd/keys.go` | MODIFY — `--ip-allowlist` and `--role service` validation | F2 |
| `core/management/policy/oidc.go` | CREATE — OIDC client + session signing | F1 |
| `core/management/policy/oidc_test.go` | CREATE — 10 tests | F1 |
| `core/management/policy/policy.go` | MODIFY — Service.oidc field; checkAdminAuth honours session | F1 |
| `core/config/config.go` | MODIFY — added `OIDCConfig` under `management.oidc` | F1 |
| `cmd/policy.go` | MODIFY — calls `EnableOIDC` if issuer configured | F1 |

## Migration guide

For operators upgrading from Phase E:

1. **No breaking changes.** Existing AdminKey Bearer auth continues to
   work. OIDC is opt-in.

2. **To enable OIDC**, set the four `management.oidc.*` values in YAML
   (or the four `SOVSTACK_OIDC_*` env vars). Restart the policy service.
   Visit `/api/v1/auth/login`.

3. **To convert an existing user into a service account**, edit the
   profile (or recreate it):
   ```bash
   sovstack keys remove ci-runner
   sovstack keys add ci-runner \
     --role service \
     --ip-allowlist 10.0.0.0/8
   ```

4. **No DB schema changes.** No keys.json schema break (new fields are
   optional with `omitempty`).
