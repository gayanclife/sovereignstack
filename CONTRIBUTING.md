# Contributing to SovereignStack

Thanks for your interest in contributing. This document explains how to
propose changes, the code-review expectations, and the project's
conventions.

For the model-registry contribution flow specifically (adding a new model
to the auto-deploy catalogue), see
[`docs/CONTRIBUTING.md`](docs/CONTRIBUTING.md).

## Quick start

```bash
git clone https://github.com/sovereignstack/sovereignstack
cd sovereignstack
go test ./...        # all 260+ tests should pass
go build -o sovstack .
./sovstack --help
```

## What we accept

| Contribution type | Accepted? | Notes |
|-------------------|-----------|-------|
| Bug fixes | ✅ Always | Include a regression test |
| Performance improvements with benchmarks | ✅ Always | Show the measurement |
| New audit / quota / metric features | ✅ With discussion | Open an issue first |
| Major refactors | ⚠ Discuss first | We don't merge "rewrites" without alignment |
| New CLI commands | ⚠ Discuss first | Surface area is precious |
| New external dependencies | ❌ Default no | Standard library first; deps need a strong case |
| Documentation typos and clarifications | ✅ Always | Smaller is better |
| Translations | ❌ Not yet | Single-language for now to keep maintenance light |

## Workflow

1. **Open an issue first** for anything beyond a typo or trivial bug fix.
   This avoids you spending a weekend on something we can't merge.
2. Fork and branch off `main`. Branch names: `fix/...`, `feat/...`, `docs/...`.
3. Run `go test ./...` and `go vet ./...` locally before pushing.
4. Open a PR with:
   - A description of *what* changed and *why*
   - A test that fails before your change and passes after (for bug fixes)
   - A note of any new dependencies and why the standard library wouldn't work
5. We review within 1 week. PRs that pass CI and have a clean diff merge fast.

## Code style

- **Go fmt is the law.** `gofmt -s` before committing.
- **`go vet` clean.** No warnings.
- **Standard library first.** External deps need justification in the PR.
- **Errors wrap context.** `fmt.Errorf("read %q: %w", path, err)`, not bare `return err`.
- **Comments explain *why*.** What's already in the code; readers can run
  the code. Comments justify decisions, document invariants, and call out
  surprises.
- **Tests are first-class.** Every public function has a test. Every bug
  fix gets a regression test. Every new behaviour gets a happy-path test.

## Commit style

We don't enforce Conventional Commits but we appreciate clear messages:

```
gateway: enforce IP allowlist for service accounts

Service-account API keys with role=service now require the source IP
to match an entry in IPAllowlist (CIDR or exact). Non-service users
are unaffected.

Closes #142
```

Imperative mood, short title, optional body for the *why*.

## Tests

```bash
go test ./...                  # full suite (260+ tests)
go test ./core/keys/...        # one package
go test ./... -count=1         # disable test result caching
go test ./... -race            # race detector (slower; required for PRs touching concurrency)
```

The engine integration tests assume a clean Docker environment. If you
have `ss-*` containers running, stop them before running `./...` or scope
your test run.

## Documentation

When you add a feature, add docs. The convention is:

- **README.md** — top-level overview, quickstart
- **docs/QUICKSTART.md** — 5-minute walkthrough for new users
- **docs/CONFIGURATION.md** — every config key, with comments
- **docs/<TOPIC>.md** — feature-focused guides (KEYS_MANAGEMENT, MONITORING, etc.)
- (Implementation history lives in git log + CHANGELOG.md, not in
  separate docs.)

A new feature doesn't have to invent a new doc. Extending an existing
one is usually right.

## Dependency policy

- Standard library and `golang.org/x/...` extensions: free
- Well-known, single-purpose libraries (cobra, gorilla/mux, go-sqlite3,
  go-redis): low bar, prefer over hand-rolling
- Anything else: open an issue and explain why before adding

If your PR adds a dep, the PR description must include:
- What we tried with the standard library and why it wasn't enough
- The dep's license (must be Apache-2.0-compatible)
- The transitive dep count
- The maintainer count and last release date

## Security

For security-sensitive changes (auth, crypto, audit), please follow
[SECURITY.md](SECURITY.md). For non-vulnerability hardening
contributions, normal PR flow applies.

## Code of Conduct

Participation in this project is governed by [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## License

By contributing, you agree your contributions will be licensed under the
[Apache License 2.0](LICENSE).

## Maintainers

- Reviews from any maintainer count as approval.
- Two maintainer approvals required for changes to: `core/gateway/proxy.go`,
  `core/keys/store.go`, `core/management/policy/`.
- Direct commits to `main` are reserved for emergency security fixes.

Thanks for making SovereignStack better.
