# CLAUDE.md

This file provides guidance to Claude Code when working in this repository.

---

## Cost-Effective Working Rules

These rules apply to every session. Follow them to minimise token usage.

**1. Start from TASKS.md, not the codebase.**
Before reading any code, check `TASKS.md` for the task being worked on. It lists exactly which files to read. Do not explore the codebase freely — go directly to the listed files.

**2. Read only what the task needs.**
Each task in `TASKS.md` has a "Files to read first" section. Read those files and no others unless a specific gap requires it. Do not read files to "understand context" — the task description provides the context.

**3. Read multiple files in parallel.**
When a task requires reading several files, issue all Read tool calls in a single message, not one at a time.

**4. Search before reading.**
If you need to find a function or type in a file you haven't read, use grep to locate the line first. Then read only the relevant section using `offset` and `limit` parameters, not the whole file.

**5. Never re-read a file already read in the session.**
If a file was read earlier in the conversation, use that knowledge. Do not read it again.

**6. Build to verify, don't over-explore.**
After making changes, run `go build -o sovstack .` to verify correctness. Do not read additional files to "double-check" — let the compiler catch errors.

**7. No summaries.**
Do not summarise what you just did at the end of a response. The diff is visible. Skip the recap.

**8. One task per session.**
Each Claude Code session should implement exactly one task from `TASKS.md`. Do not start a second task unless the first is complete and marked `[x]`.

**9. Mark tasks complete.**
When a task is done and the build passes:
1. Update `TASKS.md` to mark it `[x]`
2. Update the corresponding ClickUp task to status `shipped` using the API (see ClickUp Task Map below)

**10. Reference PRD.md for product decisions, not code.**
If a task requires a product decision (naming, behaviour, UX), consult `PRD.md` rather than inferring from existing code patterns.

---

## Code Standards

**Testing Requirements**
Any code change should include relevant tests. This applies to:
- New functions or methods
- Modified behavior or logic changes
- Bug fixes
- New CLI commands or flags

Skip tests only for:
- Documentation-only changes
- Simple refactors with no behavior change (rename, move code)
- Configuration file updates
- Comments or error message improvements

When writing tests:
- Place them in `*_test.go` files alongside the code
- Use Go's standard `testing` package
- Test both happy path and error cases
- Run `go test ./...` to verify before committing

**Copyright Header**
All Go files must include the Apache License 2.0 copyright header at the top. This is required for all new files and when substantially modifying existing files.

Add this to the beginning of every `.go` file:

```go
/*
Copyright 2026 SovereignStack Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package <package_name>
```

---

## Permissions & Execution

**Allowed without prompting:**
- All `sovstack` CLI commands (build, deploy, status, stop, pull, gateway, keys, etc.)
- All bash commands within the workspace (tests, builds, scripts, file operations)
- Docker commands for container management
- Git operations (branch management)

**Always ask before:**
- Creating commits (ask if changes should be committed before running `git commit`)
- Pushing to remote (ask before `git push`)
- Running destructive operations (force push, hard reset, etc.)

**Exception — Large Model Downloads:**
Commands like `sovstack pull <model>` that download large models (>1GB) may be resource-intensive for this machine. Before running such commands, confirm the model size. Skip the download if it would exceed available disk space or cause performance issues. Commands that test auto-pull with small models (distilbert, TinyLlama, etc.) are safe without confirmation.

**These are trusted operations within the project workspace.**

---

## Documentation Standards

**Documentation-Driven Changes**
Whenever functional behavior changes, always update or create relevant documentation in the `docs/` folder. This is a hard requirement, not optional.

**When to Update Docs:**
- CLI command behavior changes (new flags, changed defaults, removed options)
- API endpoint changes (new endpoints, changed request/response format)
- Configuration file structure changes
- User-facing workflow changes (deploy, stop, gateway, etc.)
- Architecture changes that affect deployment or operations
- New features or capability additions

**What to Document:**
1. **Command Reference** (`docs/cli-reference.md`) — all CLI commands, flags, examples
2. **API Reference** (`docs/api-reference.md`) — all HTTP endpoints, request/response formats
3. **Configuration Guide** (`docs/configuration.md`) — config files, environment variables
4. **User Guide** (`docs/user-guide.md`) — step-by-step workflows, common tasks
5. **Architecture** (`docs/architecture.md`) — how components work, Docker setup, hardware requirements

**Process:**
1. Make functional change to code
2. Update relevant doc file(s) in `docs/`
3. If no relevant doc exists, create one with a clear structure
4. Build and test to verify code works
5. Mark task complete in TASKS.md

This ensures users always have accurate, up-to-date documentation.

---

## Commands

```bash
# Build
go build -o sovstack .

# Run directly (development)
go run main.go <command>

# Download dependencies
go mod download

# Test
go test ./...

# Test the CLI after building
./sovstack init
./sovstack status
./sovstack pull <model-name>
./sovstack up <model-name>
./sovstack remove <model-name>
./sovstack gateway
./sovstack keys add <user-id>
```

---

## Project Files

| File | Purpose |
|---|---|
| `TASKS.md` | Master task list — source of truth for what to build next |
| `PRD.md` | Product requirements — consult for product decisions |
| `DESIGN_PROMPT.md` | Claude Design prompt for the Visibility dashboard |
| `models.yaml` | Model registry — edit to add/update models, no code changes needed |

---

## Module

```
github.com/gayanclife/sovereignstack
```

---

## Architecture

SovereignStack is a Go CLI tool (`sovstack`) that deploys private LLM inference servers using vLLM in Docker.

**Deployment data flow:**
1. `cmd/` — Cobra CLI parses flags
2. `internal/provider/onprem/` — OnPrem provider coordinates deployment
3. `core/engine/orchestrator.go` — `EngineRoom` coordinates hardware + model + Docker
4. `internal/hardware/hardware.go` — detects GPU/CPU/RAM via `nvidia-smi` and `/proc/meminfo`
5. `core/model/` — loads model registry, manages cache, selects quantization
6. `internal/docker/vllm.go` — launches vLLM container with GPU bindings
7. `core/gateway/` — optional reverse proxy for auth, rate limiting, audit logging

**Key packages:**

| Package | Role |
|---|---|
| `cmd/` | CLI commands: init, pull, up, status, gateway, keys, remove |
| `core/types.go` | Shared types: ModelMetadata, ModelInstance, ModelCache |
| `core/model/` | Registry loading, cache management, quantization selection |
| `core/engine/orchestrator.go` | EngineRoom — main deployment coordinator |
| `core/gateway/` | Reverse proxy, API key auth, rate limiting |
| `core/audit/` | Audit logger (in-memory → SQLite with AES-256) |
| `internal/hardware/` | GPU/CPU/RAM detection |
| `internal/docker/` | vLLM container lifecycle |
| `internal/provider/` | Provider interface + OnPrem implementation |
| `internal/downloader/` | Hugging Face model download |
| `internal/installer/` | NVIDIA/CUDA/Docker auto-installer |
| `core/model/remote_registry.go` | Hybrid model registry (local + remote) |
| `visibility/` | AI Visibility Platform (Phase 2 commercial layer) |

**Quantization priority:** AWQ > FP16 > GPTQ > INT8 (best quality that fits in VRAM)

---

## Model Registry

Models defined in `models.yaml`. Loader merges sources in this order (later overrides earlier):

1. `models.yaml` — bundled defaults (community-maintained)
2. `/etc/sovereignstack/models.yaml` — system-wide
3. `~/.sovereignstack/models.yaml` — user-specific
4. `./models.local.yaml` — project-local (git-ignored)

---

## Gateway

`sovstack gateway` starts a reverse proxy (`core/gateway/proxy.go`) in front of vLLM. Token-bucket rate limiting per user. Audit logging via `core/audit/`. API keys loaded from `~/.sovereignstack/keys.json`.

---

## Visibility Platform

`sovstack visibility` starts the AI Visibility API server (`visibility/api/server.go`). Exposes operational, financial, usage, security, and compliance data. Protected by the same API key system as the gateway. This is the commercial layer — see `PRD.md` for full specification.

---

## Monitoring

`docker-compose.yml` includes Prometheus + Grafana + node-exporter:
```bash
docker-compose up -d prometheus grafana node-exporter
```
Grafana at `http://localhost:3000` (admin/admin).

---

## ClickUp Integration

When a task is complete, mark it `shipped` in ClickUp using this command (requires `CLICKUP_TOKEN` env var):

```bash
curl -s -X PUT "https://api.clickup.com/api/v2/task/{CLICKUP_TASK_ID}" \
  -H "Authorization: $CLICKUP_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status": "shipped"}'
```

Replace `{CLICKUP_TASK_ID}` with the ID from the table below.

### ClickUp Task Map

| TASK | ClickUp ID |
|---|---|
| TASK-001 | 86exd2w68 |
| TASK-002 | 86exd2w64 |
| TASK-003 | 86exd2w63 |
| TASK-004 | 86exd2w65 |
| TASK-005 | 86exd2w66 |
| TASK-006 | 86exd2w6t |
| TASK-007 | 86exd2w6u |
| TASK-008 | 86exd2w6v |
| TASK-009 | 86exd2w6z |
| TASK-010 | 86exd2w6x |
| TASK-011 | 86exd2wzd |
| TASK-012 | 86exd2wzk |
| TASK-013 | 86exd2wzg |
| TASK-014 | 86exd2wzh |
| TASK-015 | 86exd2wzf |
| TASK-016 | 86exd2wze |

**Available statuses:** `backlog` → `in development` → `testing` → `shipped`

To mark in-progress (optional, when starting a task):
```bash
curl -s -X PUT "https://api.clickup.com/api/v2/task/{CLICKUP_TASK_ID}" \
  -H "Authorization: $CLICKUP_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status": "in development"}'
```
