# SovereignStack Scripts

Helper scripts for running the SovereignStack OSS stack locally.

| Script | What it does | When to use |
|---|---|---|
| `start-stack.sh` | Native (no-Docker) full OSS startup: builds, seeds a demo user, runs the four OSS services (`policy`, `discovery`, `metrics-proxy`, `gateway`), prints every endpoint and the demo key. Supports `--stop` to terminate a previous run. | Day-to-day local development with a Go toolchain installed |
| `start-stack-docker.sh` | Same as above but runs all four services in containers via `docker-compose.yml`. Supports `--stop` (preserve state) and `--down` (remove containers). | When you don't want a Go toolchain on the host, or want to mirror a production-like container environment |
| `admission-smoke.sh` | Deterministic 503 demo for the host-aware admission controller (fake discovery + metrics + curl) | Verifying admission guardrails after changes (no GPU needed) |

For the **full commercial stack** (OSS + visibility backend + Next.js dashboard) use `../start-demo.sh` at the workspace root.

---

## start-stack.sh

**Purpose:** Native local startup of the full OSS stack — builds `sovstack`, seeds a demo user with `*` model access and configurable token quota, starts the four services (`policy`, `discovery`, `metrics-proxy`, `gateway`) in the background, waits for each `/healthz` probe, and prints a banner with every endpoint plus the demo + admin keys. Ctrl+C tears everything down via the cleanup trap.

This is the OSS-only counterpart to the workspace-root `start-demo.sh` (which adds the commercial visibility backend and Next.js dashboard).

**Usage:**

```bash
# Standard: all four services, no monitoring
./scripts/start-stack.sh

# Also bring up Prometheus + Grafana via docker-compose
./scripts/start-stack.sh --with-monitoring

# Reuse the existing ./bin/sovstack (skip rebuild)
./scripts/start-stack.sh --skip-build

# Keep an existing demo user / API key (don't recreate)
./scripts/start-stack.sh --no-seed

# Stop a previous run (reads .stack-pids written at startup)
./scripts/start-stack.sh --stop

# Show help
./scripts/start-stack.sh --help
```

The script writes `.stack-pids` when it starts services in the background; `--stop` reads that file and sends SIGTERM (escalating to SIGKILL after a short window). If you ran it in the foreground, Ctrl+C does the same via the cleanup trap.

**Env overrides:** `MANAGEMENT_PORT`, `GATEWAY_PORT`, `DEMO_USER`, `DEMO_MODEL_ALLOW`, `DAILY_QUOTA`, `MONTHLY_QUOTA`, `SOVSTACK_ADMIN_KEY`. Defaults match the demo workflow.

**Sample banner:**

```
──────────────────────────────────────────────────────────────────────
  SovereignStack OSS is running locally
──────────────────────────────────────────────────────────────────────

  Gateway              http://localhost:8001
    /healthz             liveness probe
    /readyz              readiness probe
    /metrics             Prometheus exposition
    /v1/...              OpenAI-compatible proxy
    /api/v1/audit/logs   recent audit records

  Policy               http://localhost:8888
    /api/v1/users                  list users (admin)
    /api/v1/users/{id}/quota       update quota (PATCH, admin)
    /api/v1/access/check           pre-flight access check

  Discovery            http://localhost:8889
    /api/v1/models/running         inventory of running models

  Metrics-proxy        http://localhost:8890
    /api/v1/models/{name}/metrics  proxied vLLM Prometheus metrics

  Demo user              demo
  Demo API key           sk_…
  Admin Bearer           sk_admin_…

  Try it:
    # Routing path: /models/<name>/v1/...  (NOT /v1/models/<name>/...)
    curl -H "X-API-Key: sk_…" \
         -d '{"messages":[{"role":"user","content":"hi"}]}' \
         http://localhost:8001/models/<model>/v1/chat/completions

  Press Ctrl+C to stop.
```

The script detects port conflicts on `8001`, `8888`, `8889`, and `8890` up-front so it fails fast with a helpful pointer (e.g. "is a `sovereignstack-*` container holding it?") instead of dying inside `bind: address already in use`. Logs go to `logs/{policy,discovery,metrics-proxy,gateway}.log`.

---

## start-stack-docker.sh

**Purpose:** Container-mode counterpart to `start-stack.sh`. Brings up all four OSS services (`policy`, `discovery`, `metrics-proxy`, `gateway`) in containers via `docker-compose.yml`, optionally adds Prometheus / Grafana, seeds the demo user inside the policy container so its API key works through the gateway, and prints the same endpoint banner. Useful when you don't want a Go toolchain on the host or you want to mirror a production-like container layout.

**Usage:**

```bash
# Standard: all four services in containers
./scripts/start-stack-docker.sh

# Also bring up Prometheus + Grafana
./scripts/start-stack-docker.sh --with-monitoring

# Force rebuild of the sovereignstack image
./scripts/start-stack-docker.sh --build

# Don't (re-)create the demo user
./scripts/start-stack-docker.sh --no-seed

# Stop services but keep containers (state preserved, fast restart)
./scripts/start-stack-docker.sh --stop

# Stop AND remove containers (full teardown; volumes for prom/grafana persist)
./scripts/start-stack-docker.sh --down

# Show help
./scripts/start-stack-docker.sh --help
```

The wrapper waits for each container's `HEALTHCHECK` to report `healthy` before seeding the demo user (Docker reports per-container health via `docker compose ps --format json`, polled until ready or 30s timeout).

**`.env` is auto-managed.** On every run the script writes `USER_UID` / `USER_GID` / `DOCKER_GID` to `.env` (already gitignored), so direct `docker compose ...` invocations without going through this script also pick up the right host UIDs and the correct docker socket group. Without it, `docker compose up -d` from a fresh shell silently uses 1000:1000 + DOCKER_GID=999 and `discovery` / `metrics-proxy` lose access to the docker daemon.

**Why two scripts and not one with a flag?** Day-to-day flow is short enough that one file per mode is easier to read top-to-bottom than a single script with branching for native vs container plumbing. Both share the demo-user defaults (`DEMO_USER`, `DAILY_QUOTA`, etc.) and the banner shape, so switching between them is muscle-memory.

---

## admission-smoke.sh

**Purpose:** End-to-end smoke test for the host-aware admission controller. Spins up a fake management server pinning kv-cache usage at 99%, starts the gateway pointed at it, sends a chat-completion request, and verifies the controller rejects with `503 Service Unavailable` + `Retry-After`. No GPU or real model required.

**Usage:**

```bash
# Run the full check (~10 seconds)
./scripts/admission-smoke.sh

# Leave processes running after the test for manual inspection
./scripts/admission-smoke.sh --keep
```

**What it verifies:**
- Gateway boots with admission controller wired in
- Controller polls the management metrics-proxy and ingests samples
- A request to a "saturated" model returns HTTP 503
- Response carries a `Retry-After` header
- Response body carries the admission reason (e.g. `"kv-cache 99.0% >= hard cap 65.0%"`)
- `gateway_admission_shed_total` counter increments on `/metrics`
- `gateway_admission_shed_by_model{model="..."}` per-model breakdown appears

**Exit codes:**
- `0` — all checks passed
- `1` — preflight (build / missing tool) failed
- `2` — fake management did not come up
- `3` — gateway did not come up
- `4` — request was admitted instead of rejected
- `5` — `Retry-After` header missing
- `6` — metrics counter not incremented

**Requires:** `python3`, `go`, `curl`. Uses ports `18001` (gateway) and `18888` (fake management) so it doesn't collide with `start-demo.sh` or a running stack.

---

## Documentation

For more detail see:

- [DOCKER_COMPOSE_SERVICES.md](../docs/DOCKER_COMPOSE_SERVICES.md) — details on all Docker services
- [MANAGEMENT_API.md](../docs/MANAGEMENT_API.md) — API endpoint documentation
- [MAIN_STACK_INTEGRATION.md](../docs/MAIN_STACK_INTEGRATION.md) — visibility platform integration
