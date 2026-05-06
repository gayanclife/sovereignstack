# Phase G — Observability + ops

The polish phase. Each item here is small individually but together they
turn the stack from "runs in production" into "operationally pleasant."

## G1 — OpenTelemetry tracing

`core/tracing` configures the global `TracerProvider` for any service
that calls `tracing.Init(ctx, serviceName)`. Wired into all four cobra
commands (`gateway`, `discovery`, `policy`, `metrics-proxy`).

**Zero overhead by default.** When `OTEL_EXPORTER_OTLP_ENDPOINT` is
unset, `Init` returns a no-op shutdown without touching the SDK. No
exporters, no batchers, no goroutines.

**Turning it on.** Set the standard OTel env vars:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=jaeger:4317
OTEL_EXPORTER_OTLP_INSECURE=true     # dev / Jaeger sidecar
OTEL_SERVICE_NAME=sovstack-gateway   # optional override
```

W3C TraceContext propagation is configured automatically — when you set
`traceparent` from a calling client, it flows through the gateway's
reverse-proxy to the model container.

**Local Jaeger setup** (one-line dev recipe):

```bash
docker run -d --name jaeger \
  -p 4317:4317 \
  -p 16686:16686 \
  jaegertracing/all-in-one:latest

OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317 \
OTEL_EXPORTER_OTLP_INSECURE=true \
sovstack gateway --keys ~/.sovereignstack/keys.json
```

Visit `http://localhost:16686` and you'll see spans flowing.

## G2 — Audit log retention

`core/audit/Pruner` runs every hour against the SQLite hot path and
deletes rows older than `gateway.audit.retention_days`. Wired into the
gateway when retention is configured > 0.

```yaml
gateway:
  audit:
    retention_days: 30        # 0 disables; default is 0 (keep forever)
```

The pruner notifies the gateway logger after each pass so you see
`audit prune rows_deleted=N` log lines.

JSONL cold-path files are NOT pruned by this — those are operator-managed
via `logrotate`:

```
/etc/logrotate.d/sovereignstack-audit:
/var/log/sovereignstack/audit-*.jsonl.gz {
    daily
    rotate 365
    compress
    missingok
    notifempty
}
```

## G3 — Scrape freshness

The visibility backend's `GatewayCollector` now exposes:

```
GET /api/v1/visibility/scrape-status
→ {
    "last_success_at": "2026-05-02T10:14:23Z",
    "seconds_since_success": 12,
    "last_error": ""
  }
```

The frontend can show a "data is stale" banner when
`seconds_since_success > 2 × scrape_interval` (typically > 30s for the
default 15s interval). Returns `seconds_since_success: -1` when the
collector hasn't completed any successful scrape yet (boot-time race
when the gateway isn't up first).

## G4 — Hosted pricing data

`internal/financial/Calculator` now accepts either a local file path or
an `http(s)://` URL via `--pricing` / `visibility.pricing` config:

```yaml
visibility:
  pricing: https://raw.githubusercontent.com/sovereignstack/pricing-data/main/pricing.json
```

Behaviour:
- URL is fetched once at startup with a 10s timeout.
- HTTP failure → `WARNING: pricing load failed (...)` to stderr, calculator starts with an empty pricing table.
- Empty pricing means "savings vs cloud" numbers are zero, but every other usage analysis still works.
- For air-gapped deployments, point at a local file instead.

The hosted file is intentionally the simplest possible delivery — a flat
JSON in a public repo. Operators who need offline-resilient pricing
should `wget` it via cron to a local path.

## G5 — Multi-gateway HA recipe

Phase B5 added a Redis-backed quota backend. Phase G makes the HA story
explicit: with Redis, multiple gateway replicas behind a load balancer
share quota state correctly. Without it, replicas have independent
counters and a user with a 1M monthly quota can spend 2M.

**Compose snippet** (extends the production compose):

```yaml
services:
  gateway-a:
    image: sovstack:1
    command: gateway --port 8001 --keys /run/secrets/keys.json
    environment:
      SOVSTACK_QUOTA_BACKEND: redis
      SOVSTACK_QUOTA_REDIS_ADDR: redis:6379
    depends_on:
      redis:
        condition: service_healthy

  gateway-b:
    image: sovstack:1
    command: gateway --port 8001 --keys /run/secrets/keys.json
    environment:
      SOVSTACK_QUOTA_BACKEND: redis
      SOVSTACK_QUOTA_REDIS_ADDR: redis:6379
    depends_on:
      redis:
        condition: service_healthy

  redis:
    image: redis:7-alpine
    command: redis-server --appendonly yes
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
```

Both replicas share the same `keys.json` (read-only mount) and the same
Redis quota counter. A simple round-robin nginx upstream block in front
distributes requests.

For more sophisticated routing (sticky sessions for streaming responses,
header-based routing to specific models), the existing `core/gateway/router.go`
remains the routing engine — it just runs in each replica.

## What's NOT in Phase G

- **Span instrumentation in business logic.** Init wires up the global
  TracerProvider; emitting spans from `proxy.ServeHTTP` and the policy
  handlers is a follow-up. Once you set the env var, you'll see only
  the auto-emitted gRPC/HTTP spans from the OTel libraries until that
  follow-up lands.
- **mTLS between split services.** Phase G adds TLS for browser-facing
  services (already in C3). Service-to-service mTLS is its own phase.
- **Auto-scaling triggers.** The compose recipe assumes static replica
  counts. Hooking gateway metrics to a horizontal pod autoscaler is a
  Kubernetes-shaped concern.

## Files added / modified

| File | Action | Phase |
|------|--------|-------|
| `core/tracing/tracing.go` | CREATE — OTel `Init(ctx, serviceName)` helper | G1 |
| `cmd/gateway.go` | MODIFY — `tracing.Init` call + retention pruner wiring | G1+G2 |
| `cmd/discovery.go` | MODIFY — `tracing.Init` call | G1 |
| `cmd/policy.go` | MODIFY — `tracing.Init` call | G1 |
| `cmd/metrics_proxy.go` | MODIFY — `tracing.Init` call | G1 |
| `core/audit/retention.go` | CREATE — `Pruner` with hourly DELETE loop | G2 |
| `core/audit/retention_test.go` | CREATE — 4 tests | G2 |
| `core/audit/sqlite.go` | MODIFY — `DB()` accessor for the pruner | G2 |
| `core/config/config.go` | (already had `audit.retention_days`) | G2 |
| `internal/collector/gateway_collector.go` | MODIFY — `lastSuccessAt`/`lastError`, freshness accessors | G3 |
| `internal/api/server.go` | MODIFY — `/api/v1/visibility/scrape-status` handler | G3 |
| `internal/financial/calculator.go` | MODIFY — `loadPricingSource` accepts URL or file; warns on failure | G4 |

## Test coverage

```
core/audit (incl. retention):     22/22 ✓
core/tracing:                      0   (no-op when env unset; integration-tested manually)
core/keys, core/gateway, etc.    same as Phase F (no regressions)
visibility/api, /collector, /financial: same as Phase F (no regressions)
─────────────────────────────────
Phase G total tests added: 4 (retention).
```

Smoke verified end-to-end:
- Gateway boot with `OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
  OTEL_EXPORTER_OTLP_INSECURE=true` connects to a Jaeger sidecar without
  emitting any HTTP errors. Spans appear in Jaeger UI for the auto-instrumented
  gRPC export round-trips.
- `gateway.audit.retention_days: 30` triggers the `audit retention enabled`
  log line at startup; the hourly pruner runs without error against an
  empty audit DB.

## Migration guide

For operators upgrading from Phase F:

1. **No breaking changes.** Phase G is additive — every new feature is
   opt-in via config or env var.
2. **To enable tracing**, set `OTEL_EXPORTER_OTLP_ENDPOINT`. No restart
   required if you're orchestrating with a process manager that re-reads
   env on SIGHUP; otherwise restart.
3. **To enable audit retention**, set `gateway.audit.retention_days`.
   Recommended: 30 for the SQLite hot path; pair with a `logrotate` rule
   for the JSONL cold path.
4. **To switch pricing to hosted**, change `visibility.pricing` from a
   local path to the GitHub raw URL. No data migration; pricing is
   reloaded at every visibility-backend restart.
5. **For HA**, switch `gateway.quota.backend` from `sqlite` to `redis`,
   provision a Redis instance, run two gateway replicas behind a load
   balancer.
