# Architecture

SovereignStack is one Go binary (`sovstack`) that knows how to run as
four different services. All four speak HTTP-only — no shared databases,
no shared filesystems, no shared in-process state.

## Topology

```
  API client (curl, SDK)
       │
       │ HTTPS
       ▼
  ┌──────────────────────────────────────┐
  │  Gateway (:8001)                     │
  │  Auth → Access → Quota → Rate-limit  │
  │  → Routing → Reverse proxy           │
  │  Exposes Prometheus /metrics         │
  └──────────┬─────────────────┬─────────┘
             │ poll            │ proxy /v1/...
             ▼                 ▼
  ┌────────────────────┐  ┌────────────────────┐
  │ Discovery (:8889)  │  │ Model containers   │
  │ Policy (:8888)     │  │ vLLM / llama.cpp   │
  │ Metrics-proxy(:8890)│  │                    │
  └────────────────────┘  └────────────────────┘
             │ docker socket (read-only)
             ▼
       Docker daemon
```

## Components

### Gateway (`sovstack gateway`)

The hot path. Every API client request flows through here. In order:

1. **Auth** — validates `X-API-Key` / `Bearer` against the key store
2. **Access control** — checks the requested model against the user's
   `allowed_models` (supports `["*"]` wildcard)
3. **IP allowlist** — for `service` accounts only, rejects calls from
   IPs not in `IPAllowlist`
4. **Quota check** — pre-flight against daily / monthly token caps
5. **Rate limit** — token-bucket per user
6. **Model routing** — `/v1/models/{name}/...` rewritten to the model
   container's port (registry refreshed every 30s from `discovery`)
7. **Reverse proxy** — forwards to the model container, captures status & latency
8. **Metrics** — atomic counters + Prometheus text format on `/metrics`
9. **Audit** — request / response logged to SQLite + (optional) JSONL

Quota is recorded *after* the response (token counts come from vLLM's
`usage` field).

### Policy (`sovstack policy`)

Owns `keys.json`. The control plane.

| Endpoint | Auth | Purpose |
|----------|------|---------|
| `GET /api/v1/users` | admin | List all users |
| `GET /api/v1/users/{id}` | — | Read one profile |
| `POST /api/v1/users/{id}/models/{model}` | admin | Grant model access |
| `DELETE /api/v1/users/{id}/models/{model}` | admin | Revoke model access |
| `PATCH /api/v1/users/{id}/quota` | admin | Update token quotas |
| `GET /api/v1/access/check?user=&model=` | — | Pre-flight access check |
| `GET /api/v1/auth/login` (optional) | — | OIDC sign-in |
| `GET /api/v1/auth/callback` (optional) | — | OIDC callback |
| `GET /healthz`, `/readyz` | — | Liveness, readiness |

### Discovery (`sovstack discovery`)

Lists running model containers via Docker. No auth, no state.

| Endpoint | Purpose |
|----------|---------|
| `GET /api/v1/models/running` | Inventory |
| `GET /api/v1/health` | Legacy liveness (prefer `/healthz`) |
| `GET /healthz`, `/readyz` | Liveness, readiness |

### Metrics-proxy (`sovstack metrics-proxy`)

Resolves a model name to a container port and proxies the vLLM
`/metrics` endpoint. No auth, no state.

| Endpoint | Purpose |
|----------|---------|
| `GET /api/v1/models/{name}/metrics` | vLLM Prometheus metrics |
| `GET /healthz`, `/readyz` | Liveness, readiness |

### Legacy management (`sovstack management`)

Mounts all three subservices on port 8888 — the pre-split form, kept as
a backward-compat shim. Prints a deprecation warning on startup.

## Data

### `keys.json`

The single source of truth for users. Lives at
`~/.sovereignstack/keys.json` (mode 0600) by default; only the `policy`
service writes it. Format (illustrative; encrypted fields show their
on-disk form):

```json
{
  "users": {
    "alice": {
      "id":          "alice",
      "key":         "$argon2id$v=19$m=65536,t=2,p=2$<salt>$<hash>",
      "key_index":   "x4VmKJh+YCo",
      "department":  "$enc1$<base64>",
      "team":        "$enc1$<base64>",
      "role":        "$enc1$<base64>",
      "allowed_models":     ["mistral-7b"],
      "rate_limit_per_min": 100,
      "max_tokens_per_day": 500000,
      "max_tokens_per_month": 10000000,
      "ip_allowlist":  null,
      "created_at":    "2026-04-30T10:00:00Z",
      "last_used_at":  "2026-05-07T15:32:00Z"
    }
  }
}
```

### Audit log

Two tiers, written in parallel by the gateway:

1. **Hot path** — encrypted SQLite at `~/.sovereignstack/sovstack-audit.db`.
   Queryable; pruned by `audit.retention_days`.
2. **Cold path** (optional) — daily-rotated JSONL files in
   `audit.jsonl_dir`. Yesterday's file is auto-gzipped. Operator
   manages long-term retention via `logrotate`.

### Quota counters

Backed by one of three implementations selected by `quota.backend`:

- `memory` — in-process; lost on restart. Dev only.
- `sqlite` — local file; survives restart. Single-instance prod.
- `redis` — shared across replicas. **Required for multi-gateway HA.**

Counters reset at UTC midnight (daily) and on the 1st of each month
(monthly). Resets are computed lazily on read; no scheduled job.

## Request flow (end-to-end)

A single chat-completions call exercises every layer:

1. Client sends `POST /v1/models/mistral-7b/chat/completions` + `X-API-Key`
2. Gateway: argon2 verify → resolves `userID = alice`
3. Gateway: alice's `allowed_models` includes `mistral-7b` → pass
4. Gateway: alice has quota remaining → pass
5. Gateway: token-bucket has tokens → consume one
6. Gateway: `discovery` registry says `mistral-7b` runs on port 8000 → rewrite path
7. Gateway: forwards to `localhost:8000`, captures status (200) and 842ms duration
8. Gateway: reads `usage.completion_tokens` from response, increments daily/monthly
9. Gateway: bumps `gateway_requests_total{user=alice,model=mistral-7b,status=200}` etc.
10. Gateway: writes audit row to SQLite with correlation ID

## Two metric layers

| Layer | Where | What it tells you |
|-------|-------|-------------------|
| **Gateway** | `gateway:8001/metrics` | Who used what, how much, how fast |
| **Model worker** | `metrics-proxy:8890/api/v1/models/{name}/metrics` | How the inference engine is doing |

You'll usually want both in Prometheus. Cross-layer questions ("Alice's
P99 is high *and* GPU cache is at 95% → it's GPU, not gateway") are why
they're separate but adjacent.

## What's NOT here

- **No proxy of vLLM payloads through anything but the gateway.** The
  gateway is the only request path; nothing else sees the prompt or
  response bytes.
- **No shared database between gateway and policy.** They synchronise
  via `keys.json` reads (gateway) and HTTP API (policy → gateway only
  for model-router refresh).
- **No mTLS between split services** (yet). Same-host or trusted
  network. Mutual TLS for service-to-service auth is a future addition.

## See also

- [Phase A — Foundations](PHASE_A_FOUNDATIONS.md) for config, logging, versioning
- [Phase C — Security at rest](PHASE_C_SECURITY.md) for the threat model
- [Phase D — Trust boundary](PHASE_D_TRUST_BOUNDARY.md) for the OSS / commercial split
- [Phase E — Management split](PHASE_E_MANAGEMENT_SPLIT.md) for the three-service rationale
