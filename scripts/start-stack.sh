#!/bin/bash

# SovereignStack OSS — local stack startup
#
# Builds the sovstack binary, seeds a demo user, starts the management API
# and the gateway in the background, waits for both to report healthy, and
# prints a banner with every endpoint and the demo API key. Ctrl+C tears
# everything down via the cleanup trap.
#
# This is the OSS-only counterpart to the workspace-root start-demo.sh
# (which adds the commercial visibility backend and Next.js dashboard).
#
# Usage:
#   ./scripts/start-stack.sh                # management + gateway only
#   ./scripts/start-stack.sh --with-monitoring  # also bring up prom/grafana via docker-compose
#   ./scripts/start-stack.sh --skip-build   # reuse existing ./bin/sovstack
#   ./scripts/start-stack.sh --no-seed      # don't (re-)create the demo user
#   ./scripts/start-stack.sh --help

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

BIN="$PROJECT_ROOT/bin/sovstack"
LOG_DIR="$PROJECT_ROOT/logs"
PIDS_FILE="$PROJECT_ROOT/.stack-pids"

DEMO_USER="${DEMO_USER:-demo}"
DEMO_MODEL_ALLOW="${DEMO_MODEL_ALLOW:-*}"
DAILY_QUOTA="${DAILY_QUOTA:-500000}"
MONTHLY_QUOTA="${MONTHLY_QUOTA:-10000000}"

MGMT_PORT="${MANAGEMENT_PORT:-8888}"
GW_PORT="${GATEWAY_PORT:-8001}"

ADMIN_KEY="${SOVSTACK_ADMIN_KEY:-sk_admin_$(openssl rand -hex 8 2>/dev/null || echo dev)}"
KEYS_FILE="$HOME/.sovereignstack/keys.json"

WITH_MONITORING=false
SKIP_BUILD=false
SKIP_SEED=false

# ─── Colors ───────────────────────────────────────────────────────────────────
BLUE='\033[1;36m'
GREEN='\033[1;32m'
YELLOW='\033[1;33m'
RED='\033[1;31m'
NC='\033[0m'

log()   { printf "${BLUE}[stack]${NC} %s\n" "$*"; }
ok()    { printf "${GREEN}[ ok  ]${NC} %s\n" "$*"; }
warn()  { printf "${YELLOW}[warn]${NC} %s\n" "$*"; }
die()   { printf "${RED}[fail]${NC} %s\n" "$*"; exit 1; }

show_help() {
  cat <<EOF
SovereignStack OSS — local stack startup

Brings up the management API and gateway natively (no Docker required for
the OSS pieces themselves) and prints every endpoint plus a demo API key.

Usage: $0 [OPTIONS]

Options:
  --with-monitoring   Also bring up Prometheus + Grafana via docker-compose
                      (containers: sovereignstack-prometheus, -grafana,
                      -node-exporter, -cadvisor)
  --skip-build        Reuse the existing ./bin/sovstack instead of rebuilding
  --no-seed           Don't create / refresh the demo user
  -h, --help          Show this help

Env overrides (defaults shown):
  MANAGEMENT_PORT=$MGMT_PORT       Port the management API listens on
  GATEWAY_PORT=$GW_PORT       Port the gateway listens on
  DEMO_USER=$DEMO_USER       Demo user id (used by sovstack keys add)
  DEMO_MODEL_ALLOW='*'       Models the demo user can access
  DAILY_QUOTA=$DAILY_QUOTA      Demo user daily token quota
  MONTHLY_QUOTA=$MONTHLY_QUOTA   Demo user monthly token quota
  SOVSTACK_ADMIN_KEY         Admin Bearer for management API; auto-generated if unset

Press Ctrl+C to stop everything.
EOF
}

for arg in "$@"; do
  case "$arg" in
    --with-monitoring) WITH_MONITORING=true ;;
    --skip-build)      SKIP_BUILD=true ;;
    --no-seed)         SKIP_SEED=true ;;
    -h|--help)         show_help; exit 0 ;;
    *)                 echo "unknown arg: $arg"; show_help; exit 1 ;;
  esac
done

# ─── Cleanup trap ─────────────────────────────────────────────────────────────
# Two-tier teardown: kill recorded PIDs first (clean), then escalate to a
# SIGTERM-then-SIGKILL of the whole process group as a safety net so we
# never leak a sovstack process when SIGINT propagation is flaky (e.g. when
# the script is invoked from a non-interactive shell or via `& kill -INT`).
cleanup() {
  echo ""
  log "shutting down"
  if [ -f "$PIDS_FILE" ]; then
    while read -r pid name; do
      [ -n "${pid:-}" ] && kill "$pid" 2>/dev/null || true
    done < "$PIDS_FILE"
    rm -f "$PIDS_FILE"
  fi
  if [ "$WITH_MONITORING" = true ]; then
    ( cd "$PROJECT_ROOT" && docker compose stop prometheus grafana node-exporter cadvisor 2>/dev/null || true )
  fi
  # Wait briefly for graceful shutdown, then escalate.
  sleep 1
  pkill -TERM -P $$ 2>/dev/null || true
  sleep 1
  pkill -KILL -P $$ 2>/dev/null || true
  log "done"
}
trap cleanup EXIT INT TERM

# ─── Preflight ────────────────────────────────────────────────────────────────
command -v go    >/dev/null || die "go required"
command -v curl  >/dev/null || die "curl required"

# Detect port conflicts up-front so we fail with a useful message instead
# of inside `bind: address already in use` deep in the binary.
check_port_free() {
  local port="$1" name="$2"
  if (command -v ss >/dev/null && ss -tnlH "( sport = :$port )" 2>/dev/null | grep -q LISTEN) \
     || (command -v lsof >/dev/null && lsof -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1); then
    echo ""
    warn "port $port ($name) is already in use"
    echo "  Likely culprits:"
    echo "    docker ps               # is a sovereignstack-* container holding it?"
    echo "    docker compose stop management gateway"
    echo "    lsof -iTCP:$port -sTCP:LISTEN"
    die "free port $port and re-run, or override with ${name^^}_PORT=<port>"
  fi
}
check_port_free "$MGMT_PORT" management
check_port_free "$GW_PORT"   gateway

mkdir -p "$LOG_DIR" "$(dirname "$KEYS_FILE")" "$PROJECT_ROOT/bin"
: > "$PIDS_FILE"

# ─── 1. Build ─────────────────────────────────────────────────────────────────
if [ "$SKIP_BUILD" = true ] && [ -x "$BIN" ]; then
  ok "reusing existing $BIN"
else
  log "building sovstack → $BIN"
  ( cd "$PROJECT_ROOT" && go build -o "$BIN" . ) || die "build failed"
  ok "build ok"
fi

# ─── 2. Seed the demo user ────────────────────────────────────────────────────
DEMO_KEY=""
if [ "$SKIP_SEED" = true ]; then
  log "skipping demo user seed (--no-seed)"
else
  log "seeding demo user '$DEMO_USER'"
  # Best-effort cleanup of a previous demo user; failures are silent because
  # the keystore may not exist yet on a fresh install.
  "$BIN" keys remove "$DEMO_USER" >/dev/null 2>&1 || true
  ADD_OUT="$("$BIN" keys add "$DEMO_USER" --department demo --team demo --role analyst 2>&1)"
  echo "$ADD_OUT" | grep -E '^\s*(API Key|User ID):' || true
  DEMO_KEY="$(echo "$ADD_OUT" | awk '/API Key:/ {print $3; exit}')"
  [ -z "$DEMO_KEY" ] && warn "could not capture demo key; check 'sovstack keys add' output"

  "$BIN" keys grant-model "$DEMO_USER" "$DEMO_MODEL_ALLOW" >/dev/null 2>&1 || \
    warn "grant-model failed (model whitelist may be empty)"
  "$BIN" keys set-quota "$DEMO_USER" --daily "$DAILY_QUOTA" --monthly "$MONTHLY_QUOTA" >/dev/null 2>&1 || \
    warn "set-quota failed"
  ok "demo user ready"
fi

# ─── 3. Start management ──────────────────────────────────────────────────────
log "starting management on :$MGMT_PORT → $LOG_DIR/management.log"
SOVSTACK_INSECURE_HTTP=true \
"$BIN" management \
    --port "$MGMT_PORT" \
    --keys "$KEYS_FILE" \
    --admin-key "$ADMIN_KEY" \
    >"$LOG_DIR/management.log" 2>&1 &
MGMT_PID=$!
echo "$MGMT_PID management" >> "$PIDS_FILE"

for _ in $(seq 1 30); do
  if curl -sf "http://127.0.0.1:$MGMT_PORT/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done
curl -sf "http://127.0.0.1:$MGMT_PORT/healthz" >/dev/null 2>&1 \
  || die "management did not come up on :$MGMT_PORT — see $LOG_DIR/management.log"
ok "management ready (pid $MGMT_PID)"

# ─── 4. Start gateway ─────────────────────────────────────────────────────────
log "starting gateway on :$GW_PORT → $LOG_DIR/gateway.log"
SOVSTACK_INSECURE_HTTP=true \
"$BIN" gateway \
    --port "$GW_PORT" \
    --keys "$KEYS_FILE" \
    --management-url "http://127.0.0.1:$MGMT_PORT" \
    --rate-limit 100 \
    >"$LOG_DIR/gateway.log" 2>&1 &
GW_PID=$!
echo "$GW_PID gateway" >> "$PIDS_FILE"

for _ in $(seq 1 30); do
  if curl -sf "http://127.0.0.1:$GW_PORT/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done
curl -sf "http://127.0.0.1:$GW_PORT/healthz" >/dev/null 2>&1 \
  || die "gateway did not come up on :$GW_PORT — see $LOG_DIR/gateway.log"
ok "gateway ready (pid $GW_PID)"

# ─── 5. Optional monitoring stack ─────────────────────────────────────────────
if [ "$WITH_MONITORING" = true ]; then
  log "bringing up monitoring stack (Prometheus, Grafana, node-exporter, cAdvisor)"
  ( cd "$PROJECT_ROOT" && docker compose up -d prometheus grafana node-exporter cadvisor ) \
    || warn "monitoring stack failed to come up; continuing without it"
fi

# ─── 6. Banner ────────────────────────────────────────────────────────────────
cat <<EOF

──────────────────────────────────────────────────────────────────────
  SovereignStack OSS is running locally
──────────────────────────────────────────────────────────────────────

  Gateway              http://localhost:$GW_PORT
    /healthz             liveness probe (200 if process is up)
    /readyz              readiness probe (200 if upstreams are reachable)
    /metrics             Prometheus exposition (request / latency / quota counters)
    /v1/...              OpenAI-compatible proxy (POST chat / completions)
    /api/v1/audit/logs   recent audit records (admin)
    /api/v1/audit/stats  audit summary

  Management API       http://localhost:$MGMT_PORT
    /healthz             liveness probe
    /readyz              readiness probe
    /api/v1/models/running                         list running model containers
    /api/v1/models/{name}/metrics                  proxied vLLM /metrics
    /api/v1/users                                  list users (admin)
    /api/v1/users/{id}                             single user
    /api/v1/users/{id}/models/{model}              grant/revoke (POST/DELETE, admin)
    /api/v1/users/{id}/quota                       update quota (PATCH, admin)
    /api/v1/access/check?user=...&model=...        pre-flight access check
EOF

if [ "$WITH_MONITORING" = true ]; then
  cat <<EOF

  Monitoring (containers)
    Prometheus           http://localhost:9090
    Grafana              http://localhost:3001  (admin / admin)
    node-exporter        http://localhost:9100/metrics
    cAdvisor             http://localhost:8080
EOF
fi

if [ -n "$DEMO_KEY" ]; then
  cat <<EOF

  Demo user              $DEMO_USER
  Demo API key           $DEMO_KEY
  Admin Bearer           $ADMIN_KEY
EOF
fi

cat <<EOF

  Logs                   $LOG_DIR/{management,gateway}.log

  Try it:
EOF

if [ -n "$DEMO_KEY" ]; then
cat <<EOF
    # Liveness check
    curl -s http://localhost:$GW_PORT/healthz

    # Read the audit summary
    curl -s http://localhost:$GW_PORT/api/v1/audit/stats | jq .

    # Send a request (after deploying a model, e.g. distilbert-base-uncased)
    $BIN deploy distilbert-base-uncased --type cpu     # in another shell, optional
    curl -H "X-API-Key: $DEMO_KEY" \\
         -d '{"messages":[{"role":"user","content":"hi"}]}' \\
         http://localhost:$GW_PORT/v1/models/distilbert-base-uncased/chat/completions
EOF
else
cat <<EOF
    curl -s http://localhost:$GW_PORT/healthz
    curl -s http://localhost:$MGMT_PORT/api/v1/models/running | jq .
EOF
fi

cat <<EOF

  Press Ctrl+C to stop.
──────────────────────────────────────────────────────────────────────
EOF

# Block on the background processes so Ctrl+C gets handled by the trap.
wait
