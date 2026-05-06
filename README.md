# SovereignStack

> Self-hosted LLM serving with auth, quotas, audit, and Prometheus metrics — in a single Go binary.

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go version](https://img.shields.io/badge/Go-1.24%2B-00ADD8.svg)](go.mod)
[![Tests](https://img.shields.io/badge/tests-260%20passing-brightgreen.svg)](#testing)

SovereignStack runs your LLM models on your hardware, behind a hardened HTTP
gateway. It's the boring infrastructure piece nobody enjoys writing — API key
auth, rate limits, token quotas, model routing, audit logs, Prometheus metrics
— so you can focus on the actual inference.

```bash
# Install once
go install github.com/sovereignstack/sovereignstack@latest

# Pull a model
sovstack pull mistral-7b

# Add a user
sovstack keys add alice
sovstack keys grant-model alice mistral-7b

# Start serving (auto-generates self-signed TLS cert)
sovstack policy &       # admin API on :8888
sovstack discovery &    # model discovery on :8889
sovstack gateway        # request gateway on :8001
```

You now have an OpenAI-compatible HTTPS endpoint with per-user auth, rate
limits, and audit logging — without touching a YAML file.

---

## Why?

You want to run LLMs on your own hardware (regulatory, cost, latency, or
preference reasons), but vLLM / llama.cpp / Ollama only solve the inference
half. The other half — *who can use it, how much, and what did they do?* —
is what every team rebuilds by hand.

SovereignStack is that other half:

- **Authenticate** every request with hashed API keys
- **Authorize** per-user model allowlists
- **Rate-limit** per user (tokens-per-minute)
- **Quota** per user (tokens-per-day, tokens-per-month)
- **Route** requests to the right model container by name
- **Observe** with Prometheus metrics on every layer
- **Audit** every request to an encrypted SQLite log
- **Harden** with TLS-by-default and atomic key store writes

It's one Go binary, no external services required.

## Status

Production-ready (~370 tests, 0 external runtime dependencies beyond Docker).
Used by teams running real LLM workloads. Versioning follows
[SemVer](https://semver.org/); breaking changes go in major releases only.

## Documentation

| What you want to do | Read |
|---------------------|------|
| Get a stack running in 5 minutes | [Quickstart](docs/QUICKSTART.md) |
| Configure for production | [Configuration reference](docs/CONFIGURATION.md) |
| Understand the architecture | [Architecture overview](docs/ARCHITECTURE.md) |
| Deploy with Docker | [Docker guide](docs/DOCKER_COMPOSE_SERVICES.md) |
| Manage users and keys | [Keys management](docs/KEYS_MANAGEMENT.md) |
| Set token quotas | [Token quotas](docs/TOKEN_QUOTAS.md) |
| Hook into Grafana | [Monitoring](docs/MONITORING.md) |
| Read the API | [API reference](docs/MANAGEMENT_API.md) |
| Understand the security model | [Security model](docs/PHASE_C_SECURITY.md) |

## Architecture

```
            ┌──────────────────────────────────────┐
   curl  ─► │  Gateway (:8001)                     │
   SDK      │  Auth → Access → Quota → Rate-limit  │
   …        │  → Routing → Reverse proxy           │
            │  Exposes Prometheus /metrics         │
            └──────────┬─────────────────┬─────────┘
                       │ poll            │ proxy /v1/...
                       ▼                 ▼
            ┌────────────────────┐  ┌────────────────────┐
            │ Policy   (:8888)   │  │ Model containers   │
            │ Discovery(:8889)   │  │ vLLM / llama.cpp   │
            │ Metrics  (:8890)   │  │                    │
            └────────────────────┘  └────────────────────┘
                       │
                       │ docker socket (read-only)
                       ▼
                Docker daemon
```

Three small services replace what other stacks do as one big one:

- **`gateway`** — every request flows through here. Stateless, horizontally scalable.
- **`policy`** — owns `keys.json`. Admin REST API for user/quota/model management. Optional OIDC for human admins.
- **`discovery`** — lists running model containers via Docker.
- **`metrics-proxy`** — fans vLLM `/metrics` endpoints out by model name.

Each is a separate binary; they're mounted in the same `sovstack` for ease.
You can run one container with all three, three containers, or two of one
and one of another behind a load balancer.

Full architecture in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Quickstart

```bash
# 1. Install
git clone https://github.com/sovereignstack/sovereignstack
cd sovereignstack
go install .

# 2. One-time setup
sovstack init

# 3. Pull a model (downloads vLLM-compatible weights)
sovstack pull mistral-7b

# 4. Create your first user
sovstack keys add alice --department research
# → API key printed; save it now (it won't be shown again)

# 5. Grant access to a model
sovstack keys grant-model alice mistral-7b

# 6. Start the services (use a process manager in production)
sovstack policy --master-key-file ~/.sovereignstack/master.key &
sovstack discovery &
sovstack gateway --keys ~/.sovereignstack/keys.json

# 7. Hit it
curl -H "X-API-Key: sk_..." \
     -d '{"messages":[{"role":"user","content":"hello"}]}' \
     https://localhost:8001/v1/models/mistral-7b/chat/completions
```

Detailed walkthrough in [docs/QUICKSTART.md](docs/QUICKSTART.md).

## What's in the box

- ✅ **API key auth** with argon2id-hashed keys at rest
- ✅ **Per-user model allowlists** with wildcard support
- ✅ **Token quotas** (daily + monthly) with persistence (SQLite or Redis)
- ✅ **Rate limiting** per user (tokens-per-minute)
- ✅ **Multi-model routing** via auto-discovery from Docker
- ✅ **Prometheus metrics** on `/metrics`
- ✅ **vLLM metrics passthrough** for inference-engine internals
- ✅ **Audit log** (encrypted SQLite + optional rotated JSONL)
- ✅ **TLS by default** with auto-generated self-signed certs
- ✅ **OIDC sign-in** for admin actions (Keycloak, Authentik, Auth0, …)
- ✅ **IP allowlists** for service accounts
- ✅ **Named admin attribution** for audit trails
- ✅ **OpenTelemetry tracing** (OTLP)
- ✅ **Atomic key-store writes** with cross-process locking

## Configuration

A single YAML file works for every service:

```yaml
log:
  format: json    # text in dev, json in prod
  level: info

gateway:
  port: 8001
  rate_limit: 100
  keys_file: ~/.sovereignstack/keys.json
  audit:
    jsonl_dir: /var/log/sovereignstack
    retention_days: 30
  quota:
    backend: sqlite       # memory | sqlite | redis
    sqlite_db: ./quota.db

management:
  port: 8888
  admin_keys:
    alice: sk_admin_alice
    bob:   sk_admin_bob
  oidc:
    issuer_url: https://keycloak.example.com/realms/sovstack
    client_id: sovstack-policy
    client_secret: ${SOVSTACK_OIDC_CLIENT_SECRET}
    redirect_url: https://policy.example.com/api/v1/auth/callback

cors:
  origins: ["https://dashboard.example.com"]

tls:
  cert_file: /etc/letsencrypt/.../fullchain.pem
  key_file:  /etc/letsencrypt/.../privkey.pem
```

All keys are documentable; see [`sovstack.yaml.example`](sovstack.yaml.example).

## Building

```bash
# Build the binary
go build -o sovstack .

# Run all tests (370+ across 23 packages)
go test ./...

# Build a Docker image
docker build -t sovstack:dev .
```

Requires Go 1.24+ and (for `sovstack pull`/`deploy`) Docker.

## Contributing

We welcome contributions. See [CONTRIBUTING.md](CONTRIBUTING.md) for the
development workflow, code style, and how we review PRs.

For security issues please follow [SECURITY.md](SECURITY.md) — do not
file public GitHub issues for vulnerabilities.

## License

[Apache License 2.0](LICENSE) — use it commercially, modify it, ship it.

## Links

- [Issue tracker](https://github.com/sovereignstack/sovereignstack/issues)
- [Discussions](https://github.com/sovereignstack/sovereignstack/discussions)
- [Changelog](CHANGELOG.md)
- [Code of Conduct](CODE_OF_CONDUCT.md)

## Related projects

SovereignStack focuses on the operational layer. The visualization layer
(dashboards, cost analytics, anomaly detection) is a separate commercial
product called [SovereignStack Visibility](https://sovereignstack.io). It
builds on the same `/metrics` and `/api/v1/*` endpoints documented here —
the OSS stack is fully usable on its own with Grafana / your own tooling.
