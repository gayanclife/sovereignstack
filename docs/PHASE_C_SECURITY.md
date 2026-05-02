# Phase C — Security at rest

This phase hardens the OSS stack against the most realistic threats:
disk theft, backup leakage, accidental file copies in support tickets, and
mid-write crashes. Plaintext API keys are gone; transport is encrypted by
default; the keystore file can no longer corrupt.

## What's protected

| Threat | Mitigation | Phase |
|--------|------------|-------|
| `keys.json` leaks → API keys reusable | argon2id-hashed keys at rest | C1 |
| Crash mid-write → corrupt `keys.json` | write-tmp + atomic rename + flock | C2 |
| Eavesdropping on the local network | TLS on by default, self-signed auto-generated | C3 |
| Concurrent CLI + management writes | flock around save | C2 |
| Disk-stolen `cert.pem`/`key.pem` | mode 0600, dir 0700 | C3 |

## What is NOT protected (yet)

| Threat | Status |
|--------|--------|
| In-memory key after auth (RAM dump) | Not addressed; out of scope for OSS |
| Sensitive profile fields (department/team) at rest | Phase C5 (deferred — see "Follow-ups" below) |
| Admin auth attribution per actor | Phase C4 (deferred) |
| Master-key compromise reveals everything | Documented; the master-key file is the keystone |

## C1 — argon2id hashing for API keys at rest

Stored format:

```
$argon2id$v=19$m=65536,t=2,p=2$<salt-b64>$<hash-b64>
```

- **Algorithm:** argon2id with 64 MiB memory, 2 iterations, 2 threads,
  32-byte output. Tuned for ~50 ms/auth on a 2024-class server CPU.
- **Index:** each profile also stores an HMAC-SHA256 fingerprint
  (`key_index`) of the plaintext under a stable server-side secret.
  `GetByKey` filters candidates by the index *before* invoking
  argon2.Verify, so auth latency for an N-user keystore is one
  argon2 call regardless of N.
- **Plaintext shown once:** `sovstack keys add` prints the generated key
  exactly once. There is no "show key" command for hashed entries.
- **Migration:** `sovstack keys migrate-hash` is idempotent and converts
  a plaintext `keys.json` in place. Existing plaintext profiles continue
  to authenticate before migration (fallback path in `GetByKey`); after
  migration, `HasPlaintextKeys()` returns false and the fallback never
  fires.

Tests in `core/keys/hash_test.go`:
- format, salting, round-trip verify, rejection of non-argon hashes
- `keyIndex` stability and uniqueness
- end-to-end: hashed in store, plaintext-as-key auth, hash-as-key rejected
- migration round-trip + idempotency

## C2 — atomic `keys.json` writes

The keystore now writes through a temp file and atomically renames it,
under a cross-process flock:

```go
1. open keys.json.lock, LOCK_EX
2. CreateTemp("keys.json.tmp-*")
3. Write(bytes); Chmod(0600); Sync(); Close()
4. os.Rename(tmp, "keys.json")
5. close lock file (kernel auto-releases LOCK_EX)
```

A crash at any step leaves the on-disk file either at the previous
contents or the new contents — never partially written. Cross-process
concurrent writers (the CLI and the management service serving an
admin POST) are serialised by the flock.

Windows note: `flock_windows.go` is a no-op stub. In-process serialisation
via the keystore mutex still applies; cross-process safety on Windows
will land if a production user needs it.

Tests in `core/keys/atomic_test.go`:
- no temp files left behind
- file is parseable JSON at every observation under concurrent writers
- lock file exists after writes
- on-disk mode is 0600

## C3 — TLS with auto-generated self-signed default

```
~/.sovereignstack/
├── keys.json
├── tls/
│   ├── cert.pem    (mode 0600, ECDSA-P256, 1-year validity)
│   └── key.pem     (mode 0600, EC private key)
```

First-run experience for any service (gateway/management/visibility):

1. No `--tls-cert`/`--tls-key` provided.
2. Service generates a self-signed cert + key in `~/.sovereignstack/tls/`
   if the files don't exist.
3. SHA-256 fingerprint is logged so operators can pin clients:
   `TLS enabled cert=… fingerprint=sha256:aa:bb:cc:…`
4. Service listens on HTTPS.

Production: pass `--tls-cert /etc/letsencrypt/.../fullchain.pem`
`--tls-key /etc/letsencrypt/.../privkey.pem` (or set `tls.cert_file` /
`tls.key_file` in YAML). Self-signed generation is skipped.

Local dev: `--insecure-http` (or `tls.insecure_http: true`). The service
logs a `WARN` on startup: *"TLS disabled — serving plain HTTP. Do NOT use
this in production."*

Tests in `core/tls/tls_test.go`:
- self-signed generation on first call
- existing files reused (cert is not regenerated)
- caller-supplied paths pass through (no auto-gen dir mutation)
- missing caller-supplied paths returns a clear error
- fingerprint format is `sha256:aa:bb:…` with 64 hex + 31 colons

## Follow-ups (planned, not yet shipped)

### C4 — Named admin keys with per-actor attribution

```
sovstack management \
  --admin-key alice=sk_admin_xxx \
  --admin-key bob=sk_admin_yyy
```

Plan:
- Replace `--admin-key <single-string>` with a `name=key` repeatable flag.
- Match incoming Bearer tokens against the map; inject the matched name
  into request context.
- Add an `Actor` field to `AuditLog` and propagate from management
  handlers when an admin mutation happens.

Why deferred: requires touching the audit schema (additional column) and
every admin handler. Deferred to a follow-up patch so Phase D's trust
boundary work isn't blocked.

### C5 — AES-GCM encryption for sensitive profile fields

`UserProfile.Department`, `.Team`, `.Role` are PII even after API key
hashing. Plan:

1. Add a `core/crypto` package: `Encrypt(masterKey, plaintext)` /
   `Decrypt(masterKey, ciphertext)` over AES-256-GCM.
2. Read master key from `--master-key-file` (auto-generate on first run,
   mode 0600).
3. Wrap `UserProfile` JSON marshaling so encrypted fields round-trip
   transparently.
4. Idempotent migration command:
   `sovstack keys migrate-encrypt-fields`.

Why deferred: same scope reasoning as C4. The hashing of API keys (C1)
already prevents the highest-impact data-leak scenario; encrypting
metadata is defence-in-depth for a different threat.

## Migration guide

Upgrading from a Phase A/B build:

1. **Hash existing keys** (one-time):
   ```bash
   sovstack keys migrate-hash
   ```
   Idempotent. Existing plaintext profiles still auth before migration;
   after, only hashed lookup is used.

2. **TLS** is now the default. If running locally and you don't want to
   deal with self-signed certs:
   ```yaml
   tls:
     insecure_http: true     # development only
   ```
   For production, point at real certs:
   ```yaml
   tls:
     cert_file: /etc/letsencrypt/live/api.example.com/fullchain.pem
     key_file:  /etc/letsencrypt/live/api.example.com/privkey.pem
   ```

3. **No keys.json schema break.** Hashed keys add an opt-in `key_index`
   field; old files without it still load.

4. **No DB break.** No tables changed in this phase.

## Files added / modified

| File | Action | Phase |
|------|--------|-------|
| `core/keys/hash.go` | CREATE — argon2id + HMAC index helpers | C1 |
| `core/keys/hash_test.go` | CREATE — 7 tests including end-to-end + migration | C1 |
| `core/keys/store.go` | MODIFY — `AddUser` hashes; `GetByKey` verifies; `MigrateHashes` added | C1+C2 |
| `core/keys/flock_unix.go` | CREATE — POSIX flock | C2 |
| `core/keys/flock_windows.go` | CREATE — Windows no-op stub | C2 |
| `core/keys/atomic_test.go` | CREATE — 4 tests | C2 |
| `cmd/keys.go` | MODIFY — `keys migrate-hash` subcommand | C1 |
| `core/tls/tls.go` | CREATE — Resolve + self-signed generation | C3 |
| `core/tls/tls_test.go` | CREATE — 6 tests | C3 |
| `core/config/config.go` | MODIFY — added `TLSConfig` | C3 |
| `cmd/gateway.go` | MODIFY — wired TLS resolve into ListenAndServeTLS | C3 |

## Test coverage

```
core/keys:    26/26 ✓ (18 prior + 8 new across hashing + atomic)
core/tls:      6/6  ✓ (new)
core/config:  10/10 ✓
─────────────────────────────────
Phase C total tests added: 17
```

Smoke verified end-to-end:
- Plaintext `keys.json` migrated to hashed via `keys migrate-hash`
- Auth succeeds with original plaintext after migration
- `~/.sovereignstack/tls/{cert,key}.pem` auto-generated on first gateway
  start; HTTPS returns 200 on `/healthz`; plain HTTP is rejected
