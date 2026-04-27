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
