# SovereignStack CLI Reference

Complete documentation for all SovereignStack commands.

## Configuration Commands

### `sovstack config list`

Display all configuration settings.

```bash
sovstack config list
```

**Output:**
```
SovereignStack Configuration:

  cache-dir:      /home/user/.sovereignstack/models
  log-dir:        /var/log/sovereignstack
  hf-token:       (set, encrypted)

  Config file:    /home/user/.sovereignstack/config.json
  Audit log:      /var/log/sovereignstack/audit.log
```

---

### `sovstack config get <key>`

Get a specific configuration value.

**Keys:**
- `cache-dir` - Model cache directory
- `log-dir` - Audit log directory
- `hf-token` - Hugging Face API token status

**Examples:**

```bash
# Get cache directory
sovstack config get cache-dir
# Output: /home/user/.sovereignstack/models

# Check if token is configured
sovstack config get hf-token
# Output: (set, encrypted)

# Get log directory
sovstack config get log-dir
# Output: /var/log/sovereignstack
```

---

### `sovstack config set <key> <value>`

Set a configuration value.

**Examples:**

```bash
# Set cache directory on large disk
sovstack config set cache-dir /mnt/nvme/models

# Set log directory
sovstack config set log-dir /var/log/sovereignstack

# Store Hugging Face token (encrypted)
sovstack config set hf-token hf_YOUR_TOKEN_HERE
```

**Notes:**
- Tokens are encrypted with AES-256-GCM before storage
- Encryption key is derived from user home directory (per-user isolation)
- Config file has restrictive permissions (0600, user-readable only)
- All config changes are logged to audit trail

---

## Model Management Commands

### `sovstack pull <model-name>`

Download a model from Hugging Face to the local cache.

**Syntax:**
```bash
sovstack pull <model-name>
```

**Examples:**

```bash
# Download GPT-2 (public model, ~1GB)
sovstack pull gpt2

# Download Llama 2 (gated, requires token)
sovstack pull meta-llama/Llama-2-7b-hf

# Download with custom token
HF_TOKEN=hf_xxx sovstack pull meta-llama/Llama-2-7b-hf

# Download to custom location
SOVEREIGNSTACK_CACHE_DIR=/tmp/models sovstack pull gpt2
```

**Features:**
- ✓ Automatic file type detection (safetensors, bin, pt, pth, gguf)
- ✓ Resume support (continues interrupted downloads)
- ✓ Metadata verification (size validation)
- ✓ Progress reporting (file counts and sizes)
- ✓ Audit logging (download success/failure)

**Output:**
```
📥 Pulling model: gpt2

📥 Downloading: gpt2
   Checking for model files in gpt2...
   1. model.safetensors
   ✓ Downloaded
   2. pytorch_model.bin
   ✓ Downloaded
   ✓ Download complete: 2 files
✓ Model cache entry created: gpt2
  Location: models/gpt2
  Size: 1045.44 MB
  Cached at: 2026-04-27 20:05:05

✅ Model pulled successfully!
```

---

### `sovstack status`

Show all cached models and their verification status.

**Syntax:**
```bash
sovstack status [flags]
```

**Flags:**
- `--detailed, -d` - Show detailed file listing for each model
- `--json, -j` - Output as JSON (future)

**Examples:**

```bash
# Show all models
sovstack status

# Show with file details
sovstack status --detailed

# Show with detailed output
sovstack status -d
```

**Output (basic):**
```
📊 SovereignStack Status

💾 Cached Models (2)
1. gpt2 [✓ Ready to deploy]
   Size: 1045.44 MB (2 files)
   Location: models/gpt2
   Cached: 2026-04-27 20:05:05

2. mistral [✗ Incomplete]
   Size: 512.00 MB (1 files)
   Location: models/mistral
   Cached: 2026-04-27 19:00:00
   ⚠ Warning: Size mismatch: expected 14GB, got 512MB

📈 Cache Statistics
Total Models: 2
Ready to Deploy: 1/2
Total Size: 1.54 GB
Location: /home/user/.sovereignstack/models

⚠️  Status: Models need attention
```

**Output (detailed):**
```
1. gpt2 [✓ Ready to deploy]
   Size: 1045.44 MB (2 files)
   Location: models/gpt2
   Cached: 2026-04-27 20:05:05
   Files:
     - model.safetensors (522.71 MB)
     - pytorch_model.bin (522.73 MB)
```

---

### `sovstack verify [model-name]`

Verify that a model is complete and ready to deploy.

**Syntax:**
```bash
sovstack verify [model-name]
```

**Examples:**

```bash
# Verify all models
sovstack verify

# Verify specific model
sovstack verify gpt2

# Verify before deploying
sovstack verify mistral && sovstack up mistral
```

**Output (all models):**
```
✓ gpt2: ready
✓ mistral: ready
⚠ incomplete-model: incomplete
  • No model files found in directory

✅ All models verified and ready!
```

**Output (specific model):**
```
📋 Verification Report: gpt2

Status: ready
Ready to Deploy: true
File Count: 2
Total Size: 1.02 GB (1045 MB)

📁 Model Files:
  ✓ model.safetensors (522.71 MB)
  ✓ pytorch_model.bin (522.73 MB)

✅ READY: Model is complete and ready to deploy

Next step: sovstack up gpt2
```

**Verification Checks:**
- ✓ Directory exists and is accessible
- ✓ Model files are present (safetensors, bin, pt, pth, gguf)
- ✓ File counts match metadata
- ✓ Total size matches cache entry
- ✓ No corruption or missing files

---

### `sovstack remove <model-name>`

Remove a model from the cache.

**Syntax:**
```bash
sovstack remove <model-name> [flags]
```

**Flags:**
- `--force, -f` - Skip confirmation prompt

**Examples:**

```bash
# Interactive removal
sovstack remove old-model

# Force removal (skip confirmation)
sovstack remove old-model --force

# Clean up to free space
sovstack remove gpt2 && sovstack status
```

**Output:**
```
🗑️  Remove Cached Model

Model: gpt2
Path: models/gpt2
Size: 1045.44 MB
Cached: 2026-04-27 20:05:05

🔄 Removing model...

✅ Model removed successfully!

📊 Cache Statistics
Cached Models: 1
Total Size: 0.00 GB

Remaining models:
  1. mistral (14.5 GB)
```

---

### `sovstack init`

Initialize the system and detect hardware.

**Syntax:**
```bash
sovstack init
```

**Features:**
- Detects GPUs and VRAM
- Checks CUDA/NVIDIA driver version
- Reports system CPU and RAM
- Shows compatible models for your hardware

**Output (GPU system):**
```
✓ Detected 1 GPU(s):
  GPU 1: NVIDIA A100 (40384 MB VRAM)
✓ CUDA installed: 12.0
✓ System: 64 CPU cores, 256.0 GB RAM

--- Available Models for Your Hardware ---
✓ 6 model(s) compatible with your hardware:
  • meta-llama/Llama-2-7b-hf (GPU)
  • meta-llama/Llama-2-13b-hf (GPU)
  • mistralai/Mistral-7B-v0.1 (GPU)
```

---

## Deployment Commands

### `sovstack deploy <model-name>`

Deploy a model and start the inference server.

Validates hardware compatibility, plans the deployment (quantization, VRAM), and starts the vLLM container.

**Syntax:**
```bash
sovstack deploy <model-name> [flags]
```

**Flags:**
- `--port, -p INT` - Port to expose inference server on (default: 8000)
- `--quantization, -q STRING` - Override quantization method (none/awq/gptq/int8/auto, default: auto)

**Examples:**

```bash
# Deploy with auto-selected quantization
sovstack deploy gpt2

# Deploy on custom port
sovstack deploy gpt2 --port 9000

# Deploy with specific quantization
sovstack deploy meta-llama/Llama-2-7b-hf --quantization awq

# Verify first, then deploy
sovstack verify gpt2 && sovstack deploy gpt2
```

**Output includes:**
- ✓ Hardware compatibility check
- 📋 Deployment plan showing model, quantization, VRAM requirements, context length
- 🚀 Deployment progress
- ✅ Success message with API endpoint

**Prerequisites:**
- Model must be cached (run `sovstack pull <model>` first)
- Model must be verified as ready (run `sovstack verify <model>`)
- Docker must be running
- Sufficient VRAM/RAM for the model

---

### `sovstack gateway`

Start the API gateway with authentication and rate limiting.

**Syntax:**
```bash
sovstack gateway [flags]
```

**Flags:**
- `--backend URL` - vLLM backend URL (default: http://localhost:8000)
- `--port PORT` - Gateway port (default: 8001)
- `--rate-limit N` - Tokens per minute per user
- `--api-key-header NAME` - Header name for API key
- `--audit-buffer N` - Audit log buffer size

**Example:**
```bash
sovstack gateway \
  --backend http://localhost:8000 \
  --port 8001 \
  --rate-limit 1000
```

---

## Workflow Examples

### Basic Workflow

```bash
# 1. Check hardware
sovstack init

# 2. Configure storage
sovstack config set cache-dir /mnt/large-disk/models

# 3. Download a model
sovstack pull gpt2

# 4. Verify it's ready
sovstack verify gpt2

# 5. Deploy
sovstack deploy gpt2

# 6. Use the API
curl http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt2", "messages":[{"role":"user","content":"Hi"}]}'
```

### Production Workflow with Gated Models

```bash
# 1. Store Hugging Face token securely
sovstack config set hf-token hf_YOUR_TOKEN

# 2. Configure logging
sovstack config set log-dir /var/log/sovereignstack

# 3. Download Llama 2
sovstack pull meta-llama/Llama-2-7b-hf

# 4. Verify completeness
sovstack verify meta-llama/Llama-2-7b-hf

# 5. Deploy with gateway
sovstack up meta-llama/Llama-2-7b-hf
sovstack gateway --port 8001 --rate-limit 5000

# 6. Monitor (in another terminal)
tail -f /var/log/sovereignstack/audit.log | jq .
```

### CI/CD Workflow

```bash
#!/bin/bash
set -e

# Use environment variables instead of config file
export SOVEREIGNSTACK_CACHE_DIR="/tmp/models"
export SOVEREIGNSTACK_LOG_DIR="/tmp/logs"
export HF_TOKEN="$HF_TOKEN"

# Download model
sovstack pull gpt2

# Verify it's ready
sovstack verify gpt2

# Run tests
./run-inference-tests.sh

# Clean up
sovstack remove gpt2
```

---

## Configuration File

Configuration is stored in `~/.sovereignstack/config.json`:

```json
{
  "cache_dir": "/mnt/models",
  "log_dir": "/var/log/sovereignstack",
  "hf_token": "...encrypted..."
}
```

**Permissions:** 0600 (user-readable only)

**Audit Log Location:** `{log_dir}/audit.log`

**Log Entry Example:**
```json
{
  "timestamp": "2026-04-27T20:05:05+10:00",
  "action": "model_download",
  "user": "gayangunapala",
  "details": "model=gpt2 2 files",
  "status": "success"
}
```

---

## Environment Variables

Override configuration with environment variables:

```bash
# Cache directory
SOVEREIGNSTACK_CACHE_DIR=/tmp/models

# Log directory
SOVEREIGNSTACK_LOG_DIR=/tmp/logs

# Hugging Face token
HF_TOKEN=hf_xxx

# Example usage
HF_TOKEN=hf_xxx SOVEREIGNSTACK_CACHE_DIR=/tmp sovstack pull meta-llama/Llama-2-7b-hf
```

---

## Help

Get help for any command:

```bash
sovstack --help
sovstack config --help
sovstack pull --help
sovstack status --help
sovstack verify --help
```
