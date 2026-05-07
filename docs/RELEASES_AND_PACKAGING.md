# Releases and packaging

How SovereignStack ships, for both maintainers (cutting a release) and
users (installing a release).

The pipeline is driven by [GoReleaser](https://goreleaser.com) +
GitHub Actions. One YAML file (`.goreleaser.yaml`) plus one workflow
(`.github/workflows/release.yml`) produces:

- Linux binaries (amd64 + arm64), tarballed with LICENSE / README / config
- macOS binaries (amd64 + arm64), same shape
- `.deb`, `.rpm`, and `.apk` packages
- A GitHub Release with all of the above + a SHA-256 checksum file
- An updated formula in `sovereignstack/homebrew-tap`

---

## For users — installing

### Homebrew (macOS, Linux)

```bash
brew tap sovereignstack/tap
brew install sovstack
sovstack --help
```

Brew updates work as you'd expect: `brew upgrade sovstack`.

### apt (Debian, Ubuntu)

There are two paths.

#### Path A — one-shot `.deb` from GitHub Releases (simplest)

```bash
# Replace the version with whatever's current
VERSION=1.0.0
curl -sSL -o sovstack.deb \
  https://github.com/sovereignstack/sovereignstack/releases/download/v${VERSION}/sovstack_${VERSION}_linux_amd64.deb

sudo apt install ./sovstack.deb
```

Pros: zero infrastructure cost, works today.
Cons: `apt upgrade` won't pick up new releases automatically.

#### Path B — a real APT repository (auto-upgrades)

Adding a hosted apt repo lets users do `apt-get install sovstack` and
get future releases via `apt-get upgrade`. There are two practical hosts:

- **[Cloudsmith](https://cloudsmith.io)** — free tier covers OSS projects
  generously. Smallest amount of work.
- **Self-hosted via GitHub Pages** using a tool like
  [`apt-repo-action`](https://github.com/jcansdale-sandbox/apt-repo-action)
  or `aptly` — free, but you maintain the repo metadata.

The README points users at Path A by default; Path B can be added once
the project has more than a handful of installs and someone wants to
own the repo upkeep.

### Direct binary download

For environments without a package manager:

```bash
VERSION=1.0.0
curl -sSL -o sovstack.tar.gz \
  https://github.com/sovereignstack/sovereignstack/releases/download/v${VERSION}/sovstack_${VERSION}_linux_x86_64.tar.gz
tar -xzf sovstack.tar.gz
sudo mv sovstack /usr/local/bin/
```

### Build from source

```bash
git clone https://github.com/sovereignstack/sovereignstack
cd sovereignstack
go install .
```

---

## For maintainers — cutting a release

### One-time setup (prerequisites)

#### 1. Create the Homebrew tap repository

```bash
# In a browser, create an empty public repo:
#   github.com/sovereignstack/homebrew-tap
# Initialize it with just a README. GoReleaser will commit the formula
# to it on every release.
```

The repo name *must* start with `homebrew-` for `brew tap` to find it.
That's why it's `homebrew-tap` and users say `brew tap sovereignstack/tap`.

#### 2. Create a fine-grained personal access token

GoReleaser needs to push to the tap repo. The default `GITHUB_TOKEN`
in Actions can only write to the *current* repo, so we need a separate
token.

In GitHub: **Settings → Developer settings → Personal access tokens
→ Fine-grained tokens → Generate new token**

- Token name: `sovstack-release-tap`
- Repository access: **Only select repositories** → `homebrew-tap`
- Permissions: **Contents: Read and write**
- Expiration: 1 year (re-issue annually)

Copy the token. You won't see it again.

#### 3. Add the token as a repo secret

In the OSS repo: **Settings → Secrets and variables → Actions → New
repository secret**

- Name: `HOMEBREW_TAP_GITHUB_TOKEN`
- Value: the token from step 2

#### 4. (Optional) Sign in to Cloudsmith for hosted apt

If you want Path B above, set up a Cloudsmith account, create an
`opensource/sovstack` repo on their side, and add a small step to the
release workflow that uses the Cloudsmith CLI to upload the `.deb` file.
Deferred until traction warrants it.

### Cutting a release

Once the prerequisites are done, releasing is two commands:

```bash
git tag -a v1.0.0 -m "Initial public release"
git push origin v1.0.0
```

GitHub Actions picks up the tag, runs the test suite, builds binaries
for four targets, signs the checksums, creates the GitHub Release,
publishes the `.deb`/`.rpm`/`.apk` packages, and pushes a fresh
formula to `homebrew-tap`. Total time: about 5 minutes.

Watch progress at:
`https://github.com/sovereignstack/sovereignstack/actions`

### Pre-releases

Tags matching `v1.0.0-rc.1`, `v1.1.0-beta.2`, etc. are auto-marked
as **prereleases** on GitHub. Users have to opt in (`brew install
--HEAD` or download manually); regular `apt-get install` won't see them.

### Verifying a release

After the workflow completes:

1. The release page (`/releases/tag/v1.0.0`) should list 8 archives + a
   `checksums.txt` + the `.deb`/`.rpm`/`.apk` packages.
2. `brew tap sovereignstack/tap && brew install sovstack` should
   succeed on a clean Mac.
3. `dpkg -i sovstack_1.0.0_linux_amd64.deb` should succeed on a clean
   Ubuntu.
4. `sovstack --version` should print the tag and commit SHA.

### Hotfixes

```bash
git tag -a v1.0.1 -m "Fix: argon2 panic on 0-length keys"
git push origin v1.0.1
```

The release pipeline runs again. Brew users get the fix on their next
`brew upgrade`. apt users with Path A re-download; Path B users get
it on `apt-get upgrade`.

### Rolling back a bad release

GoReleaser doesn't have a "delete release" feature, but you can:

1. **Mark the bad tag prerelease** in the GitHub UI (Releases → edit →
   "Set as a pre-release") so apt-from-Releases users don't auto-pull
   it.
2. **Cut the next patch immediately**. `v1.0.1` reverting the change
   takes ~5 minutes.
3. **Manually edit the homebrew-tap formula** to point at the previous
   stable version if Brew users were affected. The next clean release
   will rewrite it.

---

## CI gates that prevent bad releases

The release workflow runs `go test ./...` before building anything.
A failing test stops the release at zero-cost. The regular `ci.yml`
workflow runs on every PR; that catches most issues before they reach
a tag.

Extra gates worth adding once the project has more contributors:

- **Signed commits** required for the tag (Settings → Branches → require signed commits)
- **Required reviewers** on PRs that touch `core/gateway/proxy.go`,
  `core/keys/store.go`, or `core/management/policy/`
- **CodeQL** scanning + Dependabot — GitHub-native, takes one click each

---

## Costs

- **GitHub Releases** — free for public repos
- **Homebrew tap repo** — free
- **GitHub Actions for the release pipeline** — free for public repos
  (2,000 minutes/month for private; we'd use ~20 minutes per release)
- **Cloudsmith for apt repo (Path B)** — free OSS tier

Total ongoing cost: $0 until you outgrow the free tiers, which won't
be soon.
