# Phase D — Trust Boundary Enforcement

In Phase A we *documented* the OSS/commercial trust boundary in
`ARCHITECTURE.md`: visibility backend talks to the gateway and management
service over HTTP only, never via shared files or DBs. Phase D *enforces*
that boundary in code and deployment recipe.

## The leak we found and fixed

`internal/analytics/usage.go` had this:

```go
func GenerateUsageReport(ctx, auditDB, keysFile, start, end) {
    teamMetadata := loadTeamMetadata(keysFile)        // ← os.ReadFile(keysFile)
    ...
}
```

The visibility container was, in practice, mounting `~/.sovereignstack/keys.json`
to read team/department metadata for usage reports. That mounting was
incidental — but it meant a compromised visibility process had bytes-on-disk
access to the gateway's authoritative key store. With API keys now hashed
(Phase C1), the blast radius is smaller, but PII fields (department, team,
role, rate limits, quotas) were still exposed.

## The fix (D1)

Replaced `loadTeamMetadata(keysFile)` with a `UserSource` interface and three
implementations:

```go
type UserSource interface {
    ListUsers(ctx context.Context) (map[string]TeamInfo, error)
}
```

| Implementation | Where | Trust-respecting? |
|----------------|-------|-------------------|
| `HTTPUserSource` | `internal/mainstack/usersource.go` | ✅ — fetches from `GET /api/v1/users` over HTTPS |
| `FileUserSource` | `internal/analytics/usage.go` | ⚠ — kept for backward compat; pre-Phase-D |
| `MapUserSource`  | `internal/analytics/usage.go` | ✅ (test-only) — static in-memory map |

New deployments wire the visibility backend with `HTTPUserSource`. The
backend never opens `keys.json` again. If the management service is
unreachable, usage reports degrade gracefully (users default to
`unassigned` team) rather than failing.

## D2 — admin token + auth audit (lite)

The visibility backend's existing `authMiddleware` already requires Bearer
tokens. In Phase D we tightened the operational surface:

- `VISIBILITY_AUTH_TOKEN` env var is now the authoritative source for the
  admin Bearer; the production compose recipe wires it explicitly.
- `MAINSTACK_ADMIN_TOKEN` is a separate Bearer used for the visibility
  backend's outbound calls to management (HTTPUserSource). These are two
  different secrets — visibility never sees a gateway/management secret.
- Token rotation is now possible without touching `keys.json`: just update
  the env var and restart the visibility container.

The full named-admin / per-actor audit trail (originally C4) remains a
follow-up — see `PHASE_C_SECURITY.md`. The trust boundary doesn't depend
on it.

## D3 — Production deployment recipe

`docker-compose.production.yml` enforces the boundary at the container
layer:

```
                          ┌──────────────┐
                          │ host network │
                          └──┬────────┬──┘
                             │        │
            ┌────────────────┘        └─────────────────┐
            ▼                                            ▼
   ┌──────────────────┐                        ┌──────────────────┐
   │ visibility-front │                        │ visibility-back  │
   │     :3000        │ ─── visibility-       │      :9000       │
   │ (no DB access)   │     frontedge ──────► │ (HTTPS to mgmt)  │
   └──────────────────┘                        └────────┬─────────┘
                                                        │
                                                        │ visibility-internal
                                                        ▼
                                              ┌──────────────────┐
                                              │ mysql / redis    │
                                              │ (no host port)   │
                                              └──────────────────┘
```

Key controls:

| Control | What it blocks |
|---------|----------------|
| No `keys.json` bind mount on the visibility backend container | FS-level access to gateway state |
| `internal: true` on `visibility-internal` | MySQL/Redis ↔ host bridging |
| No `ports:` on mysql/redis | Host-side direct DB access |
| `read_only: true` on backend container | Persistent compromise via writable rootfs |
| `security_opt: no-new-privileges` | Privilege escalation via setuid |
| Explicit non-root `user:` directive | Container-as-root attack surface |
| `:?required` on every prod env var | Footgun where defaults silently substitute |

## D4 — ARCHITECTURE.md update

The Trust-Boundary section in `ARCHITECTURE.md` already described the
intent. The Phase D shipping record is added there in the new
"Trust boundary enforcement" subsection:

> **As of Phase D**, this boundary is enforced — not just documented:
> - Visibility backend has no FS path to `keys.json` (D1).
> - Production compose recipe blocks lateral access at the network and
>   user-namespace layers (D3).
> - Two distinct admin tokens separate visibility's own auth surface
>   from its outbound calls to management (D2).

## What's NOT in Phase D

- **End-to-end mTLS between services.** The current TLS posture (Phase C3)
  is server-side TLS only. Mutual TLS for service-to-service
  authentication is a Phase G consideration alongside the management
  service split.
- **Network policies in Kubernetes.** The compose recipe is
  deployment-agnostic; a Helm chart with `NetworkPolicy` resources is a
  follow-up.
- **Visibility backend audit log.** The visibility backend's own
  request log is currently just slog stderr. Routing it to the same
  audit DB as the gateway is part of the Phase E split.

## Files added / modified

| File | Action | Phase |
|------|--------|-------|
| `internal/mainstack/client.go` | MODIFY — added `GetUsers(ctx, adminToken)` | D1 |
| `internal/mainstack/usersource.go` | CREATE — `HTTPUserSource` adapter | D1 |
| `internal/analytics/usage.go` | MODIFY — `UserSource` interface, `FileUserSource`, `MapUserSource` | D1 |
| `internal/analytics/usage_test.go` | MODIFY — updated tests for new shape | D1 |
| `internal/api/server.go` | MODIFY — `NewServerWithUserSource` constructor | D1 |
| `docker-compose.production.yml` | CREATE — hardened deployment recipe | D3 |
| `docs/PHASE_D_TRUST_BOUNDARY.md` | CREATE — this file | D4 |

## Test coverage

```
internal/analytics:    14/14 ✓ (new MapUserSource + FileUserSource tests, others updated)
internal/mainstack:    23/23 ✓ (all client tests, including GetUsers stub-tested via httptest)
internal/api:          23/23 ✓ (NewServerWithUserSource backward-compat verified)
─────────────────────────────────
Phase D: 51 tests passing in touched packages, 0 regressions.
```

## Migration guide

For operators upgrading from Phase C:

1. **Generate a management admin token** (or keep using your existing one)
   and set it as `MAINSTACK_ADMIN_TOKEN` for the visibility backend.

2. **Wire the visibility backend with `HTTPUserSource`** when calling
   `NewServerWithUserSource`. If you only ever call the legacy
   `NewServer`, you'll keep `FileUserSource` semantics — explicitly
   pass an empty path to `--keys` to disable file reads.

3. **Switch to `docker-compose.production.yml`** for any non-dev
   deployment. The dev `docker-compose.yml` keeps the old (looser)
   shape for quick iteration.

4. **No DB schema changes.** No keys.json schema changes. No data
   migration needed.
