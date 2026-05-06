# Security Policy

SovereignStack is an authentication, authorization, and quota layer for LLM
inference. We treat security bugs seriously.

## Supported versions

| Version | Supported |
|---------|-----------|
| Latest minor (1.x) | ✅ security patches |
| Older minors | ❌ — please upgrade |

We follow [SemVer](https://semver.org/). Security fixes are backported only
to the latest minor.

## Reporting a vulnerability

**Please do not open a public GitHub issue.**

Email `security@sovereignstack.io` with:

1. A description of the issue and its impact
2. Steps to reproduce (or a working PoC)
3. The version (`sovstack --version`) and OS / Go version where you observed it
4. Whether you'd like public credit when the fix lands

You can encrypt your report with the maintainer PGP key — fingerprint published
in `docs/security/pgp-key.txt` (planned; until then, please send unencrypted
and we will respond with a secure channel).

We aim to:
- Acknowledge your report within **2 business days**
- Provide an initial assessment within **5 business days**
- Ship a fix and CVE (where applicable) within **30 days**, faster for
  high-severity issues

## What we consider in scope

- Auth bypass (gateway, policy admin, OIDC)
- Privilege escalation between roles (`user` → `admin`, `service` → other)
- Token / API key disclosure (keys.json leaks beyond mode 0600)
- Audit-log evasion or tampering
- TLS / cipher misconfiguration leading to plaintext exposure
- Quota or rate-limit bypass that lets a user exceed their configured caps
- Path traversal, SQL injection, or XSS in any HTTP endpoint
- Denial of service via crafted requests

## What we consider out of scope

- DoS via raw load (the default config is *not* tuned for adversarial traffic;
  put a WAF / load balancer in front)
- Issues that require already-compromised infrastructure (root on the host,
  stolen master key, write access to `keys.json`)
- Findings against deprecated paths (`/api/health`, hardcoded test keys)
- Self-XSS or similar that requires the victim to paste hostile input
- Best-practice recommendations without a concrete attack path
- Vulnerabilities in dependencies that we've already pinned to the fixed
  version (please open an issue for non-security advisories)

## After a fix

We publish security advisories via:

- A signed release tag with the fix
- A note in [CHANGELOG.md](CHANGELOG.md) under "Security"
- A GitHub Security Advisory with CVSS scoring (high or critical only)
- An entry on the project's security page (planned)

We don't sell early access to security fixes. The OSS path is the canonical
fix path.

## Hardening checklist

Operators running SovereignStack should review
[docs/PHASE_C_SECURITY.md](docs/PHASE_C_SECURITY.md) and the production
docker-compose recipe in the visibility-platform repo. The TL;DR:

- Run with `--master-key-file` set so `keys.json` fields are encrypted at rest
- Run with explicit `--admin alice=…` named admin tokens, not `--admin-key`
- Front the gateway with a real TLS cert (`--tls-cert` / `--tls-key`), don't
  rely on the auto-generated self-signed for internet-facing deployments
- Set `gateway.audit.retention_days` so the audit DB doesn't grow forever
- Ship audit JSONL files off-host (compliance + tamper-evidence)
- Restrict service-account API keys with `--ip-allowlist`
- Run `sovstack policy` and `sovstack gateway` as different OS users so a
  gateway compromise can't read `keys.json`

Thanks for keeping the ecosystem safe.
