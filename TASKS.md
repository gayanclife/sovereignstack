# SovereignStack — Implementation Tasks

## How to Use This File

Each task below is self-contained. To work on a task with Claude Code:

```
"Read TASKS.md and implement TASK-XXX. Read the files listed under 'Files' before making changes."
```

Mark tasks `[x]` when complete. Tasks within the same phase with no listed dependencies can run in parallel (separate Claude Code sessions or worktrees).

---

## Status Overview

| ID | Title | Phase | Priority | Status | Depends On |
|---|---|---|---|---|---|
| TASK-001 | Real Hugging Face model download | 1 | Critical | [x] | — |
| TASK-002 | Complete the deploy pipeline | 1 | Critical | [x] | TASK-001 |
| TASK-003 | Fix status command bug | 1 | High | [ ] | — |
| TASK-004 | Upgrade init to auto-install prerequisites | 1 | High | [x] | — |
| TASK-005 | Unify the two cache systems | 1 | High | [ ] | TASK-001 |
| TASK-006 | Encrypted SQLite audit log | 1 | High | [ ] | — |
| TASK-007 | Gateway API key management | 1 | Medium | [ ] | — |
| TASK-008 | Update model registry to current models | 1 | Medium | [ ] | — |
| TASK-009 | Provider interface + OnPrem implementation | 1 | Medium | [ ] | TASK-002 |
| TASK-010 | Integration tests (blank → running endpoint) | 1 | Medium | [ ] | TASK-002 |
| TASK-011 | Visibility: metrics collection agent | 2 | High | [ ] | TASK-002 |
| TASK-012 | Visibility: financial comparison engine | 2 | High | [ ] | TASK-011 |
| TASK-013 | Visibility: usage analytics + team attribution | 2 | Medium | [ ] | TASK-011 |
| TASK-014 | Visibility: compliance report export | 2 | Medium | [ ] | TASK-006 |
| TASK-015 | Visibility: security anomaly detection | 2 | Medium | [ ] | TASK-011 |
| TASK-016 | Visibility: REST API + dashboard backend | 2 | High | [ ] | TASK-011 |

---

## PHASE 1 — Complete the Open-Source CLI

---

### TASK-001: Real Hugging Face Model Download

**Why:** `sovstack pull` is a stub. It creates a directory and a `model.json` placeholder but downloads nothing. This is the most important missing feature — everything else depends on real models being on disk.

**Files to read first:**
- `core/model/cache.go` — `CacheManager.downloadFromHuggingFace()` is the stub to replace
- `cmd/pull.go` — calls `cm.DownloadModel()`, shows the expected UX

**What to build:**

Replace `downloadFromHuggingFace()` in `core/model/cache.go` with a real implementation using the Hugging Face HTTP API. Do not use Python, `git lfs`, or external CLI tools. Pure Go using standard library + HTTP.

The Hugging Face API works like this:
1. Fetch model file list: `GET https://huggingface.co/api/models/{model_id}`
   - Response includes a `siblings` array where each entry has a `rfilename` field (the file path)
2. Download each file: `GET https://huggingface.co/{model_id}/resolve/main/{rfilename}`
   - Stream the response body to disk, showing download progress
3. If the request requires authentication (gated models like Llama 3), read `HF_TOKEN` from environment

**Implementation requirements:**
- Stream downloads to disk (do not load entire model into memory — models are 3–70GB)
- Show a progress bar per file: `filename [=====>    ] 45% 2.3 GB/5.1 GB`
- Support resuming interrupted downloads (check if file already exists and has correct size)
- If `HF_TOKEN` env var is set, include it as `Authorization: Bearer {token}` header
- Skip files that are not model weights (`.gitattributes`, `README.md` are fine to skip or download)
- After all files downloaded, verify at least one `.safetensors` or `.bin` file exists

**New file to create:** `internal/downloader/huggingface.go`
```
Package: downloader
Key types:
  - HFDownloader struct (holds token, cacheDir, httpClient)
  - DownloadModel(modelID string, destDir string) error
  - fetchFileList(modelID string) ([]string, error)
  - downloadFile(url string, destPath string, showProgress bool) error
```

**Update:** `core/model/cache.go`
- Replace `downloadFromHuggingFace()` body to call `downloader.HFDownloader.DownloadModel()`
- Add `HF_TOKEN` check and print a helpful message if a 401 is returned (gated model)

**Add dependency to go.mod** (if needed for progress bar):
- Use `github.com/schollz/progressbar/v3` for the progress display

**Test model:** Use `distilbert-base-uncased` — small (~250MB), no HF token required, fast to download. Verify all acceptance criteria against this model before testing larger ones.

**Acceptance criteria:**
- [ ] `sovstack pull distilbert-base-uncased` downloads real files from Hugging Face
- [ ] Progress bar shows during download
- [ ] Re-running `pull` on an already-downloaded model says "already cached" and exits
- [ ] `HF_TOKEN` env var is used if present
- [ ] If model is gated (401), prints: "This model requires a Hugging Face token. Set HF_TOKEN=your_token"
- [ ] Downloaded files are present on disk after command completes

---

### TASK-002: Complete the Deploy Pipeline

**Why:** `sovstack deploy <model>` validates hardware compatibility but never actually deploys the model. The `EngineRoom.Deploy()` method exists in `core/engine/orchestrator.go` and is fully implemented — it just never gets called.

**Files to read first:**
- `cmd/deploy.go` — currently calls `er.GetSuitableModels()` and prints results, never calls `er.Deploy()`
- `core/engine/orchestrator.go` — `Deploy()` method is implemented, needs to be called
- `internal/docker/vllm.go` — `VLLMOrchestrator.Start()` runs the actual Docker container

**What to fix:**

In `cmd/deploy.go`, after the hardware compatibility check passes, add:

```go
ctx := context.Background()

// Plan the deployment first so user sees what will happen
plan, err := er.PlanDeployment(ctx, modelName)
if err != nil {
    fmt.Printf("✗ Cannot plan deployment: %v\n", err)
    return
}

fmt.Printf("\nDeployment Plan:\n")
fmt.Printf("  Model:         %s\n", plan.ModelName)
fmt.Printf("  Quantization:  %s\n", plan.Quantization)
fmt.Printf("  Required VRAM: %.1f GB\n", float64(plan.RequiredVRAM)/(1024*1024*1024))
fmt.Printf("  Available VRAM: %.1f GB\n", float64(plan.AvailableVRAM)/(1024*1024*1024))
fmt.Printf("  Notes: %s\n\n", plan.Notes)

fmt.Println("Starting deployment...")

if err := er.Deploy(ctx, modelName, nil); err != nil {
    fmt.Printf("✗ Deployment failed: %v\n", err)
    return
}

fmt.Printf("✓ Model deployed successfully\n")
fmt.Printf("  API endpoint: http://localhost:8000/v1/chat/completions\n")
fmt.Printf("  Run 'sovstack gateway' to start the secure proxy\n")
```

**Also fix:** `cmd/deploy.go` has a dead variable `modelMetadata := er.GetSystemInfo()` — this is `*hardware.SystemHardware`, not model metadata. The variable name is misleading. The GPU check below uses it correctly but it should be renamed to `sysInfo`.

**Add flags to deploy command:**
- `--port` (default: 8000) — port to expose vLLM on
- `--quantization` (default: auto) — override quantization (none/awq/gptq/int8)

**Acceptance criteria:**
- [ ] `sovstack deploy distilbert-base-uncased` starts a Docker container
- [ ] Deployment plan is printed before deploying
- [ ] Container health check passes before command exits
- [ ] Success message shows the API endpoint URL
- [ ] `--port` flag works
- [ ] `--quantization` flag works
- [ ] Incompatible model prints a clear error with alternatives

---

### TASK-003: Fix Status Command Bug

**Why:** `cmd/status.go` has a bug. It calls `os.Stat(cacheDir)` and stores the result as `hw`, then later tries to use it as if it's hardware info. The hardware section shows no actual hardware data.

**Files to read first:**
- `cmd/status.go` — full file, see the `hw` variable misuse on line 38

**What to fix:**

1. **Remove the broken hardware section** — `os.Stat()` returns `os.FileInfo`, not hardware info. The variable `hw` is used to check if cacheDir exists, then incorrectly referenced at line 86 for hardware display.

2. **Add real hardware info** to the status output using `hardware.GetSystemHardware()`:
```
🖥️  Hardware
  GPUs: 1x NVIDIA RTX 4090 (24 GB VRAM)
  CPU:  16 cores
  RAM:  64.0 GB
  CUDA: 12.1
  Docker: ✓ installed
```

3. **Add running containers section** using `docker ps` to show which models are currently deployed:
```
🚀 Running Models
  vllm-mistralai/Mistral-7B-v0.3  →  http://localhost:8000  (up 2h)
```

4. **Fix the cache dir existence check** — use the `os.IsNotExist(err)` check correctly, not the `hw` variable.

**Files to modify:**
- `cmd/status.go` — full rewrite of `runStatus()`

**Acceptance criteria:**
- [ ] `sovstack status` compiles and runs without errors
- [ ] Hardware section shows real GPU/CPU/RAM data
- [ ] Running models section shows any active vLLM containers
- [ ] Cached models section unchanged in functionality
- [ ] Handles no-GPU gracefully (shows CPU info)

---

### TASK-004: Upgrade Init to Auto-Install Prerequisites

**Why:** `sovstack init` currently only detects hardware and prints what's missing. It should actually fix what's missing. A DevOps engineer should be able to run `sovstack init` on a blank Ubuntu server and have everything ready.

**Files to read first:**
- `cmd/init.go` — currently just detects, does not install
- `internal/hardware/hardware.go` — `CheckDocker()`, `CheckCUDA()`, `DetectGPUs()`

**What to build:**

`sovstack init` should run a pre-flight check, then offer to fix anything missing:

```
Running pre-flight checks...

✓ NVIDIA GPU detected: RTX 4090 (24 GB VRAM)
✗ CUDA not installed
✗ NVIDIA Container Toolkit not installed
✓ Docker installed
✓ System RAM: 64 GB

2 issues found. Fix automatically? [y/N]:
```

If user confirms:
1. Install CUDA toolkit (Ubuntu only for MVP): `apt-get install -y cuda-toolkit-12-1`
2. Install NVIDIA Container Toolkit:
   ```
   curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor ...
   apt-get install -y nvidia-container-toolkit
   sudo systemctl restart docker
   ```
3. Re-run checks and confirm all green

**New file:** `internal/installer/installer.go`
```
Package: installer
Functions:
  - InstallCUDA(version string) error          // runs apt-get install
  - InstallNvidiaContainerToolkit() error      // adds nvidia repo + installs
  - InstallDocker() error                      // installs docker if missing
  - VerifyInstallation() (*InstallStatus, error)
  
Type InstallStatus:
  - CUDAInstalled bool
  - CUDAVersion string
  - ContainerToolkitInstalled bool
  - DockerInstalled bool
  - DockerVersion string
```

**Implementation notes:**
- All install commands must run with `sudo` — check if user has sudo access first
- If not on Ubuntu/Debian, print: "Auto-install only supported on Ubuntu/Debian. Manual instructions: [URL]"
- Detect OS via `/etc/os-release`
- Print each install command before running it (transparency)
- If install fails, print the exact error and the manual command to run

**Acceptance criteria:**
- [ ] `sovstack init` passes on a machine with all prerequisites
- [ ] `sovstack init` offers to fix missing prerequisites
- [ ] Install actually works on Ubuntu 22.04 (test in Docker)
- [ ] Non-Ubuntu systems get manual instructions instead of auto-install
- [ ] `sovstack init --check` flag runs checks only, no install prompts

---

### TASK-005: Unify the Two Cache Systems

**Why:** There are currently two separate cache implementations that do overlapping things and create confusion:
- `core/model/cache.go` — `CacheManager` with `.metadata.json` file (used by `pull` and `status`)
- `core/model/manager.go` — `Manager` with its own cache map (used by `deploy`)

They do not share data. A model pulled via `CacheManager` is invisible to `Manager.ValidateModel()`, which means `deploy` will always say "model not cached locally" even after a successful `pull`.

**Files to read first:**
- `core/model/cache.go` — the `CacheManager` implementation
- `core/model/manager.go` — the `Manager` implementation
- `cmd/pull.go` — uses `CacheManager`
- `cmd/deploy.go` — uses `Manager` via `EngineRoom`

**What to fix:**

Option: Make `Manager` the single source of truth. Update `Manager.ValidateModel()` to check for the model directory existence directly (which it already does) AND cross-reference the `.metadata.json` file from `CacheManager`.

Simpler approach: Update `Manager.ValidateModel()` to check if the model directory exists AND contains at least one `.safetensors` or `.bin` file (proof of real download). Remove the separate metadata check entirely.

```go
func (m *Manager) ValidateModel(modelName string) error {
    localPath := filepath.Join(m.cacheDir, modelName)
    
    // Check directory exists
    if _, err := os.Stat(localPath); os.IsNotExist(err) {
        return fmt.Errorf("model not cached: %s. Run: sovstack pull %s", modelName, modelName)
    }
    
    // Check it contains actual model files (not just a placeholder)
    hasModelFiles := false
    filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {
        if strings.HasSuffix(path, ".safetensors") || strings.HasSuffix(path, ".bin") {
            hasModelFiles = true
        }
        return nil
    })
    
    if !hasModelFiles {
        return fmt.Errorf("model directory exists but contains no model files: run 'sovstack pull %s'", modelName)
    }
    
    return nil
}
```

Also update `Manager.GetModelPath()` to use the same logic.

**Acceptance criteria:**
- [ ] `sovstack pull <model>` followed by `sovstack deploy <model>` works end-to-end
- [ ] `sovstack status` and `sovstack deploy` see the same cached models
- [ ] No duplicate metadata files or conflicting state

---

### TASK-006: Encrypted SQLite Audit Log

**Why:** The audit logger (`core/audit/logger.go`) is in-memory only. Logs are lost when the gateway restarts. Regulated industries require persistent, tamper-evident audit logs.

**Files to read first:**
- `core/audit/logger.go` — current in-memory implementation and the `AuditLog` struct
- `cmd/gateway.go` — creates `audit.NewLogger(auditBuffer)` — this call needs to change

**What to build:**

New file: `core/audit/sqlite.go`

```go
type SQLiteLogger struct {
    db  *sql.DB
    key []byte  // AES-256 encryption key
}

func NewSQLiteLogger(dbPath string, encryptionKey string) (*SQLiteLogger, error)
func (l *SQLiteLogger) Log(entry AuditLog)
func (l *SQLiteLogger) GetLogs(n int) []AuditLog
func (l *SQLiteLogger) GetLogsByUser(user string) []AuditLog
func (l *SQLiteLogger) GetLogsByModel(model string) []AuditLog
func (l *SQLiteLogger) GetLogsInTimeRange(start, end time.Time) []AuditLog
func (l *SQLiteLogger) GetStats() map[string]interface{}
```

**Implementation notes:**
- Use `modernc.org/sqlite` (pure Go, no CGO required — keeps single binary)
- Encrypt each row's `request_body` and `response_body` fields with AES-256-GCM
- Other fields (timestamp, user, model, status_code, duration) stored in plaintext for queryability
- Schema:
  ```sql
  CREATE TABLE IF NOT EXISTS audit_logs (
      id            TEXT PRIMARY KEY,
      timestamp     DATETIME NOT NULL,
      event_type    TEXT NOT NULL,
      level         TEXT NOT NULL,
      user          TEXT,
      model         TEXT,
      method        TEXT,
      endpoint      TEXT,
      request_size  INTEGER,
      response_size INTEGER,
      tokens_used   INTEGER,
      duration_ms   INTEGER,
      status_code   INTEGER,
      error_message TEXT,
      ip_address    TEXT,
      user_agent    TEXT,
      correlation_id TEXT
  );
  CREATE INDEX IF NOT EXISTS idx_user ON audit_logs(user);
  CREATE INDEX IF NOT EXISTS idx_model ON audit_logs(model);
  CREATE INDEX IF NOT EXISTS idx_timestamp ON audit_logs(timestamp);
  ```
- Derive encryption key using PBKDF2 from the `encryptionKey` string + a stored salt
- If `dbPath` is empty, fall back to in-memory logger (backward compat)
- The `Logger` in `logger.go` and `SQLiteLogger` should both implement a `AuditLogger` interface

**Add to go.mod:** `modernc.org/sqlite`

**Update `cmd/gateway.go`:**
- Add `--audit-db` flag (default: `./sovstack-audit.db`)
- Add `--audit-key` flag (reads from `SOVSTACK_AUDIT_KEY` env var, or generates and prints one on first run)
- Use `SQLiteLogger` when `--audit-db` is set

**Acceptance criteria:**
- [ ] Audit logs persist to SQLite after gateway restarts
- [ ] `GET /api/audit/logs` returns logs from SQLite
- [ ] Logs survive a gateway restart
- [ ] AES-256 encryption verified (raw SQLite file has no plaintext request bodies)
- [ ] Binary stays single-file (no CGO, uses `modernc.org/sqlite`)
- [ ] `--audit-db` and `--audit-key` flags work

---

### TASK-007: Gateway API Key Management

**Why:** Gateway currently has two hardcoded test API keys (`sk_test_123`, `sk_demo_456`). There is no way for a user to add, remove, or list their own keys without editing code.

**Files to read first:**
- `cmd/gateway.go` — hardcoded keys at lines 49–51
- `core/gateway/auth.go` — `APIKeyAuthProvider` implementation

**What to build:**

New subcommand group: `sovstack keys`

```bash
sovstack keys add <user-id>        # generates and prints a new API key for that user
sovstack keys list                  # lists all users with keys (not the key values)
sovstack keys remove <user-id>      # revokes a user's key
```

Store keys in a JSON file at `~/.sovereignstack/keys.json` (or `--keys-file` flag):
```json
{
  "test-user": "sk_abc123...",
  "hr-team": "sk_def456..."
}
```

**Update `cmd/gateway.go`:**
- Remove the two hardcoded test keys
- Load keys from `~/.sovereignstack/keys.json` at startup
- Print how many keys were loaded: "Loaded 3 API keys"
- If no keys file found: "No API keys configured. Add one with: sovstack keys add <user-id>"

**Acceptance criteria:**
- [ ] `sovstack keys add alice` generates and prints a key
- [ ] Gateway loads keys from file on startup
- [ ] `sovstack keys list` shows users (not key values)
- [ ] `sovstack keys remove alice` revokes the key
- [ ] No hardcoded test keys in the binary

---

### TASK-008: Update Model Registry to Current Models

**Why:** `models.yaml` lists Llama 2 (2023) and Mistral 7B v0.1 (2023). Both are significantly outdated. Current models are Llama 3.1/3.2 and Mistral v0.3. This is the face of the product.

**Files to modify:**
- `models.yaml` — replace outdated models, add current ones

**Replace gpu_models with:**
```yaml
gpu_models:
  - name: meta-llama/Llama-3.1-8B-Instruct
    repo: meta-llama/Llama-3.1-8B-Instruct
    description: "Meta Llama 3.1 8B Instruct — best 8B model, requires HF token"
    parameters: "8B"
    context_length: 131072
    hardware_target: gpu
    minimum_system_ram_gb: 0
    sizes:
      none: 16000000000
      int8: 8000000000
      awq: 4500000000
    required_vram_gb:
      none: 16
      int8: 9
      awq: 5
    default_quantization: awq
    tags: ["llm", "chat", "instruct", "production", "8b", "llama3"]

  - name: meta-llama/Llama-3.1-70B-Instruct
    repo: meta-llama/Llama-3.1-70B-Instruct
    description: "Meta Llama 3.1 70B Instruct — near GPT-4 quality, requires HF token"
    parameters: "70B"
    context_length: 131072
    hardware_target: gpu
    minimum_system_ram_gb: 0
    sizes:
      none: 140000000000
      awq: 40000000000
    required_vram_gb:
      none: 140
      awq: 40
    default_quantization: awq
    tags: ["llm", "chat", "instruct", "production", "70b", "llama3"]

  - name: mistralai/Mistral-7B-Instruct-v0.3
    repo: mistralai/Mistral-7B-Instruct-v0.3
    description: "Mistral 7B v0.3 Instruct — fast, high quality, 32k context"
    parameters: "7B"
    context_length: 32768
    hardware_target: gpu
    minimum_system_ram_gb: 0
    sizes:
      none: 14000000000
      int8: 7500000000
      awq: 4000000000
    required_vram_gb:
      none: 14
      int8: 8
      awq: 4
    default_quantization: awq
    tags: ["llm", "chat", "instruct", "production", "7b", "mistral"]

  - name: mistralai/Mixtral-8x7B-Instruct-v0.1
    repo: mistralai/Mixtral-8x7B-Instruct-v0.1
    description: "Mixtral 8x7B — MoE model, GPT-3.5 quality at 7B inference cost"
    parameters: "47B"
    context_length: 32768
    hardware_target: gpu
    minimum_system_ram_gb: 0
    sizes:
      awq: 24000000000
    required_vram_gb:
      awq: 24
    default_quantization: awq
    tags: ["llm", "chat", "instruct", "moe", "production"]

  - name: Qwen/Qwen2.5-7B-Instruct
    repo: Qwen/Qwen2.5-7B-Instruct
    description: "Qwen 2.5 7B Instruct — top-ranked open model, strong coding ability"
    parameters: "7B"
    context_length: 131072
    hardware_target: gpu
    minimum_system_ram_gb: 0
    sizes:
      none: 15000000000
      awq: 4500000000
    required_vram_gb:
      none: 15
      awq: 5
    default_quantization: awq
    tags: ["llm", "chat", "instruct", "coding", "7b"]
```

**Replace cpu_models with:**
```yaml
cpu_models:
  - name: microsoft/Phi-3-mini-4k-instruct
    repo: microsoft/Phi-3-mini-4k-instruct
    description: "Microsoft Phi-3 Mini — high quality small model, CPU-friendly"
    parameters: "3.8B"
    context_length: 4096
    hardware_target: cpu
    minimum_system_ram_gb: 8
    sizes:
      none: 7600000000
      int8: 3800000000
    required_vram_gb:
      none: 0
      int8: 0
    default_quantization: int8
    tags: ["cpu", "llm", "chat", "small", "microsoft"]

  - name: TinyLlama/TinyLlama-1.1B-Chat-v1.0
    repo: TinyLlama/TinyLlama-1.1B-Chat-v1.0
    description: "TinyLlama 1.1B Chat — minimal RAM, runs on anything"
    parameters: "1.1B"
    context_length: 2048
    hardware_target: cpu
    minimum_system_ram_gb: 3
    sizes:
      none: 2200000000
      int8: 1100000000
    required_vram_gb:
      none: 0
      int8: 0
    default_quantization: int8
    tags: ["cpu", "llm", "chat", "tiny", "minimal"]

  - name: google/gemma-2-2b-it
    repo: google/gemma-2-2b-it
    description: "Google Gemma 2 2B Instruct — best-in-class 2B model"
    parameters: "2B"
    context_length: 8192
    hardware_target: cpu
    minimum_system_ram_gb: 5
    sizes:
      none: 4500000000
      int8: 2200000000
    required_vram_gb:
      none: 0
      int8: 0
    default_quantization: int8
    tags: ["cpu", "llm", "chat", "google", "gemma"]
```

**Acceptance criteria:**
- [ ] `models.yaml` parses without errors (`go run main.go status`)
- [ ] `sovstack init` shows new model list
- [ ] No Llama 2 or Mistral v0.1 references remain

---

### TASK-009: Provider Interface + OnPrem Implementation

**Why:** The code goes directly from CLI → `EngineRoom` → Docker. The PRD specifies a `Provider` interface so the same CLI can later target AWS, Azure, GCP. This is the abstraction that makes SovereignStack "deploy anywhere."

**Files to read first:**
- `core/engine/orchestrator.go` — `EngineRoom` struct, this becomes the OnPrem provider
- `internal/docker/vllm.go` — already handles Docker specifics
- `PRD.md` — Provider interface specification

**What to build:**

New file: `internal/provider/interface.go`
```go
package provider

type Config struct {
    ModelCacheDir      string
    Port               int
    VRAMLimit          int64
    GPUIndices         []int
    EnableAuditLogging bool
    AuditDBPath        string
}

type ModelConfig struct {
    Name         string
    Quantization string
    Port         int
}

type NodeStatus struct {
    NodeID           string
    Status           string    // "running", "stopped", "error"
    GPUUtilization   []float32
    VRAMUsedBytes    []int64
    ActiveModels     []string
    RequestsPerSec   float32
    UptimeSeconds    int64
}

type Provider interface {
    Initialize(ctx context.Context, config Config) error
    RunModel(ctx context.Context, modelConfig ModelConfig) error
    StopModel(ctx context.Context, modelName string) error
    Status(ctx context.Context) (*NodeStatus, error)
    GetLogs(ctx context.Context, modelName string, lines int) ([]string, error)
}
```

New file: `internal/provider/onprem/provider.go`
- Wrap the existing `EngineRoom` functionality into the `Provider` interface
- `Initialize()` calls `hardware.GetSystemHardware()` and validates Docker is running
- `RunModel()` calls `EngineRoom.Deploy()`
- `Status()` calls `EngineRoom.Status()` and maps to `NodeStatus`
- `GetLogs()` calls `VLLMOrchestrator.GetLogs()`

**Acceptance criteria:**
- [ ] `internal/provider/interface.go` compiles
- [ ] `OnPremProvider` implements all interface methods
- [ ] `cmd/deploy.go` uses `OnPremProvider` instead of `EngineRoom` directly
- [ ] Behaviour is identical to current deploy flow

---

### TASK-010: Integration Test — Blank to Running Endpoint

**Why:** No tests exist. The single most important test is end-to-end: does the stack actually work?

**Files to read first:**
- `go.mod` — current dependencies
- `internal/docker/vllm.go` — what the container setup looks like

**What to build:**

New file: `tests/integration/deploy_test.go`

Use `testcontainers-go` to:
1. Start a minimal test container (use `hello-world` or a tiny mock HTTP server, not a real LLM — tests must run in CI without a GPU)
2. Verify `VLLMOrchestrator.Start()` creates and starts a container
3. Verify `VLLMOrchestrator.IsRunning()` returns true
4. Verify `VLLMOrchestrator.Stop()` stops it
5. Verify `VLLMOrchestrator.Remove()` removes it

Also add unit tests for:
- `core/model/quantization.go` — `SuggestQuantization()` with various VRAM amounts
- `core/gateway/proxy.go` — rate limiter allows/blocks correctly
- `core/audit/sqlite.go` — log persists and is readable after re-open

**Add dependency:** `github.com/testcontainers/testcontainers-go`

**Acceptance criteria:**
- [ ] `go test ./...` passes
- [ ] Tests do not require a GPU or real LLM
- [ ] Integration test creates and destroys a real Docker container
- [ ] Quantization unit tests cover: fits in VRAM, doesn't fit, boundary cases
- [ ] Rate limiter tests cover: under limit, at limit, over limit, refill

---

## PHASE 2 — AI Visibility Platform

---

### TASK-011: Visibility Metrics Collection Agent

**Why:** This is the foundation of the commercial layer. Every SovereignStack deployment emits metrics — GPU utilization, request rates, latency, VRAM usage. Without collecting these, none of the visibility features are possible.

**Files to read first:**
- `core/audit/logger.go` — `AuditLog` struct has request/response metrics
- `internal/hardware/hardware.go` — `GetSystemHardware()` for GPU metrics
- `internal/docker/vllm.go` — `GetLogs()` for container state

**What to build:**

New package: `visibility/collector/`

```
visibility/
  collector/
    agent.go        — MetricsAgent struct, Start(), Stop(), collect loop
    gpu.go          — GPU metrics via nvidia-smi (utilization %, temp, power)
    inference.go    — Inference metrics from vLLM /metrics endpoint (Prometheus format)
    system.go       — CPU, RAM, disk metrics
  store/
    store.go        — TimeSeriesStore interface
    sqlite.go       — SQLite implementation (5-minute resolution, 90-day retention)
  api/
    server.go       — HTTP API server for the visibility dashboard
```

**`MetricsAgent`** runs in the background when `sovstack up` is called:
- Every 15 seconds: collect GPU utilization, VRAM used, temperature, power draw
- Every 15 seconds: scrape vLLM `/metrics` endpoint for tokens/sec, queue depth, latency
- Every 60 seconds: collect system CPU, RAM, disk usage
- Store all metrics to `TimeSeriesStore`

**Metrics to collect:**
```go
type GPUSnapshot struct {
    Timestamp      time.Time
    GPUIndex       int
    UtilizationPct float32
    VRAMUsedBytes  int64
    VRAMTotalBytes int64
    TemperatureC   float32
    PowerWatts     float32
}

type InferenceSnapshot struct {
    Timestamp          time.Time
    ModelName          string
    RequestsPerSec     float32
    TokensPerSec       float32
    P50LatencyMS       float32
    P99LatencyMS       float32
    QueueDepth         int
    ActiveRequests     int
}

type SystemSnapshot struct {
    Timestamp    time.Time
    CPUPct       float32
    RAMUsedBytes int64
    RAMTotalBytes int64
    DiskUsedBytes int64
}
```

**Acceptance criteria:**
- [ ] Agent starts automatically when `sovstack up` runs
- [ ] GPU metrics collected every 15 seconds and stored to SQLite
- [ ] Inference metrics scraped from vLLM `/metrics` endpoint
- [ ] Metrics survive gateway restarts (persistent SQLite)
- [ ] `GET /api/visibility/metrics?hours=24` returns time series data

---

### TASK-012: Financial Comparison Engine

**Why:** This is the "killer demo" feature from the original vision. Showing an enterprise exactly how much money they saved vs. OpenAI or AWS Bedrock is the most compelling sales tool.

**Files to read first:**
- `visibility/collector/inference.go` — token counts are collected here
- `visibility/store/sqlite.go` — where to read historical token data from

**What to build:**

New file: `visibility/financial/calculator.go`

```go
// Current pricing (update quarterly)
var CloudPricing = map[string]ModelPricing{
    "openai/gpt-4o": {
        InputPer1MTokens:  5.00,   // USD
        OutputPer1MTokens: 15.00,
    },
    "openai/gpt-4o-mini": {
        InputPer1MTokens:  0.15,
        OutputPer1MTokens: 0.60,
    },
    "openai/gpt-3.5-turbo": {
        InputPer1MTokens:  0.50,
        OutputPer1MTokens: 1.50,
    },
    "aws/claude-3-sonnet": {
        InputPer1MTokens:  3.00,
        OutputPer1MTokens: 15.00,
    },
    "aws/llama3-8b": {
        InputPer1MTokens:  0.30,
        OutputPer1MTokens: 0.60,
    },
}

type CostReport struct {
    PeriodStart        time.Time
    PeriodEnd          time.Time
    TotalInputTokens   int64
    TotalOutputTokens  int64
    LocalCostUSD       float64   // electricity + hardware amortization
    CloudAlternatives  []CloudCost
    SavingsUSD         float64   // vs cheapest comparable cloud option
    SavingsPct         float64
}

type CloudCost struct {
    Provider    string
    ModelName   string
    CostUSD     float64
}

func CalculateCostReport(store TimeSeriesStore, period time.Duration, 
    hardwareCostPerHour float64) (*CostReport, error)
```

**Also build:** `visibility/financial/hardware.go`
- Estimate electricity cost: GPU TDP × hours × electricity rate (configurable, default $0.12/kWh)
- Amortize hardware cost: `--hardware-cost` flag (e.g., "$5000") amortized over 3 years

**API endpoint:** `GET /api/visibility/costs?period=30d`
```json
{
  "period": "last_30_days",
  "total_tokens": 150000000,
  "local_cost_usd": 42.50,
  "cloud_alternatives": [
    {"provider": "OpenAI", "model": "gpt-4o", "cost_usd": 1125.00},
    {"provider": "OpenAI", "model": "gpt-4o-mini", "cost_usd": 112.50},
    {"provider": "AWS Bedrock", "model": "llama3-8b", "cost_usd": 67.50}
  ],
  "savings_usd": 25.00,
  "savings_pct": 40
}
```

**Acceptance criteria:**
- [ ] `GET /api/visibility/costs` returns a cost report
- [ ] Cost calculation uses real token counts from the metrics store
- [ ] Hardware/electricity cost is configurable
- [ ] Pricing table is updateable without code changes (JSON config file)
- [ ] Report covers customizable time periods (24h, 7d, 30d, 90d, all-time)

---

### TASK-013: Usage Analytics and Team Attribution

**Why:** Enterprises need to know which teams are using the AI, how much, and for what. This is the "who is doing what" visibility layer.

**Files to read first:**
- `core/audit/sqlite.go` — every request has a `user` field (the API key owner)
- `core/gateway/auth.go` — `APIKeyAuthProvider` maps key → userID
- `cmd/gateway.go` — keys are loaded from file

**What to build:**

Extend the keys file to support team/department metadata:
```json
{
  "alice": {
    "key": "sk_abc123",
    "team": "hr",
    "department": "Human Resources",
    "created": "2026-04-01T00:00:00Z"
  }
}
```

New file: `visibility/analytics/usage.go`

```go
type UsageReport struct {
    Period      string
    ByUser      []UserUsage
    ByTeam      []TeamUsage
    ByModel     []ModelUsage
    PeakHours   []HourlyUsage
    TotalRequests int64
    TotalTokens   int64
}

type UserUsage struct {
    UserID      string
    Team        string
    Requests    int64
    Tokens      int64
    AvgLatencyMS float32
    LastSeen    time.Time
}

type TeamUsage struct {
    Team        string
    Department  string
    Requests    int64
    Tokens      int64
    TopModel    string
}
```

**API endpoints:**
- `GET /api/visibility/usage?period=7d` — full usage report
- `GET /api/visibility/usage/users` — per-user breakdown
- `GET /api/visibility/usage/teams` — per-team breakdown  
- `GET /api/visibility/usage/models` — per-model breakdown

**Acceptance criteria:**
- [ ] Usage is attributed to users and teams correctly
- [ ] `GET /api/visibility/usage` returns accurate counts from audit log
- [ ] Peak hour analysis shows busiest times of day
- [ ] Unused model detection: models deployed but with 0 requests in 7 days

---

### TASK-014: Compliance Report Export

**Why:** A compliance officer needs to hand a report to an auditor. The report needs to be exportable, dated, and cover a specific period. This is the direct revenue driver for regulated industries.

**Files to read first:**
- `core/audit/sqlite.go` — source of all audit data
- `PRD.md` — compliance requirements section

**What to build:**

New file: `visibility/reports/compliance.go`

Support three report formats:
1. **JSON** — machine-readable, for integration with other compliance tools
2. **CSV** — for import into Excel, audit tools
3. **PDF** — human-readable, for direct submission to auditors (use `github.com/jung-kurt/gofpdf`)

Report types:
- **Access Report** — who accessed which model, when, from where
- **Data Flow Report** — request/response sizes, no content (for privacy)
- **Anomaly Report** — failed auth attempts, unusual access patterns
- **Retention Report** — confirms data retention policy is being followed

New CLI command:
```bash
sovstack report --type access --from 2026-01-01 --to 2026-03-31 --format pdf --out q1-audit.pdf
sovstack report --type access --format csv --out audit.csv
```

**Acceptance criteria:**
- [ ] `sovstack report` command generates a PDF report
- [ ] Report covers the specified date range
- [ ] CSV export is importable into Excel
- [ ] JSON export is valid and includes all audit fields
- [ ] Report is generated from the encrypted SQLite audit log

---

### TASK-015: Security Anomaly Detection

**Why:** Security teams need to know when something unusual is happening — an account making 10× its normal request volume, off-hours access, or PII appearing in prompts.

**Files to read first:**
- `core/audit/sqlite.go` — source of historical patterns
- `visibility/analytics/usage.go` — usage baselines

**What to build:**

New file: `visibility/security/anomaly.go`

Detection rules (start simple, no ML required):

```go
type AnomalyRule struct {
    Name        string
    Description string
    Check       func(recent, baseline UsageStats) bool
    Severity    string // "low", "medium", "high"
}

var DefaultRules = []AnomalyRule{
    {
        Name: "volume_spike",
        Description: "User made 5x their normal request volume",
        Check: func(r, b UsageStats) bool { return r.RequestsPerHour > b.RequestsPerHour*5 },
        Severity: "medium",
    },
    {
        Name: "off_hours_access",
        Description: "Access outside normal working hours (9am-6pm local)",
        Check: ...,
        Severity: "low",
    },
    {
        Name: "auth_failure_burst",
        Description: "More than 5 auth failures in 5 minutes from one IP",
        Check: ...,
        Severity: "high",
    },
    {
        Name: "new_user_high_volume",
        Description: "User first seen today making unusually high requests",
        Check: ...,
        Severity: "medium",
    },
}
```

Run anomaly checks every 5 minutes. Store triggered anomalies in SQLite.

**API endpoint:** `GET /api/visibility/security/anomalies?period=24h`

**Acceptance criteria:**
- [ ] Volume spike rule triggers correctly in tests
- [ ] Auth failure burst rule triggers on 5+ failures from same IP in 5 min
- [ ] Anomalies are stored and queryable
- [ ] `GET /api/visibility/security/anomalies` returns recent anomalies with severity

---

### TASK-016: Visibility REST API + Dashboard Backend

**Why:** All the visibility data needs a clean REST API that a dashboard frontend (or third-party tools like Grafana) can query.

**Files to read first:**
- `cmd/gateway.go` — pattern for HTTP server setup
- All `visibility/` packages — the data they produce

**What to build:**

New file: `visibility/api/server.go`

Single HTTP server with these endpoints:

```
GET  /api/health                           — liveness check
GET  /api/visibility/metrics?hours=24      — operational metrics time series
GET  /api/visibility/costs?period=30d      — financial comparison
GET  /api/visibility/usage?period=7d       — usage analytics
GET  /api/visibility/usage/users           — per-user breakdown
GET  /api/visibility/usage/teams           — per-team breakdown
GET  /api/visibility/usage/models          — per-model breakdown  
GET  /api/visibility/security/anomalies    — security events
GET  /api/visibility/nodes                 — all deployed nodes status
GET  /api/audit/logs?n=100&user=alice      — raw audit log query
GET  /api/audit/stats                      — aggregate audit statistics
POST /api/visibility/report                — trigger report generation
```

New CLI command:
```bash
sovstack visibility --port 9000
```

Starts the visibility API server. Designed to run alongside the gateway.

**Authentication:** Visibility API protected by the same API key system, but only admin-level keys can access it.

**Acceptance criteria:**
- [ ] All endpoints return valid JSON
- [ ] `GET /api/health` returns 200 with `{"status": "ok"}`
- [ ] Endpoints are protected by API key auth
- [ ] `sovstack visibility` command starts the server
- [ ] Server is documented with inline comments describing each endpoint

---

## Notes for Claude Code Sessions

**Starting a task:**
Tell Claude: *"Read TASKS.md and the files listed under 'Files to read first' in TASK-XXX. Then implement it."*

**Parallel tasks (no dependencies):**
These can run in separate Claude Code sessions simultaneously:
- TASK-003, TASK-004, TASK-007, TASK-008 (all Phase 1, no interdependencies)
- TASK-011, TASK-012, TASK-013, TASK-014, TASK-015 (all Phase 2, depend only on TASK-006)

**Critical path (must be sequential):**
TASK-001 → TASK-005 → TASK-002 → TASK-009 → TASK-010

**After each task:**
1. Run `go build -o sovstack .` to verify it compiles
2. Run the relevant `./sovstack` command to test manually
3. Mark the task `[x]` in this file
