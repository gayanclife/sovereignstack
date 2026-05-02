# Phase A — Foundations

Foundational changes applied before all later phases. These are pure
infrastructure: config plumbing, structured logging, API versioning, CORS,
health probes, and CLI ergonomics. No new functionality — but everything
later builds on them.

## Goals

- Replace ad-hoc `fmt.Printf` logging with structured `log/slog`
- Replace flag-only configuration with a YAML config file (still
  flag/env-overridable)
- Cut over every HTTP API path to `/api/v1/*` (incubating: hard cutover, no
  compatibility shim)
- Replace open `*` CORS with an explicit allowlist
- Add Kubernetes-style `/healthz` and `/readyz` probes
- Add `--output json` to CLI commands that produce reportable output
- Document everything

## What changed

### A1 — Structured logging (`core/logging/`)

`logging.Init(cfg.Log)` configures `slog.SetDefault` with either a text or
JSON handler. All services log via `slog.Info`, `slog.Warn`, etc., and
attach a `service=<name>` tag via `logging.Service("<name>")`.

In **text mode** (default), the existing decorative startup banners are
preserved for human operators.
In **json mode**, banners are suppressed and only structured records are
written — suitable for log shippers (Loki, Datadog, Splunk, etc.).

```bash
# Default text output
sovstack gateway

# JSON, ready for a log shipper
sovstack gateway --log-format json --log-level info
```

Tests: 8 in `core/logging/logging_test.go` covering format selection, level
filtering, service tagging, and error cases.

### A2 — YAML config (`core/config/`)

A small (~190 LOC) YAML loader with strict precedence:

> CLI flags > environment variables > YAML file > defaults

```yaml
# sovstack.yaml
log:
  format: json
  level: info
gateway:
  port: 8001
  rate_limit: 100
  keys_file: ~/.sovereignstack/keys.json
```

Env-var overrides are declared via struct tags (`env:"SOVSTACK_..."`).
CLI overrides happen at the call site via `cobra`'s `cmd.Flags().Changed(...)`
so users only override what they explicitly type.

Tests: 10 in `core/config/config_test.go` covering all four precedence
levels, malformed YAML, ~/ expansion, and bool/int parsing.

### A3 — `/api/v1/*` hard cutover

Every server-side route registration and every client URL was rewritten.
Path-parsing handlers (e.g. `handleUsers`, `handleModelEndpoints`) had
their part-index offsets shifted by one because the path now has an extra
`v1` segment.

| Before | After |
|--------|-------|
| `/api/users` | `/api/v1/users` |
| `/api/models/running` | `/api/v1/models/running` |
| `/api/models/{name}/metrics` | `/api/v1/models/{name}/metrics` |
| `/api/access/check` | `/api/v1/access/check` |
| `/api/audit/logs` | `/api/v1/audit/logs` |
| `/api/visibility/*` | `/api/v1/visibility/*` |
| `/api/health` | `/api/v1/health` (legacy; prefer `/healthz`) |

Files touched: 60. No backward-compat shim — incubating stage means we
break callers cleanly rather than carry forwards-compat baggage.

### A4 — Configurable CORS

`cors.origins []string` config + `--cors-origins <csv>` flag. The previous
hardcoded `Access-Control-Allow-Origin: *` is gone. `cors.allow_dev_wildcard:
true` (and `--cors-allow-dev-wildcard`) is an explicit dev-only escape
hatch — it echoes any incoming `Origin` and warns at startup.

Tests: 7 in `internal/api/cors_test.go` (visibility) covering allowed,
disallowed, no-origin, dev wildcard, and multiple-origins scenarios.

### A5 — Health probes (`core/health/`)

Two endpoints per service:

- `GET /healthz` — liveness; 200 unconditionally if the HTTP server is
  responsive. Use for container restart policies.
- `GET /readyz` — readiness; runs all registered dependency checks in
  parallel (3 s timeout each), returns 200 only if every check passes.

```json
// /readyz response when one downstream is unreachable
{
  "status": "unhealthy",
  "uptime_seconds": 12,
  "checks": {
    "keystore":   {"status": "ok",   "latency_ms": 0},
    "management": {"status": "fail", "latency_ms": 12, "error": "dial tcp 127.0.0.1:8888: connect: connection refused"}
  }
}
```

Built-in `health.HTTPCheck(client, url)` is used by the gateway to verify
the management service is reachable. The management service registers its
own `keystore` check.

Tests: 9 in `core/health/health_test.go` covering all paths, including
timeout-as-failure.

### A6 — CLI `--output json`

A small helper `emit(cmd, payload, textRenderer)` in `cmd/output.go`
either renders the payload as indented JSON to stdout (for piping to `jq`)
or invokes the human-friendly text renderer. Wired into `keys add | list |
info | remove`.

```bash
sovstack keys list -o json | jq '.users[] | select(.role=="admin")'
sovstack keys info alice -o json | jq -r .key   # extract just the key
```

### A7 — Documentation

This document, plus updated `README.md` for both stacks (with breaking-change
notice at the top) and the top-level `ARCHITECTURE.md` describing the new
Phase A surface.

## Files added / modified

| File | Action |
|------|--------|
| `core/config/config.go` | CREATE — YAML loader + env override (~190 LOC) |
| `core/config/config_test.go` | CREATE — 10 tests |
| `core/logging/logging.go` | CREATE — slog wrapper (~110 LOC) |
| `core/logging/logging_test.go` | CREATE — 8 tests |
| `core/health/health.go` | CREATE — liveness/readiness checker (~180 LOC) |
| `core/health/health_test.go` | CREATE — 9 tests |
| `cmd/root.go` | MODIFY — persistent `--config` flag, `loadConfig()` helper |
| `cmd/output.go` | CREATE — `emit()` helper for text/json output |
| `cmd/gateway.go` | MODIFY — config + slog + health probes |
| `cmd/management.go` | MODIFY — config + slog + health probes + path parsing for v1 |
| `cmd/keys.go` | MODIFY — `--output` flag, all subcommands use `emit()` |
| `internal/api/server.go` | MODIFY — visibility CORS allowlist, v1 routes |
| `internal/api/cors_test.go` | CREATE — 7 tests |
| `cmd/visibility/main.go` | MODIFY — `--cors-origins`, `--cors-allow-dev-wildcard` flags |
| `sovstack.yaml.example` | CREATE — annotated config reference |
| `README.md` (both stacks) | MODIFY — breaking-change notice |
| `ARCHITECTURE.md` (top level) | MODIFY — Phase A foundations section |

## Test coverage

```
core/config:    10/10 ✓
core/logging:    8/8  ✓
core/health:     9/9  ✓
core/keys:      14/14 ✓
core/gateway:   49/49 ✓
core/audit:      7/7  ✓
cmd:            10/10 ✓ (4 keys + 4 management + 2 root)
visibility/api: 16/16 ✓ + 7 cors ✓
visibility/collector: 9/9 ✓
─────────────────────────────────
Total: 138 tests passing
```

5 pre-existing engine integration tests fail in environments where Docker
already has containers running — unrelated to Phase A, environmental only.

## Migration guide for existing users

If you had earlier incubating builds running:

1. **API path callers** — anything talking to `/api/users`, `/api/models`,
   `/api/visibility/...` must be updated to `/api/v1/...`. The gateway
   itself doesn't need updating (its model-routing path remains `/v1/...`).

2. **Frontend `lib/api.ts`** — already updated as part of A3. Re-pull and
   rebuild.

3. **CORS** — add your dashboard origin to config (or pass `--cors-origins`).
   Browser fetches that worked under the wildcard will start failing
   otherwise.

4. **Healthcheck callers** (load balancers, k8s probes) — switch from
   `/api/health` to `/healthz` and `/readyz`. The legacy path still works
   but will be removed in Phase E (management split).

5. **No keys.json migration needed** — the file format is unchanged.
   Hashing of keys at rest happens in Phase C.

6. **No DB migration needed** — the database layer is untouched. MySQL
   migration is Phase B.
