# Beta release flow (macOS validation)

This is how the macOS beta channel works. It's intentionally separate
from the stable Linux release path so you can iterate on Mac-specific
issues without disrupting Linux users.

## What this is

A long-lived `release/v1.1.0-beta-macos` branch, plus a parallel
GoReleaser config and CI workflow:

```
release/v1.1.0-beta-macos      ← iterate here, tag v1.1.0-beta.N
├── .goreleaser-beta.yaml      ← darwin amd64 + arm64 only
└── .github/workflows/release-beta.yml   ← runs on macos-latest

main                            ← linux v1.0.x stays here
├── .goreleaser.yaml
└── .github/workflows/release.yml
```

Stable releases (`vX.Y.Z` without `-beta`) fire `release.yml` and ship
Linux artefacts. Pre-release tags (`vX.Y.Z-beta.N`) fire
`release-beta.yml` and ship macOS artefacts.

## Why a macOS runner

CGO (used by the sqlite driver) cross-compiling from a Linux runner to
darwin needs osxcross — fiddly, fragile, and costs time per build.
Building on `macos-latest` (arm64 hardware in 2026) lets the Xcode
toolchain handle darwin/amd64 + darwin/arm64 natively. ~30s build
instead of ~10min cross-compile.

## Cutting a beta release

```bash
# On the beta branch
git checkout release/v1.1.0-beta-macos
git pull

# (Make any Mac-specific fixes, commit them.)

# Tag and push. The -beta suffix is what routes this through the
# beta workflow; the .N is the iteration number (start at 1).
git tag -a v1.1.0-beta.1 -m "First macOS beta"
git push origin v1.1.0-beta.1
```

The `release-beta.yml` workflow runs (~3 minutes), produces:

- `sovstack_1.1.0-beta.1_darwin_x86_64.tar.gz`
- `sovstack_1.1.0-beta.1_darwin_arm64.tar.gz`
- `checksums-darwin.txt`

…and creates a **prerelease** at
`https://github.com/gayanclife/sovereignstack/releases/tag/v1.1.0-beta.1`.

## Testing on a Mac

```bash
# Apple silicon
curl -sSL -o sovstack.tar.gz \
  https://github.com/gayanclife/sovereignstack/releases/download/v1.1.0-beta.1/sovstack_1.1.0-beta.1_darwin_arm64.tar.gz

# Or amd64 / Intel:
#   sovstack_1.1.0-beta.1_darwin_x86_64.tar.gz

tar -xzf sovstack.tar.gz
./sovstack version

# Quickstart smoke
./sovstack init
./sovstack keys add alice
./sovstack policy --master-key-file ~/.sovereignstack/master.key &
./sovstack discovery &
./sovstack metrics-proxy &
./sovstack gateway --keys ~/.sovereignstack/keys.json
```

If something doesn't work on macOS but works on Linux, it's a
platform issue worth fixing on the beta branch.

## Iterating

Found a Mac-specific bug? Push a fix to the beta branch and tag the
next iteration:

```bash
git tag -a v1.1.0-beta.2 -m "Fix path expansion on darwin"
git push origin v1.1.0-beta.2
```

Each tag produces a fresh prerelease. Old prereleases stay around for
comparison; delete them in the GitHub UI when noisy.

## Promoting to stable v1.1.0

When the beta is solid:

1. Merge `release/v1.1.0-beta-macos` → `main` via PR (review the
   .goreleaser changes carefully; the *stable* config should grow
   darwin support, not just be replaced)
2. On `main`, edit `.goreleaser.yaml` to include `darwin` in `goos`
   (and re-enable the `brews:` block now that macOS works)
3. Update `.github/workflows/release.yml` to use a matrix with both
   `ubuntu-latest` and `macos-latest`, or split into two jobs
4. Tag `v1.1.0`. The stable workflow ships everything in one release.
5. Delete the beta branch and `.goreleaser-beta.yaml` /
   `release-beta.yml` files

## What's NOT in the beta

- Homebrew tap updates — the `brews:` block stays commented in both
  configs until v1.1.0 stable. The tap repo (`homebrew-tap`) and the
  PAT secret (`HOMEBREW_TAP_GITHUB_TOKEN`) don't need to exist for
  the beta channel.
- Linux artefacts — they keep coming from the v1.0.x line.
- `.deb`/`.rpm`/`.apk` — those are Linux package formats; not
  produced for the macOS-only beta.

## Failure modes and fixes

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| Workflow fails at "Run GoReleaser" with "go.mod toolchain mismatch" | The pre-built `goreleaser` binary was built with an older Go than `go.mod` declares. | Add `install-mode: source` under `goreleaser-action.with` so the runner builds it from source under the runner's Go (matches what we did locally for golangci-lint). |
| `gcc: command not found` on the macos-latest runner | Xcode tools missing on a fresh runner image | Add `- run: xcode-select --install \|\| true` step before GoReleaser. macos-latest images normally have it pre-installed. |
| `cgo: C compiler "clang" not found` | Same root cause as above | Same fix. |
| Build succeeds locally on Mac but the binary panics at runtime on a different macOS version | macOS deployment-target mismatch | Add `MACOSX_DEPLOYMENT_TARGET=11.0` to the `env:` block in `.goreleaser-beta.yaml` (covers Big Sur and newer; suitable for any active machine in 2026). |
