# Publish Checklist

This is a one-time checklist for publishing the OSS repo. Once 1.0 is
out, future releases use the [CHANGELOG.md](CHANGELOG.md) flow.

Delete this file after the first release.

## What I (the assistant) already did

- [x] Replaced `README.md` with a publication-ready landing page
- [x] Wrote `SECURITY.md` (vulnerability reporting policy)
- [x] Wrote `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1 reference)
- [x] Wrote `CHANGELOG.md` with the 1.0.0 entry
- [x] Refreshed top-level `CONTRIBUTING.md`
- [x] Added `.github/ISSUE_TEMPLATE/{bug,feature}.yml`
- [x] Added `.github/PULL_REQUEST_TEMPLATE.md`
- [x] Added `.github/workflows/ci.yml` (build, test, vet, govulncheck, golangci-lint)
- [x] Refreshed `docs/QUICKSTART.md`
- [x] Wrote `docs/ARCHITECTURE.md` (OSS-only view)
- [x] Refreshed `docs/README.md` (topic-organized index)
- [x] Moved legacy `PHASE_{1,2,2B,3}_COMPLETION.md`, `PROGRESS_SUMMARY.md`,
      `TASKS.md`, `TESTING_MULTIMODEL.md` to `docs/development/`

## What you need to decide

### 1. Repository name and module path

Currently `go.mod` says:

```
module github.com/gayanclife/sovereignstack
```

Before publishing, this should match where you'll host:

- `github.com/sovereignstack/sovereignstack` — if you create a `sovereignstack` GitHub org
- `github.com/<your-handle>/sovereignstack` — if you publish from your personal account

Changing the module path is a global rewrite; here's the one-liner:

```bash
NEW=github.com/sovereignstack/sovereignstack
OLD=github.com/gayanclife/sovereignstack

go mod edit -module $NEW
grep -rl "$OLD" --include="*.go" . | xargs sed -i "s|$OLD|$NEW|g"
go mod tidy
go build ./...
go test ./...
```

### 2. Remove the developer-assistant context file

`CLAUDE.md` is internal context for Claude Code sessions. Public OSS
repos don't usually carry this. Recommend:

```bash
git rm CLAUDE.md
```

(You can keep a copy in your private dev notes; it just doesn't belong
in the public repo.)

### 3. Cleanup of `models/` and other large committed binaries

The `.gitignore` already excludes them, but pre-existing commits in the
git history may include large model weights. Run:

```bash
# See what's in history, by size
git rev-list --all --objects | \
  git cat-file --batch-check='%(objecttype) %(objectsize) %(rest)' | \
  awk '$1=="blob" && $2 > 1000000 { print }' | sort -k2 -nr | head

# If there are large blobs, rewrite history with git-filter-repo:
#   git filter-repo --path models/ --invert-paths
#   git filter-repo --path-glob '*.safetensors' --invert-paths
#   git push --force-with-lease
```

This is destructive — coordinate with anyone who has cloned the repo.

### 4. License sanity-check

The repo has `LICENSE` already (Apache 2.0). Confirm:
- All `.go` files have the Apache 2.0 SPDX header (most already do)
- Third-party deps in `go.sum` are Apache-2.0-compatible (check with `go-licenses`)

```bash
go install github.com/google/go-licenses@latest
go-licenses report ./... 2>/dev/null | head -20
```

### 5. Initial release tag

Once steps 1–4 are done:

```bash
git tag -a v1.0.0 -m "Initial public release"
git push origin v1.0.0
```

Then on GitHub: edit the release notes from `CHANGELOG.md`, attach
binaries (built via `go build` for linux/amd64, linux/arm64,
darwin/arm64, darwin/amd64).

### 6. Optional: GitHub repo settings

- Enable **Issues**, **Discussions**, **Security advisories**
- Add a **description** matching the README tagline
- Add **topics**: `llm`, `inference`, `vllm`, `gateway`, `self-hosted`,
  `ai`, `ml-ops`, `prometheus`, `oidc`
- **Branch protection** on `main`: require CI passing, require 1 review
- **CodeQL** scanning (Settings → Security → Code scanning → "Set up")
- **Dependabot** for `gomod` (Settings → Code security → Dependabot)

### 7. Domain / contact addresses

`SECURITY.md` and `CODE_OF_CONDUCT.md` reference:
- `security@sovereignstack.io`
- `conduct@sovereignstack.io`

These need to actually receive mail (or change the addresses to ones
you control). Bare-minimum option: a forwarding alias to your own
inbox.

## Final pre-publish run

```bash
# Full test sweep
go test ./...

# Static analysis
go vet ./...
go install honnef.co/go/tools/cmd/staticcheck@latest && staticcheck ./...

# Vulnerability scan
go install golang.org/x/vuln/cmd/govulncheck@latest && govulncheck ./...

# Confirm CI workflow syntax
yamllint .github/workflows/ci.yml || true

# Diff-against-clean
git status
git diff --stat
```

If all green: tag, push, announce. Welcome to OSS.
