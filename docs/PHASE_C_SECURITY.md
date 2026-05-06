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

## C4 — Named admin keys with per-actor attribution (shipped)

```
sovstack policy \
  --admin alice=sk_admin_xxx \
  --admin bob=sk_admin_yyy
```

Or in YAML:

```yaml
management:
  admin_keys:
    alice: sk_admin_xxx
    bob:   sk_admin_yyy
```

Behaviour:
- Each name → token pair is registered in `Service.NamedAdmins`.
- Incoming Bearer is constant-time-compared against the map; the
  matched name is the *actor*.
- Legacy `--admin-key` (single string) still works and is recorded as
  actor `admin`.
- OIDC sessions (Phase F1) with `role=admin` use the OIDC `subject` as
  the actor.
- The `Service.AdminAudit` hook fires after every successful admin
  mutation with `(actor, action, request)`. The policy command wires
  it to slog so each grant/revoke/quota mutation logs:

```json
{"level":"INFO","msg":"admin action","actor":"alice","action":"grant_model user=carol model=mistral-7b","ip":"10.0.0.5:54321","path":"/api/v1/users/carol/models/mistral-7b"}
```

Tests in `core/management/policy/admins_test.go`:
- named-token match returns the actor; unknown token returns ""
- legacy AdminKey maps to actor `admin`
- AdminAudit fires for grant/revoke/set-quota mutations
- nil AdminAudit hook is safe (handlers proceed normally)

## C5 — AES-GCM encryption for sensitive profile fields (shipped)

`UserProfile.Department`, `.Team`, `.Role` are encrypted at rest under a
master key configured via `--master-key-file`:

```
sovstack policy \
  --keys ~/.sovereignstack/keys.json \
  --master-key-file ~/.sovereignstack/master.key
```

Behaviour:
- On first run with `--master-key-file`, a 32-byte random key is
  generated and saved to the path with mode `0600`.
- The keystore caches the key and transparently encrypts those three
  fields on every save (`saveLocked` deep-copies and encrypts a clone
  so the in-memory map stays plaintext for fast reads).
- A fresh load surfaces ciphertext in memory until
  `DecryptProfilesInPlace` is called — the policy command does this
  immediately after load.
- Storage format: `$enc1$<base64(nonce || AES-GCM ciphertext)>`.
- A keys.json mixed with plaintext (pre-Phase-C5) and encrypted
  (post-) profiles loads cleanly — `crypto.Decrypt` passes plaintext
  through unchanged.

Migration (idempotent):

```bash
sovstack policy --keys ... --master-key-file ~/.sovereignstack/master.key
# any subsequent admin write triggers re-save with encryption
# OR explicitly:
# (programmatic) ks.MigrateEncryptFields()
```

Tests in `core/keys/encryption_test.go`:
- ciphertext on disk; plaintext-only fields don't appear
- in-memory profile stays plaintext after `AddUser`
- fresh load → ciphertext in memory → `DecryptProfilesInPlace` → plaintext
- wrong master key fails decryption (GCM auth tag verification)
- migration from a plaintext keys.json
- idempotent migration (second call reports 0 newly-encrypted profiles)
- `HasEncryptedFields` correctly distinguishes load-from-disk vs
  populated-via-AddUser state

Plus `core/crypto/crypto_test.go`:
- round-trip encrypt/decrypt for ASCII + Unicode
- non-deterministic ciphertext (different nonces)
- prefix recognition + plaintext passthrough
- tampered ciphertext fails GCM auth
- `LoadOrCreateMasterKey` generates with 0600 mode and is idempotent

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
