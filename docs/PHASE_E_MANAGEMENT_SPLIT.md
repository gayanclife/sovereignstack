# Phase E — Management service split

The pre-Phase-E management service was three services in one trench coat:

1. **Docker discovery** — list running model containers
2. **User policy** — manage `keys.json`, grant/revoke models, set quotas
3. **vLLM metrics proxy** — forward `/metrics` from each model container

Each subset has different auth requirements, lifetimes, scaling
characteristics, and FS-access needs. Phase E splits them into three
packages and three commands while keeping the monolithic command as a
backward-compat shim.

## Three packages

```
core/management/
├── discovery/        # GET /api/v1/models/running, /api/v1/health
├── policy/           # GET/POST/DELETE/PATCH /api/v1/users/*, /api/v1/access/check
└── metricsproxy/     # GET /api/v1/models/{name}/metrics
```

Each package exports:
- A `Service` struct holding its dependencies (KeyStore for policy;
  Docker resolver for discovery + metricsproxy).
- A `New(...)` constructor.
- A `Register(mux *http.ServeMux)` method that attaches handlers.

This shape lets every package be tested in isolation (no global state)
and deployed in any combination — three separate ports, two combined,
or all three on one port (the legacy mode).

## Three new commands

```
sovstack discovery       # port 8889, no auth, stateless
sovstack policy          # port 8888, admin auth, owns keys.json
sovstack metrics-proxy   # port 8890, no auth, stateless
sovstack management      # port 8888, all three (deprecated)
```

The legacy `management` command still works and now mounts all three
packages internally — but it logs a deprecation warning at startup. New
deployments should run the three split commands.

## Why this matters

| Concern | Before | After |
|---------|--------|-------|
| FS access to keys.json | Whole management service | Only the `policy` binary |
| Admin auth surface | One process | One process (policy); discovery + metrics-proxy require none |
| Container privileges | One container needs Docker socket *and* keys.json | Discovery + metrics-proxy need Docker socket; policy needs keys.json. Different containers, different users. |
| Restart blast radius | Restart management = downtime for all three | Restart any one without affecting the others |
| Compromise blast radius | Discovery vuln → keys.json access | Discovery vuln → no auth state on the same machine |

## Deployment patterns

### (a) Monolith (backward-compat / dev)

```bash
sovstack management --port 8888 --keys ~/.sovereignstack/keys.json
```

Runs all three subservices on port 8888. Works for single-host installs
and for upgrading from earlier builds without changing reverse-proxy
config.

### (b) Split (production)

```bash
# On host A (Docker socket access, no keys.json):
sovstack discovery     --port 8889
sovstack metrics-proxy --port 8890

# On host B (keys.json access, no Docker socket):
sovstack policy --port 8888 --admin-key $SOVSTACK_ADMIN_KEY
```

Production compose recipe (`docker-compose.production.yml`):

```yaml
services:
  policy:
    image: sovstack:1
    command: policy --port 8888
    volumes: ["~/.sovereignstack/keys.json:/keys.json:rw"]
    networks: ["mgmt"]
  discovery:
    image: sovstack:1
    command: discovery --port 8889
    volumes: ["/var/run/docker.sock:/var/run/docker.sock:ro"]
    networks: ["mgmt"]
  metrics-proxy:
    image: sovstack:1
    command: metrics-proxy --port 8890
    volumes: ["/var/run/docker.sock:/var/run/docker.sock:ro"]
    networks: ["mgmt"]
```

The gateway and visibility backend continue to call the documented HTTP
endpoints — no callers know whether they hit one process or three.

## What didn't break

- **Endpoint paths and shapes** — all three subservices preserve the
  exact paths and request/response JSON of the pre-split monolith.
- **Existing tests** — `cmd/management_test.go` and the visibility
  backend's collector still pass without changes.
- **Single-port operators** — the legacy `management` command continues
  to mount all three on port 8888.

## What's NOT in Phase E

- **mTLS between services.** Currently the three split services trust
  each other on the same internal network. Mutual TLS for
  service-to-service authentication is a Phase G consideration.
- **Per-actor admin attribution.** The audit logging for "which named
  admin made this change" was deferred from Phase C4; the policy
  service's admin auth is currently a single shared Bearer.

## Files added / modified

| File | Action |
|------|--------|
| `core/management/discovery/discovery.go` | CREATE — discovery handlers |
| `core/management/discovery/discovery_test.go` | CREATE — 3 tests |
| `core/management/policy/policy.go` | CREATE — policy handlers |
| `core/management/policy/policy_test.go` | CREATE — 8 tests (auth, grant/revoke, quota, access check) |
| `core/management/metricsproxy/metricsproxy.go` | CREATE — metrics-proxy handlers |
| `core/management/metricsproxy/metricsproxy_test.go` | CREATE — 6 tests (incl. upstream-unreachable) |
| `cmd/discovery.go` | CREATE — `sovstack discovery` |
| `cmd/policy.go` | CREATE — `sovstack policy` |
| `cmd/metrics_proxy.go` | CREATE — `sovstack metrics-proxy` |
| `cmd/management.go` | MODIFY — now mounts all three packages internally; logs deprecation warning |

## Test coverage

```
core/management/discovery:    3/3   ✓
core/management/policy:       8/8   ✓
core/management/metricsproxy: 6/6   ✓
─────────────────────────────────
Phase E: 17 new tests passing.
```

End-to-end smoke verified:
- `sovstack discovery --port 28889` → `/healthz`, `/api/v1/models/running`
  both 200 with real Docker container data
- `sovstack metrics-proxy --port 28890` → `/healthz` 200,
  `/api/v1/models/nonexistent/metrics` correctly 404s
- `sovstack management` (legacy) starts, prints deprecation warning,
  serves all three subservice paths on port 8888
