#!/bin/bash

# Admission controller smoke test
#
# Spins up a fake management server that pins kv-cache usage at 99%,
# starts the gateway pointed at it, sends a chat-completion request, and
# verifies the admission controller rejects with 503 + Retry-After. No
# GPU or real model required — useful when the unit tests aren't enough
# but a full ./start-demo.sh is overkill.
#
# Usage: ./scripts/admission-smoke.sh
#        ./scripts/admission-smoke.sh --keep   # leave processes running for inspection
#
# Exit codes:
#   0  all checks passed
#   1  build failed
#   2  fake management did not come up
#   3  gateway did not come up
#   4  request was admitted instead of rejected
#   5  Retry-After header missing
#   6  metrics counter not incremented

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

GW_PORT=18001
MGMT_PORT=18888
MODEL_NAME="smoke-test-model"
API_KEY="sk_test_123"
TMPDIR="${TMPDIR:-/tmp}"
KEEP_RUNNING=false

for arg in "$@"; do
  case "$arg" in
    --keep) KEEP_RUNNING=true ;;
    *) echo "unknown arg: $arg"; exit 1 ;;
  esac
done

# ─── Colors / logging ─────────────────────────────────────────────────────────
log()  { printf "\033[1;36m[smoke]\033[0m %s\n" "$*"; }
ok()   { printf "\033[1;32m[ ok  ]\033[0m %s\n" "$*"; }
fail() { printf "\033[1;31m[fail]\033[0m %s\n" "$*"; exit "${2:-1}"; }

cleanup() {
  if [ "$KEEP_RUNNING" = "true" ]; then
    log "leaving processes running (--keep). To stop:"
    [ -n "${MGMT_PID:-}" ] && echo "  kill $MGMT_PID  # fake management"
    [ -n "${GW_PID:-}" ]   && echo "  kill $GW_PID    # gateway"
    return
  fi
  log "tearing down"
  [ -n "${MGMT_PID:-}" ] && kill "$MGMT_PID" 2>/dev/null || true
  [ -n "${GW_PID:-}" ]   && kill "$GW_PID"   2>/dev/null || true
  wait 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# ─── Preflight ────────────────────────────────────────────────────────────────
command -v python3 >/dev/null || fail "python3 required (used to fake the management API)" 1
command -v go      >/dev/null || fail "go required to build the gateway" 1
command -v curl    >/dev/null || fail "curl required" 1

# ─── 1. Fake management server (kv-cache pinned at 99%) ──────────────────────
log "starting fake management on :$MGMT_PORT (kv-cache pinned at 99%)"

# A self-contained Python HTTP server. We pass MGMT_PORT and MODEL_NAME via
# env vars rather than heredoc interpolation so $-signs in Python source
# don't get mangled by the shell.
MGMT_PORT="$MGMT_PORT" MODEL_NAME="$MODEL_NAME" \
python3 - >"$TMPDIR/admission-smoke-mgmt.log" 2>&1 <<'PYEOF' &
import http.server, json, os, sys

PORT  = int(os.environ["MGMT_PORT"])
MODEL = os.environ["MODEL_NAME"]

# Pinned saturation: 99% kv-cache, 50 in-flight, 200 queued. The admission
# controller's hard cap (default 95%) should reject every incoming request.
METRICS = (
    b"# HELP vllm:gpu_cache_usage_perc kv-cache usage\n"
    b"# TYPE vllm:gpu_cache_usage_perc gauge\n"
    b"vllm:gpu_cache_usage_perc 0.99\n"
    b"vllm:num_requests_running 50\n"
    b"vllm:num_requests_waiting 200\n"
)

class H(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a, **k):
        pass  # quiet; the gateway log is what matters

    def _send(self, code, body, ctype="application/json"):
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/api/v1/models/running":
            body = json.dumps([{"name": MODEL, "status": "running"}]).encode()
            self._send(200, body)
        elif self.path == f"/api/v1/models/{MODEL}/metrics":
            self._send(200, METRICS, ctype="text/plain; version=0.0.4")
        elif self.path in ("/healthz", "/readyz"):
            self._send(200, b'{"status":"ok"}')
        else:
            self._send(404, b'{"error":"not found"}')

http.server.HTTPServer(("127.0.0.1", PORT), H).serve_forever()
PYEOF
MGMT_PID=$!

# Wait for fake mgmt to come up.
for _ in $(seq 1 25); do
  if curl -sf "http://127.0.0.1:$MGMT_PORT/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
done
if ! curl -sf "http://127.0.0.1:$MGMT_PORT/healthz" >/dev/null 2>&1; then
  fail "fake management did not come up; see $TMPDIR/admission-smoke-mgmt.log" 2
fi
ok "fake management ready (pid $MGMT_PID)"

# ─── 2. Build sovstack and start the gateway pointed at the fake mgmt ────────
log "building sovstack"
( cd "$PROJECT_ROOT" && go build -o "$TMPDIR/sovstack-smoke" . ) \
  >"$TMPDIR/admission-smoke-build.log" 2>&1 \
  || fail "go build failed; see $TMPDIR/admission-smoke-build.log" 1
ok "build ok"

log "starting gateway on :$GW_PORT (poll=1s, hard=95%, no rate limit, plain HTTP)"
# Gateway exposes the TLS-disable knob via env (SOVSTACK_INSECURE_HTTP) and
# YAML (tls.insecure_http) only — there is no --insecure-http CLI flag, so
# we pass it through the env to keep this script self-contained.
SOVSTACK_INSECURE_HTTP=true \
"$TMPDIR/sovstack-smoke" gateway \
    --port "$GW_PORT" \
    --backend "http://127.0.0.1:9999" \
    --discovery-url "http://127.0.0.1:$MGMT_PORT" \
    --rate-limit 0 \
    --admission-poll-seconds 1 \
    >"$TMPDIR/admission-smoke-gateway.log" 2>&1 &
GW_PID=$!

for _ in $(seq 1 30); do
  if curl -sf "http://127.0.0.1:$GW_PORT/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
done
if ! curl -sf "http://127.0.0.1:$GW_PORT/healthz" >/dev/null 2>&1; then
  fail "gateway did not come up; see $TMPDIR/admission-smoke-gateway.log" 3
fi

# Allow at least one admission poll cycle to land per-model metrics in state.
sleep 2
ok "gateway ready (pid $GW_PID), admission poller has fetched at least once"

# ─── 3. POST a chat completion — expect 503 from admission shed ──────────────
log "POST /v1/models/$MODEL_NAME/chat/completions — expecting 503"

BODY_FILE="$TMPDIR/admission-smoke-resp-body"
HEAD_FILE="$TMPDIR/admission-smoke-resp-head"

STATUS=$(curl -s -o "$BODY_FILE" -D "$HEAD_FILE" -w "%{http_code}" \
    -H "X-API-Key: $API_KEY" \
    -H "Content-Type: application/json" \
    -X POST \
    -d '{"messages":[{"role":"user","content":"hi"}]}' \
    "http://127.0.0.1:$GW_PORT/v1/models/$MODEL_NAME/chat/completions")

if [ "$STATUS" != "503" ]; then
  log "response body:"; sed 's/^/  /' "$BODY_FILE"
  log "response headers:"; sed 's/^/  /' "$HEAD_FILE"
  fail "expected status 503 (admission shed), got $STATUS" 4
fi
ok "got 503 from gateway"

# Verify Retry-After header (case-insensitive grep).
if grep -qi '^retry-after:' "$HEAD_FILE"; then
  RA=$(grep -i '^retry-after:' "$HEAD_FILE" | head -1 | tr -d '\r')
  ok "$RA"
else
  fail "Retry-After header missing — request was rejected but client has no backoff hint" 5
fi

# Verify the body carries the admission reason for downstream debugging.
if grep -q 'service_unavailable' "$BODY_FILE"; then
  ok "body shape: $(cat "$BODY_FILE")"
fi

# ─── 4. Confirm gateway_admission_shed_total incremented ─────────────────────
SHED=$(curl -s "http://127.0.0.1:$GW_PORT/metrics" \
       | grep -E '^gateway_admission_shed_total ' \
       | awk '{print $2}' || true)

if [ -z "${SHED:-}" ]; then
  fail "gateway_admission_shed_total counter is missing from /metrics" 6
fi

if [ "$SHED" -ge 1 ] 2>/dev/null; then
  ok "gateway_admission_shed_total = $SHED"
else
  fail "gateway_admission_shed_total = $SHED (expected >= 1)" 6
fi

PER_MODEL=$(curl -s "http://127.0.0.1:$GW_PORT/metrics" \
            | grep -E '^gateway_admission_shed_by_model\{' || true)
if [ -n "$PER_MODEL" ]; then
  ok "per-model breakdown: $PER_MODEL"
fi

echo ""
ok "all checks passed — admission controller is rejecting under simulated saturation"
echo ""
echo "  Logs (kept for inspection):"
echo "    $TMPDIR/admission-smoke-gateway.log"
echo "    $TMPDIR/admission-smoke-mgmt.log"
echo "    $TMPDIR/admission-smoke-build.log"
