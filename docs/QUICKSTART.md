# Quickstart

Get a SovereignStack instance running in five minutes.

> Goal of this doc: a working `curl` against your own private LLM.
> Not goals: production hardening (covered in
> [PHASE_C_SECURITY.md](PHASE_C_SECURITY.md)) or HA topology
> (covered in [PHASE_G_OBSERVABILITY.md](PHASE_G_OBSERVABILITY.md)).

## Prerequisites

- **Linux** (any recent distro), **macOS**, or **Windows + WSL2**
- **Go 1.24+** for building (or use a release binary)
- **Docker** for running the model container
- **At least one model** — small ones (Phi-3, TinyLlama) work on CPU; for
  Mistral-7B and larger you'll want a GPU with 16GB+ VRAM
- ~8 GB free disk for model weights

## 1. Install

### From source

```bash
git clone https://github.com/sovereignstack/sovereignstack
cd sovereignstack
go install .
```

### Pre-built binary (when releases are available)

```bash
curl -sSL https://github.com/sovereignstack/sovereignstack/releases/latest/download/sovstack-linux-amd64.tar.gz \
  | tar -xz -C /usr/local/bin
sovstack --version
```

## 2. Initialize

```bash
sovstack init
```

This creates:

```
~/.sovereignstack/
├── keys.json           (mode 0600 — your user database)
├── master.key          (mode 0600 — AES-256 master for field encryption)
├── tls/
│   ├── cert.pem        (auto-generated self-signed; replace in production)
│   └── key.pem
└── sovstack.yaml       (default configuration)
```

## 3. Pull a model

```bash
sovstack pull mistral-7b      # ~14GB, requires GPU
# or for CPU-friendly testing:
sovstack pull TinyLlama-1.1B-Chat
```

This downloads weights into `~/.sovereignstack/models/` and registers a
Docker image. Verify:

```bash
sovstack models list
```

## 4. Create a user

```bash
sovstack keys add alice --department research

# Output:
# ✓ Created user "alice"
#   API Key: sk_xxxxxxxxxxxxxxxxxxxxxxxxxxxxx
#   Rate Limit: 100 requests/min
```

**Save the API key now.** It's hashed at rest immediately; the next
`keys list` won't show it.

## 5. Grant model access

```bash
sovstack keys grant-model alice mistral-7b
# or for all models:
sovstack keys grant-model alice "*"
```

## 6. Start the services

For a one-host dev setup, run all three subservices via the legacy
`management` shim plus the gateway:

```bash
# Terminal 1: management API
sovstack management \
  --keys ~/.sovereignstack/keys.json \
  --master-key-file ~/.sovereignstack/master.key

# Terminal 2: gateway
sovstack gateway \
  --keys ~/.sovereignstack/keys.json \
  --management-url http://localhost:8888
```

For the production-shaped split:

```bash
sovstack policy --master-key-file ~/.sovereignstack/master.key &
sovstack discovery &
sovstack metrics-proxy &
sovstack gateway --keys ~/.sovereignstack/keys.json
```

The gateway logs the TLS fingerprint on startup; pin it in your clients
if you're using the auto-generated self-signed cert.

## 7. Make a request

```bash
ALICE=sk_xxxxxxxxxxxxxxxxxxxxxxxxxxxxx   # from step 4

curl -k -H "X-API-Key: $ALICE" \
     -H "Content-Type: application/json" \
     -d '{"messages":[{"role":"user","content":"Say hi in one word."}]}' \
     https://localhost:8001/v1/models/mistral-7b/chat/completions
```

`-k` skips cert verification (fine for self-signed dev). For production,
use a real cert and drop the flag.

## 8. See what happened

```bash
# Prometheus metrics
curl -k https://localhost:8001/metrics | grep gateway_requests_total

# Audit log
sqlite3 ~/.sovereignstack/sovstack-audit.db \
  "SELECT timestamp, user, model, status_code FROM audit_logs ORDER BY timestamp DESC LIMIT 5"

# Per-user quota state
sovstack keys info alice
```

## 9. (Optional) Wire up Grafana

```yaml
# prometheus.yml
scrape_configs:
  - job_name: sovstack-gateway
    static_configs: [{ targets: ['localhost:8001'] }]
    scheme: https
    tls_config: { insecure_skip_verify: true }    # dev only

  - job_name: sovstack-vllm
    static_configs: [{ targets: ['localhost:8888'] }]
    metrics_path: /api/v1/models/mistral-7b/metrics
```

Two metric layers, two scrape jobs — see
[MONITORING.md](MONITORING.md) for the dashboard JSONs.

## What's next

- **Production setup**: [`docs/CONFIGURATION.md`](CONFIGURATION.md) for the
  full YAML reference
- **Multiple users / teams**: [`docs/KEYS_MANAGEMENT.md`](KEYS_MANAGEMENT.md)
- **Quotas**: [`docs/TOKEN_QUOTAS.md`](TOKEN_QUOTAS.md)
- **OIDC sign-in for admins**: [`docs/PHASE_F_USER_MANAGEMENT.md`](PHASE_F_USER_MANAGEMENT.md)
- **HA with Redis-backed quotas**: [`docs/PHASE_G_OBSERVABILITY.md`](PHASE_G_OBSERVABILITY.md)

## Troubleshooting

| Symptom | Likely cause |
|---------|--------------|
| `401 Unauthorized` | API key didn't match. Check `sovstack keys info <user>` and confirm the key prefix in your client. |
| `403 access denied` | User doesn't have the model in their `allowed_models`. Run `sovstack keys grant-model …`. |
| `403 source IP not allowed` | Service-account key with `--ip-allowlist` set; request came from outside the allowed range. |
| `429 token_quota_exceeded` | User hit their daily or monthly cap. `sovstack keys set-quota <user> --daily 0` removes it. |
| `502 Bad Gateway` | Model container is down. `docker ps` and `sovstack deploy <model>`. |
| `connection refused on :8888` | Management service isn't running. Start `sovstack management` (or the split commands). |
| `x509: certificate signed by unknown authority` | Self-signed cert in dev. Use `-k` with curl, or pin the cert fingerprint logged at gateway startup. |

If you're stuck, file a [bug report](https://github.com/sovereignstack/sovereignstack/issues/new?template=bug.yml).
