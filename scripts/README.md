# SovereignStack Scripts

Helper scripts for managing the SovereignStack main stack.

| Script | What it does | When to use |
|---|---|---|
| `start-stack.sh` | Native (no-Docker) full OSS startup: builds, seeds a demo user, runs management + gateway, prints every endpoint and the demo key. Supports `--stop` to terminate a previous run. | Day-to-day local development with a Go toolchain installed |
| `start-stack-docker.sh` | Same as above but runs both services in containers via `docker-compose.yml`. Supports `--stop` (preserve state) and `--down` (remove containers). | When you don't want a Go toolchain on the host, or want to mirror a production-like container environment |
| `start-management.sh` | Container-only quick-start for **just** the management API in Docker | Wiring the visibility backend or other clients to a stable management URL without running the gateway |
| `admission-smoke.sh` | Deterministic 503 demo for the host-aware admission controller (fake mgmt + curl) | Verifying admission guardrails after changes (no GPU needed) |

For the **full commercial stack** (OSS + visibility backend + Next.js dashboard) use `../start-demo.sh` at the workspace root.

## Available Scripts

### start-stack.sh

**Purpose:** Native local startup of the full OSS stack — builds `sovstack`, seeds a demo user with `*` model access and configurable token quota, starts the management API and the gateway in the background, waits for both `/healthz` probes, and prints a banner with every endpoint plus the demo + admin keys. Ctrl+C tears everything down via the cleanup trap.

This is the OSS-only counterpart to the workspace-root `start-demo.sh` (which adds the commercial visibility backend and Next.js dashboard).

**Usage:**

```bash
# Standard: management + gateway, no monitoring
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

The script writes `.stack-pids` when it starts services in the background;
`--stop` reads that file and sends SIGTERM (escalating to SIGKILL after a
short window). If you ran it in the foreground, Ctrl+C does the same via
the cleanup trap.

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

  Management API       http://localhost:8888
    /api/v1/models/running          list running model containers
    /api/v1/models/{name}/metrics   proxied vLLM /metrics
    /api/v1/users                   list users (admin)
    /api/v1/access/check            pre-flight access check

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

The script detects port conflicts on `8001` and `8888` up-front so it
fails fast with a helpful pointer (e.g. "is a `sovereignstack-*`
container holding it?") instead of dying inside `bind: address already
in use`. Logs go to `logs/management.log` and `logs/gateway.log`.

---

### start-stack-docker.sh

**Purpose:** Container-mode counterpart to `start-stack.sh`. Brings up
management + gateway in containers via `docker-compose.yml`, optionally
adds Prometheus / Grafana, seeds the demo user inside the management
container so its API key works through the gateway, and prints the same
endpoint banner. Useful when you don't want a Go toolchain on the host
or you want to mirror a production-like container layout.

**Usage:**

```bash
# Standard: management + gateway in containers
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

The wrapper waits for each container's `HEALTHCHECK` to report `healthy`
before seeding the demo user (Docker reports per-container health via
`docker compose ps --format json`, polled until ready or 30s timeout).

**Why two scripts and not one with a flag?** Day-to-day flow is short
enough that one file per mode is easier to read top-to-bottom than a
single script with branching for native vs container plumbing. Both
share the demo-user defaults (`DEMO_USER`, `DAILY_QUOTA`, etc.) and the
banner shape, so switching between them is muscle-memory.

---

### start-management.sh

**Purpose:** Container-only quick-start for **just** the management API in Docker. Useful when you want a stable container running the management surface (e.g. for the visibility backend to point at) without bringing up the gateway. For full OSS startup, prefer `start-stack.sh` above.

**Features:**
- Automatic environment setup (.env creation)
- Docker and docker-compose validation
- Health check verification
- Automatic image building (with optional force rebuild)
- Real-time logging option
- Status checking and health monitoring

**Usage:**

```bash
# Make script executable (first time only)
chmod +x scripts/start-management.sh

# Start with cached Docker image (fastest)
./scripts/start-management.sh

# Start with fresh rebuild
./scripts/start-management.sh --build

# Start and watch logs
./scripts/start-management.sh --logs

# Check if it's running
./scripts/start-management.sh --status

# Check API health
./scripts/start-management.sh --health

# Stop the container
./scripts/start-management.sh --stop

# Show help
./scripts/start-management.sh --help
```

**Endpoints exposed by the management container:**

```
Health:         http://localhost:8888/healthz
Readiness:      http://localhost:8888/readyz
Running Models: http://localhost:8888/api/v1/models/running
Users (admin):  http://localhost:8888/api/v1/users
```

**Test it:**

```bash
curl http://localhost:8888/healthz
curl http://localhost:8888/api/v1/models/running | jq .
```

**Aliases (Optional)**

Add to your shell profile (`.bash_profile`, `.zshrc`, etc.) for easy access:

```bash
# In ~/.zshrc or ~/.bash_profile
alias ss-mgmt='~/Projects/sstack/sovereignstack/scripts/start-management.sh'

# Then use:
ss-mgmt                # Start
ss-mgmt --build --logs # Rebuild and watch logs
ss-mgmt --status       # Check status
```

---

### admission-smoke.sh

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

## Quick Start

### 1. First Time Setup

```bash
cd ~/Projects/sstack/sovereignstack

# Make script executable
chmod +x scripts/start-management.sh

# Start the management API
./scripts/start-management.sh

# Script will automatically:
# - Create .env file from .env.example
# - Check Docker installation
# - Build the Docker image
# - Start the container
# - Verify health
```

### 2. Verify It's Running

```bash
# Option A: Use the script
./scripts/start-management.sh --status

# Option B: Direct commands
curl http://localhost:8888/healthz
curl http://localhost:8888/api/v1/models/running | jq .

# Option C: Docker commands
docker-compose ps management
docker-compose logs management
```

### 3. Integrate with Visibility Platform

Once management API is running on port 8888, the visibility platform can discover models:

```bash
cd ~/Projects/sstack/sovereignstack-visibility

# Ensure MAIN_STACK_API_URL is set correctly
cat .env | grep MAIN_STACK_API_URL
# Should be: MAIN_STACK_API_URL=http://localhost:8888

# Start visibility platform
docker compose up -d
```

## Advanced Usage

### Build Without Starting

```bash
# Just build the image
docker-compose build management

# Or with script
./scripts/start-management.sh --build
# (and then stop it with --stop if you don't want it running)
```

### View Logs

```bash
# Real-time logs
./scripts/start-management.sh --logs

# Or via docker-compose
docker-compose logs -f management

# Last 50 lines
docker-compose logs management | tail -50

# Filter for errors
docker-compose logs management | grep -i error
```

### Troubleshooting Commands

```bash
# Check if container is running
docker-compose ps management

# Inspect container
docker-compose exec management sh

# Restart container
docker-compose restart management

# Remove container and rebuild
docker-compose down management
./scripts/start-management.sh --build

# Check port usage
lsof -i :8888  # On macOS
netstat -tulpn | grep 8888  # On Linux
```

### Manual Docker Commands

```bash
# Alternative to script (if you prefer docker-compose directly)

# Start
docker-compose up -d management

# Stop
docker-compose down management

# Rebuild
docker-compose build --no-cache management

# View logs
docker-compose logs -f management

# Status
docker-compose ps management
```

## Configuration

### Environment Variables

The script reads from `.env` file. Key variables:

```bash
# Management API port (default: 8888)
MANAGEMENT_PORT=8888

# HuggingFace token (for model downloads)
HF_TOKEN=hf_xxxxxxxxxxxxx

# GPU configuration
CUDA_VISIBLE_DEVICES=0
```

### Changing the Port

```bash
# Edit .env
nano .env

# Change:
MANAGEMENT_PORT=8888  # → 9999

# Restart
./scripts/start-management.sh --build
```

## Integration with Other Services

### With Model Deployment

```bash
# Start management API
./scripts/start-management.sh

# In another terminal, deploy a model
./sovstack deploy distilbert-base-uncased --type cpu

# Management API immediately exposes it
curl http://localhost:8888/api/v1/models/running

# Visibility platform discovers it automatically
```

### With Monitoring Stack

```bash
# Start all services including Prometheus and Grafana
docker-compose up -d

# Then check dashboards
# - Prometheus: http://localhost:9090
# - Grafana: http://localhost:3000
```

## Performance Tips

### Speed Up Image Build

First build takes longer (~2-5 minutes). Subsequent builds use cache:

```bash
# Fast (uses cache): ~10 seconds
./scripts/start-management.sh

# Slow (rebuilds everything): ~3-5 minutes
./scripts/start-management.sh --build

# Very slow (no cache): ~5-10 minutes
docker-compose build --no-cache management
```

### Optimize Startup Time

```bash
# Pre-build image at setup time
./scripts/start-management.sh

# Then subsequent starts are instant
docker-compose up -d management
```

## Documentation

For more information, see:

- [DOCKER_COMPOSE_SERVICES.md](../docs/DOCKER_COMPOSE_SERVICES.md) - Details on all Docker services
- [MANAGEMENT_API.md](../docs/MANAGEMENT_API.md) - API endpoint documentation
- [MAIN_STACK_INTEGRATION.md](../docs/MAIN_STACK_INTEGRATION.md) - Visibility platform integration
