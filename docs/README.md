# SovereignStack Documentation

This is the index. Pick what matches what you're trying to do.

## I want to…

### …get something working fast
- [Quickstart](QUICKSTART.md) — a real `curl` against your own LLM in 5 minutes
- [CLI reference](CLI_REFERENCE.md) — every `sovstack` subcommand
- [Configuration reference](CONFIGURATION.md) — every YAML key

### …understand what this thing is
- [Architecture](ARCHITECTURE.md) — high-level diagram, components, request flow
- [Architecture: auth path](ARCHITECTURE_AUTH.md) — deeper dive on auth + access + quota

### …manage users and access
- [Keys management](KEYS_MANAGEMENT.md) — adding users, granting models, setting quotas
- [Token quotas](TOKEN_QUOTAS.md) — daily / monthly caps and how they reset
- [Gateway access control](GATEWAY_ACCESS_CONTROL.md) — model allowlists and the wildcard
- [Gateway security](GATEWAY_SECURITY.md) — short overview of the gateway's security posture

### …deploy in production
- [Docker compose services](DOCKER_COMPOSE_SERVICES.md) — bundled vs BYO MySQL
- [Hybrid registry](HYBRID_REGISTRY_IMPLEMENTATION.md) — running models from multiple sources
- [Multi-model routing](MULTI_MODEL_ROUTING.md) — `/v1/models/{name}/...` URL convention

### …observe, monitor, alert
- [Monitoring](MONITORING.md) — Prometheus + Grafana
- [Monitoring quickstart](MONITORING_QUICKSTART.md) — copy-paste recipes
- [Gateway Prometheus metrics](GATEWAY_PROMETHEUS_METRICS.md) — every metric and its meaning
- [Audit](AUDIT.md) — how the audit log works and how to ship it

### …call the management API
- [Management API](MANAGEMENT_API.md) — REST endpoints owned by `sovstack policy`
- [Model discovery](MODEL_DISCOVERY.md) — how the gateway learns which models exist
- [Model management](MODEL_MANAGEMENT.md) — pull/deploy/stop lifecycle

## Phase notes (implementation history)

These describe how each subsystem landed — useful if you want to
understand *why* something is the way it is, or you're maintaining the
codebase. Most users won't need them.

- [Phase A — Foundations](PHASE_A_FOUNDATIONS.md)
- [Phase B — Datastore](PHASE_B_DATASTORE.md)
- [Phase C — Security at rest](PHASE_C_SECURITY.md)
- [Phase D — Trust boundary](PHASE_D_TRUST_BOUNDARY.md)
- [Phase E — Management split](PHASE_E_MANAGEMENT_SPLIT.md)
- [Phase F — User management](PHASE_F_USER_MANAGEMENT.md)
- [Phase G — Observability + ops](PHASE_G_OBSERVABILITY.md)
- [Phase 4 — Gateway Prometheus metrics](PHASE_4_GATEWAY_PROMETHEUS_METRICS.md)
- [Phase 8 — Management service APIs](PHASE_8_MANAGEMENT_SERVICE_APIS.md)
- [Phase 9 — vLLM metrics endpoint](PHASE_9_VLLM_METRICS_ENDPOINT.md)

## Reference

- [STRUCTURE.md](STRUCTURE.md) — repo layout
- [PRD.md](PRD.md) — product requirements (historical context)
- [COMMANDS.md](COMMANDS.md) — exhaustive command catalogue

## Other top-level files

- [`README.md`](../README.md) — project overview
- [`CONTRIBUTING.md`](../CONTRIBUTING.md) — how to contribute
- [`CHANGELOG.md`](../CHANGELOG.md) — release history
- [`SECURITY.md`](../SECURITY.md) — vulnerability reporting
- [`CODE_OF_CONDUCT.md`](../CODE_OF_CONDUCT.md) — community standards
