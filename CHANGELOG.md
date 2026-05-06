# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

Nothing pending; cut a release tag when ready.

## [1.0.0] — 2026-05-07

The first stable, publishable release. The previous incubating builds
introduced breaking changes between phases; from this version forward
SemVer applies.

### Added

#### Foundations
- YAML config loader with strict precedence: CLI > env > file > defaults
- Structured logging via `log/slog` with `text` and `json` formats
- Liveness (`/healthz`) and readiness (`/readyz`) endpoints on every service
- CLI `--output json` for `keys add | list | info | remove`

#### Datastore
- Visibility backend honours a YAML config for MySQL endpoints (BYO or bundled)
- Audit log fan-out: encrypted SQLite hot path + daily-rotated JSONL cold path
- Pluggable quota backend: memory (dev), SQLite (single-instance prod), Redis (HA)
- `sovstack migrate audit` command for moving SQLite → MySQL

#### Security
- API keys hashed with argon2id at rest
- HMAC-SHA256 fingerprint index for O(log N) auth lookup
- Atomic `keys.json` writes (write-tmp + rename) under cross-process flock
- TLS by default with auto-generated self-signed certs in `~/.sovereignstack/tls/`
- `--tls-cert` / `--tls-key` overrides for production certificates
- AES-256-GCM encryption of `Department`, `Team`, `Role` fields when a master
  key is configured (`--master-key-file`)
- Named admin attribution: `--admin alice=sk_xxx` (repeatable) with per-actor
  audit hooks

#### Trust boundary
- Visibility backend no longer reads `keys.json` directly; uses `GET /api/v1/users`
  over HTTP with an admin token instead
- Production `docker-compose.production.yml` recipe with non-root users,
  read-only root filesystem, no host port mappings on internal services

#### Management split
- `sovstack discovery` — Docker discovery only (port 8889, no auth)
- `sovstack policy` — user policy only (port 8888, admin auth, owns `keys.json`)
- `sovstack metrics-proxy` — vLLM `/metrics` passthrough (port 8890, no auth)
- `sovstack management` retained as a backward-compat shim that mounts all three

#### User management
- OIDC sign-in for admin actions (`/api/v1/auth/{login,callback,logout}`)
  with signed session cookies — works with Keycloak, Authentik, Auth0, Okta
- `service` role + `IPAllowlist` for machine-to-machine API keys

#### Observability + ops
- OpenTelemetry tracing in every service (zero-overhead when
  `OTEL_EXPORTER_OTLP_ENDPOINT` is unset)
- Audit log retention pruner (`audit.retention_days`)
- Visibility backend exposes `/api/v1/visibility/scrape-status` for
  data-freshness banners
- Hosted pricing data: `--pricing` accepts a URL or file path

### Changed

- All HTTP API endpoints moved under `/api/v1/...`. Older paths return 404.
  Update API consumers; there is no compatibility shim. (Breaking only for
  pre-1.0 builds; no migration needed for new users.)
- Default audit logger emits structured records via `slog`.

### Removed

- Hardcoded `sk_test_123` / `sk_demo_456` development keys (use `sovstack keys add`).
- Single shared CORS wildcard. Operators must explicitly list origins.

### Security

- `keys.json` permissions tightened to mode 0600 on creation.
- TLS cert / key files in `~/.sovereignstack/tls/` are mode 0600;
  containing directory mode 0700.

### Known limitations

- Engine integration tests assume a clean Docker environment; if
  `ss-*` containers are running on the host they will fail (5/260 tests).
  Stop the containers or run `go test ./... -short` to skip those.
- Multi-instance gateways need the Redis quota backend to share state
  correctly. The default `memory` backend is single-instance only.
- Per-actor audit attribution flows to the `slog` `admin action` line for
  named admins and OIDC sessions; persisting to a structured audit table
  is on the roadmap.

### Test coverage

- 260 Go tests across 23 packages (~370 total counting subtests)
- All passing on a clean Docker environment

[Unreleased]: https://github.com/sovereignstack/sovereignstack/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/sovereignstack/sovereignstack/releases/tag/v1.0.0
