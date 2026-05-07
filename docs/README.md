# SovereignStack Documentation

This is the index. Pick what matches what you're trying to do.

## Get started

- [Quickstart](QUICKSTART.md) — `curl` against your own LLM in 5 minutes
- [Architecture](ARCHITECTURE.md) — high-level diagram, components, request flow

## Configure and operate

- [Configuration reference](CONFIGURATION.md) — every YAML key
- [CLI reference](CLI_REFERENCE.md) — every `sovstack` subcommand
- [Repo structure](STRUCTURE.md) — package / directory layout
- [Releases & packaging](RELEASES_AND_PACKAGING.md) — install via brew/apt; how releases are cut

## Manage users and access

- [Keys management](KEYS_MANAGEMENT.md) — adding users, granting models, setting quotas
- [Token quotas](TOKEN_QUOTAS.md) — daily / monthly caps and how they reset
- [Gateway access control](GATEWAY_ACCESS_CONTROL.md) — model allowlists and the wildcard
- [Gateway security](GATEWAY_SECURITY.md) — short overview of the gateway's security posture
- [Authentication architecture](ARCHITECTURE_AUTH.md) — deep dive on the auth → access → quota path

## Run models

- [Model management](MODEL_MANAGEMENT.md) — pull/deploy/stop lifecycle
- [Model discovery](MODEL_DISCOVERY.md) — how the gateway learns which models exist
- [Multi-model routing](MULTI_MODEL_ROUTING.md) — `/v1/models/{name}/...` URL convention

## Deploy

- [Docker compose services](DOCKER_COMPOSE_SERVICES.md) — bundled vs BYO MySQL

## Observe

- [Monitoring](MONITORING.md) — Prometheus + Grafana
- [Monitoring quickstart](MONITORING_QUICKSTART.md) — copy-paste recipes
- [Gateway Prometheus metrics](GATEWAY_PROMETHEUS_METRICS.md) — every metric and its meaning
- [Audit log](AUDIT.md) — how the audit log works and how to ship it

## API

- [Management API](MANAGEMENT_API.md) — REST endpoints owned by `sovstack policy`

## Contribute

- [How to contribute (this repo)](CONTRIBUTING.md) — workflow + code style
- (Top-level [`/CONTRIBUTING.md`](../CONTRIBUTING.md) covers PR review process and dep policy)

## Other top-level files

- [`README.md`](../README.md) — project overview
- [`CHANGELOG.md`](../CHANGELOG.md) — release history (this is where Phase notes live now)
- [`SECURITY.md`](../SECURITY.md) — vulnerability reporting
- [`CODE_OF_CONDUCT.md`](../CODE_OF_CONDUCT.md) — community standards
