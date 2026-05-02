# Phase B — Datastore

This phase makes data storage flexible enough for both single-node OSS
deployments and HA enterprise deployments, without forking the OSS code.

## What changed

| Concern | Before | After |
|---------|--------|-------|
| Quota state | In-memory only (lost on restart) | Pluggable: memory / sqlite / redis |
| Visibility DB | Flag-only config | YAML config (with flag/env override) |
| Audit hot path | Encrypted SQLite | + optional MySQL `audit_logs` table |
| Audit cold path | None | Daily-rotated JSONL with auto-gzip |
| MySQL deployment | Manual | docker-compose with healthcheck + schema |
| Redis | N/A | Optional compose profile, schema-less |
| SQLite → MySQL | Manual | `sovstack migrate audit --from --to` |

## Quota backend selection

Configure in `sovstack.yaml`:

```yaml
gateway:
  quota:
    backend: sqlite          # memory | sqlite | redis
    sqlite_db: ./sovstack-quota.db
    redis:
      addr: localhost:6379
      password: ""
      db: 0
      key_prefix: "sovstack:quota:"
```

| Backend | Persistence | Multi-instance | When to use |
|---------|-------------|----------------|-------------|
| `memory` (default) | Lost on restart | No | Local dev, throwaway demos |
| `sqlite` | Persists | No | Single-instance OSS production |
| `redis` | Persists | **Yes** | HA / load-balanced gateways |

**Recommended OSS default:** `sqlite`. Survives restarts; single dependency
(file). The default `quota.backend: memory` exists for backward
compatibility — set `sqlite` for any real deployment.

**For HA:** `redis` is required. With `memory` or `sqlite`, two gateway
replicas behind a load balancer would each track their own counters and a
user with a 1M-token monthly limit could spend 2M before either replica
saw the cap.

## Visibility backend MySQL configuration

Two deployment patterns:

### (a) Bundled MySQL (docker-compose)

For dev, demo, and small-scale single-host deployments:

```bash
cd sovereignstack-visibility
docker compose up -d mysql backend
# Open http://localhost:9000/healthz
```

`docker-compose.yml` ships MySQL 8 with a healthcheck, persistent volume
(`mysql_data`), and an auto-applied schema (`db/init.sql`). Backend waits
for MySQL `service_healthy` before starting.

### (b) BYO MySQL (production)

For production, point env vars or YAML at your managed MySQL (RDS, Cloud
SQL, self-hosted). No docker-compose changes needed.

```yaml
# sovstack.yaml
visibility:
  port: 9000
  db_type: mysql
  db:
    host: visibility-prod.cluster-xyz.rds.amazonaws.com
    port: 3306
    name: sovstack_visibility
    user: sovstack_app
    password: ""              # set via SOVSTACK_DB_PASSWORD env
```

Apply the schema once, against your DB:

```bash
mysql -h $DB_HOST -u $DB_USER -p $DB_NAME < sovereignstack-visibility/db/init.sql
```

The schema is idempotent (`CREATE TABLE IF NOT EXISTS`); re-running on an
existing DB is safe.

**Recommended sizing (rough order of magnitude):**

| Workload | DB rows/day | Disk after 90 days | Connections |
|----------|-------------|---------------------|-------------|
| Single team, ~10 users | ~5K | ~50 MB | 5 |
| Department, ~100 users | ~50K | ~500 MB | 25 |
| Org, ~1000 users | ~500K | ~5 GB | 100 |

InnoDB defaults work fine. Enable `slow_query_log` if you see >100 ms on
the visibility dashboard's `/api/v1/visibility/gateway/timeseries` endpoint.

## Audit log: hot vs cold path

The gateway can write audit records to **multiple sinks simultaneously**.
Pick whatever combination fits your retention and analysis needs.

```yaml
gateway:
  audit_db: ./sovstack-audit.db    # SQLite (default; encrypted)
  audit:
    jsonl_dir: /var/log/sovereignstack    # cold path; daily-rotated, gzipped
    mysql_dsn: ""                          # hot path (B3, optional)
    retention_days: 30                     # MySQL pruning
    shipper_url: ""                        # s3://, syslog://, https:// (future)
```

| Sink | Path | Use case |
|------|------|----------|
| SQLite (encrypted) | `gateway.audit_db` | Default; queryable from gateway debug endpoints |
| JSONL (rotated) | `gateway.audit.jsonl_dir` | Compliance archives, log shippers, offline grep |
| MySQL `audit_logs` | `gateway.audit.mysql_dsn` | Dashboard queries (visibility backend reads here) |

**File layout for JSONL cold path:**

```
/var/log/sovereignstack/
├── audit-2026-05-01.jsonl       ← today, open file handle
├── audit-2026-04-30.jsonl.gz    ← yesterday, auto-gzipped
└── audit-2026-04-29.jsonl.gz
```

Files are not auto-pruned. Use `logrotate` or a cron job:

```bash
find /var/log/sovereignstack -name 'audit-*.gz' -mtime +365 -delete
```

## Migrating from SQLite to MySQL

After running on SQLite for a while and adding MySQL, copy historical data
once with the migrate command:

```bash
export SOVSTACK_AUDIT_KEY=...   # the encryption key the gateway used

sovstack migrate audit \
    --from ./sovstack-audit.db \
    --to "sovstack_app:secret@tcp(db-host:3306)/sovstack_visibility?parseTime=true" \
    --batch 1000

# Optionally: migrate only recent rows
sovstack migrate audit \
    --from ./sovstack-audit.db \
    --to "$DSN" \
    --since 2026-04-01T00:00:00Z
```

The migration is **idempotent** (UPSERT on
`(correlation_id, event_type)`), so re-running won't double-count rows.
Records without a `correlation_id` are skipped (those are pre-Phase 4
audit entries that didn't include trace IDs).

## Files added / modified

| File | Action | Phase |
|------|--------|-------|
| `core/gateway/quota.go` | MODIFY — refactored to use Backend interface | B5 |
| `core/gateway/quota_backend.go` | CREATE — Backend interface + memory impl | B5 |
| `core/gateway/quota_sqlite.go` | CREATE — SQLite backend, persists across restarts | B5 |
| `core/gateway/quota_redis.go` | CREATE — Redis backend, multi-instance | B5 |
| `core/gateway/quota_builder.go` | CREATE — config-driven backend factory | B5 |
| `core/gateway/quota_backend_test.go` | CREATE — 13 tests including persistence | B5 |
| `core/audit/jsonl.go` | CREATE — daily-rotated JSONL with auto-gzip | B4 |
| `core/audit/multi.go` | CREATE — MultiSinkLogger fan-out | B3+B4 |
| `core/audit/jsonl_test.go` | CREATE — 7 tests | B3+B4 |
| `core/config/config.go` | MODIFY — added QuotaConfig + AuditConfig | B5+B3 |
| `cmd/gateway.go` | MODIFY — wire quota builder + audit fan-out | B5+B4 |
| `cmd/migrate.go` | CREATE — `sovstack migrate audit` command | B6 |
| `sovereignstack-visibility/internal/config/config.go` | CREATE — visibility's own YAML loader | B2 |
| `sovereignstack-visibility/cmd/visibility/main.go` | MODIFY — config-first wiring | B2 |
| `sovereignstack-visibility/db/init.sql` | MODIFY — added `gateway_metrics` + `audit_logs` | B1+B3 |
| `sovereignstack-visibility/docker-compose.yml` | MODIFY — Redis profile, health URL fix | B1 |

**Total tests added in Phase B:** 25 (quota backends) + 7 (audit JSONL/multi) = 32 new tests.

## Test summary

```
core/gateway:        74/74 ✓ (49 prior + 25 new quota)
core/audit:          14/14 ✓ (7 prior + 7 new)
core/config:         10/10 ✓
visibility/config:    5/5  ✓ (new)
visibility/api:      23/23 ✓
visibility/collector: 9/9  ✓
─────────────────────────────────
Phase A+B total: 170+ tests passing
```

## Migration guide

If upgrading from a Phase A build:

1. **Quota backend** — explicitly set `gateway.quota.backend: sqlite` (or
   `memory` to keep current behavior). Default is still `memory` for
   backward compat, but `sqlite` is recommended for any real deployment.
2. **Audit cold path (optional)** — set `gateway.audit.jsonl_dir` to start
   writing rotated JSONL alongside your existing SQLite. No migration of
   existing data needed; old SQLite rows stay in SQLite.
3. **Visibility backend** — schema additions are idempotent; re-run
   `db/init.sql` against existing MySQL to add the `gateway_metrics` and
   `audit_logs` tables.
4. **No breaking changes** — all defaults preserve Phase A behavior.
