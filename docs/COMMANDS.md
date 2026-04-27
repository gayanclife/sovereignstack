# Command Reference

Complete reference for all SovereignStack CLI commands.

## Global Options

All commands accept the following global options:

```bash
--help, -h      Show help for the command
--version, -v   Show SovereignStack version
```

---

## sovstack init

Provision a machine for SovereignStack deployment.

Detects hardware, checks prerequisites (CUDA, Docker, NVIDIA drivers), and optionally auto-installs missing components.

### Usage

```bash
sovstack init [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--check` | boolean | false | Run checks only, do not install prerequisites |
| `--help, -h` | - | - | Show help message |

### Examples

**Run full check and installation:**
```bash
./sovstack init
```

**Check-only mode (no installation prompts):**
```bash
./sovstack init --check
```

### Output

```
Running pre-flight checks...

System: ubuntu
Sudo access: yes

✓ Detected 1 GPU(s):
  GPU 1: NVIDIA RTX 4090 (24576 MB VRAM)
✓ NVIDIA Driver: 545.29.06
✓ CUDA: 12.1
✓ Docker: 24.0.6
✓ System: 16 CPU cores, 64.0 GB RAM

✓ All prerequisites installed successfully!
```

### What It Does

1. Detects the operating system
2. Checks if NVIDIA GPUs are present
3. Verifies NVIDIA drivers are installed
4. Checks if CUDA toolkit is installed
5. Verifies Docker is installed
6. Checks if NVIDIA Container Toolkit is installed
7. Detects available CPU cores and system RAM
8. (Optionally) Installs missing prerequisites
9. Lists compatible models for your hardware

### Supported OS

- Ubuntu 20.04+
- Debian 11+

Auto-installation only works on Ubuntu/Debian. Other systems will receive manual installation instructions.

---

## sovstack pull

Download a model from Hugging Face and cache it locally.

Automatically selects the best quantization based on available VRAM.

### Usage

```bash
sovstack pull <model-name> [flags]
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `<model-name>` | yes | Hugging Face model ID (e.g., `mistralai/Mistral-7B-Instruct-v0.3`) |

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--hf-token` | string | - | Hugging Face API token (or use `HF_TOKEN` env var) |
| `--help, -h` | - | - | Show help message |

### Examples

**Download a public model:**
```bash
./sovstack pull mistralai/Mistral-7B-Instruct-v0.3
```

**Download a gated model (requires Hugging Face token):**
```bash
export HF_TOKEN=hf_abc123xyz...
./sovstack pull meta-llama/Llama-3.1-70B-Instruct
```

Or inline:
```bash
./sovstack pull meta-llama/Llama-3.1-70B-Instruct --hf-token hf_abc123xyz...
```

### Output

```
Downloading mistralai/Mistral-7B-Instruct-v0.3...
config.json                [=====>        ] 45%  2.3 GB/5.1 GB  (2.1 MB/s)
model.safetensors          [========>     ] 65%  3.5 GB/5.3 GB  (1.8 MB/s)

✓ Download complete
  Model cached at: ~/.sovereignstack/models/mistralai/Mistral-7B-Instruct-v0.3
  Size: 5.1 GB
  Quantization: awq (auto-selected)
```

### What It Does

1. Fetches the model file list from Hugging Face API
2. Checks available VRAM on your GPU
3. Selects best quantization that fits: AWQ > FP16 > GPTQ > INT8
4. Downloads all model files to local cache
5. Shows progress for each file
6. Resumes interrupted downloads automatically
7. Verifies downloaded files

### Gated Models

Some models (like Meta Llama) require authentication. You'll need a Hugging Face account and an API token:

1. Visit [huggingface.co/settings/tokens](https://huggingface.co/settings/tokens)
2. Create a "User access token"
3. Accept the model's license on its Hugging Face page
4. Set the token: `export HF_TOKEN=hf_your_token...`

### Model Cache Location

Models are cached in:
- Linux/Mac: `~/.sovereignstack/models/`
- Environment variable: `SOVEREIGNSTACK_CACHE_DIR`

---

## sovstack deploy

Deploy a model to the vLLM inference server.

Validates hardware compatibility, creates a deployment plan, and starts a Docker container running the model with an OpenAI-compatible API.

### Usage

```bash
sovstack deploy <model-name> [flags]
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `<model-name>` | yes | Model name (must be downloaded with `pull` first) |

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--port, -p` | int | 8000 | Port to expose inference server on |
| `--quantization, -q` | string | auto | Override quantization (none/awq/gptq/int8/auto) |
| `--help, -h` | - | - | Show help message |

### Examples

**Deploy with auto-selected quantization:**
```bash
./sovstack deploy mistralai/Mistral-7B-Instruct-v0.3
```

**Deploy on specific port:**
```bash
./sovstack deploy mistralai/Mistral-7B-Instruct-v0.3 --port 9000
```

**Deploy without quantization (if enough VRAM):**
```bash
./sovstack deploy meta-llama/Llama-3.1-8B-Instruct --quantization none
```

**Deploy with AWQ quantization:**
```bash
./sovstack deploy mistralai/Mistral-7B-Instruct-v0.3 --quantization awq
```

### Output

```
Deploying model: mistralai/Mistral-7B-Instruct-v0.3
✓ Model mistralai/Mistral-7B-Instruct-v0.3 is compatible with your hardware

📋 Deployment Plan:
  Model:           mistralai/Mistral-7B-Instruct-v0.3
  Quantization:    awq
  Required VRAM:   5.0 GB
  Available VRAM:  23.5 GB
  Context Length:  32768 tokens
  Notes:           Excellent fit for GPU

🚀 Starting deployment...

✅ Model deployed successfully!
  API endpoint: http://localhost:8000/v1/chat/completions
  Run 'sovstack gateway' to start the secure proxy
```

### What It Does

1. Validates the model is cached locally
2. Checks hardware compatibility
3. Plans deployment (quantization, GPU placement)
4. Starts a Docker container with the model
5. Waits for the inference server to be ready
6. Tests the API endpoint
7. Prints the API endpoint URL

### API Endpoint

Once deployed, the model is available at:

```
http://localhost:{port}/v1/chat/completions
```

This is OpenAI-compatible, so you can use OpenAI client libraries:

```python
import openai

openai.api_base = "http://localhost:8000/v1"
openai.api_key = "not-needed"

completion = openai.ChatCompletion.create(
  model="mistralai/Mistral-7B-Instruct-v0.3",
  messages=[{"role": "user", "content": "Hello!"}]
)
```

Or use `curl`:

```bash
curl http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mistralai/Mistral-7B-Instruct-v0.3",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 100
  }'
```

---

## sovstack status

Show running models, GPU utilization, and health status.

### Usage

```bash
sovstack status
```

### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--help, -h` | - | Show help message |

### Output

```
🖥️  Hardware
  GPUs: 1x NVIDIA RTX 4090 (24 GB VRAM)
  CPU:  16 cores
  RAM:  64.0 GB
  CUDA: 12.1
  Docker: ✓ installed

🚀 Running Models
  vllm-mistralai/Mistral-7B-v0.3  →  http://localhost:8000  (up 2h)

📦 Cached Models
  mistralai/Mistral-7B-Instruct-v0.3 (5.0 GB)
  meta-llama/Llama-3.1-8B-Instruct (4.5 GB)
```

### What It Does

1. Detects GPU count and VRAM
2. Shows CPU cores and system RAM
3. Lists running Docker containers with vLLM models
4. Shows port and uptime for each running model
5. Lists cached models and their sizes

---

## sovstack gateway

Start the secure reverse proxy gateway.

Provides API key authentication, rate limiting, and audit logging.

### Usage

```bash
sovstack gateway [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--port` | int | 8001 | Port to listen on |
| `--vllm-url` | string | http://localhost:8000 | vLLM inference server URL |
| `--audit-db` | string | ./sovstack-audit.db | Path to SQLite audit log |
| `--audit-key` | string | (env: SOVSTACK_AUDIT_KEY) | Encryption key for audit logs |
| `--help, -h` | - | - | Show help message |

### Examples

**Start gateway with defaults:**
```bash
./sovstack gateway
```

**Start on custom port:**
```bash
./sovstack gateway --port 9001
```

**Start with audit logging:**
```bash
export SOVSTACK_AUDIT_KEY=my-secret-key-12345
./sovstack gateway --audit-db /var/log/sovstack-audit.db
```

### Output

```
Starting SovereignStack Gateway...
Loaded 3 API keys
Reverse proxy listening on: http://localhost:8001
vLLM backend: http://localhost:8000
Audit logging: enabled (./sovstack-audit.db)

Gateway ready. Requests require Authorization header:
  Authorization: Bearer sk_abc123xyz...
```

### What It Does

1. Loads API keys from `~/.sovereignstack/keys.json`
2. Starts HTTP server on specified port
3. Proxies requests to vLLM with authentication
4. Rate-limits requests per API key
5. Logs all requests and responses to encrypted SQLite database
6. Adds correlation IDs for tracing

### Making Requests

```bash
curl http://localhost:8001/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk_abc123xyz..." \
  -d '{
    "model": "mistralai/Mistral-7B-Instruct-v0.3",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

---

## sovstack keys

Manage API keys for the gateway.

### Usage

```bash
sovstack keys <subcommand> [args]
```

### Subcommands

#### keys add

Generate a new API key for a user.

```bash
./sovstack keys add <user-id>
```

Output:
```
✓ API key for 'alice': sk_abc123xyz...
  Store this in a safe place. You cannot retrieve it later.
  Add to request header: Authorization: Bearer sk_abc123xyz...
```

The key is stored in `~/.sovereignstack/keys.json`.

#### keys list

List all users with API keys (keys are not shown).

```bash
./sovstack keys list
```

Output:
```
API Keys:
  alice       (added 2026-04-20)
  bob         (added 2026-04-19)
  hr-team     (added 2026-04-18)
```

#### keys remove

Revoke a user's API key.

```bash
./sovstack keys remove <user-id>
```

Output:
```
✓ API key for 'alice' removed
```

### Examples

**Add key for a user:**
```bash
./sovstack keys add alice
```

**Add key for a team:**
```bash
./sovstack keys add hr-team
```

**List all users:**
```bash
./sovstack keys list
```

**Revoke a key:**
```bash
./sovstack keys remove alice
```

---

## sovstack remove

Stop and remove a deployed model.

Stops the Docker container and removes cached model files.

### Usage

```bash
sovstack remove <model-name> [flags]
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `<model-name>` | yes | Model name to remove |

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--keep-cache` | boolean | false | Stop the container but keep model files cached |
| `--help, -h` | - | - | Show help message |

### Examples

**Remove model completely:**
```bash
./sovstack remove mistralai/Mistral-7B-Instruct-v0.3
```

**Stop container but keep cached model:**
```bash
./sovstack remove mistralai/Mistral-7B-Instruct-v0.3 --keep-cache
```

### Output

```
✓ Model stopped: vllm-mistralai/Mistral-7B-v0.3
✓ Cache cleared (5.1 GB freed)
```

Or with `--keep-cache`:
```
✓ Model stopped: vllm-mistralai/Mistral-7B-v0.3
✓ Cache preserved: ~/.sovereignstack/models/mistralai/Mistral-7B-Instruct-v0.3 (5.1 GB)
```

### What It Does

1. Stops the Docker container running the model
2. Removes the container from Docker
3. (Unless `--keep-cache`) Deletes the cached model files
4. Reports freed disk space

---

---

## sovstack models

Discover and manage available models compatible with your hardware.

Uses a hybrid registry system:
1. **Local:** Bundled models (always available, offline)
2. **Remote:** Cached from registry API (optional, fetched automatically)
3. **User:** Custom models from `~/.sovereignstack/models.yaml`

### Usage

```bash
sovstack models <subcommand> [flags]
```

### Subcommands

#### models list

Show all models compatible with your hardware.

```bash
./sovstack models list
```

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | boolean | false | Show all models including incompatible ones |
| `--gpu` | boolean | false | Filter for GPU-only models |
| `--cpu` | boolean | false | Filter for CPU-optimized models |
| `--min-vram` | int | 0 | Minimum VRAM in GB (GPU only) |
| `--registry` | string | https://models.sovereignstack.io/registry.yaml | Remote registry URL |

**Examples:**

```bash
# Show compatible models for your hardware
./sovstack models list

# Show all models (including incompatible)
./sovstack models list --all

# Show only GPU models
./sovstack models list --gpu

# Show only CPU models
./sovstack models list --cpu

# Use custom registry
./sovstack models list --registry https://my-registry.io/models.yaml
```

**Output:**

```
✓ Loaded models from local registry + remote cache

📚 Available Models (6 found):

GPU: 1x NVIDIA RTX 4090 (24 GB VRAM)
RAM: 64.0 GB

📌 mistralai/Mistral-7B-Instruct-v0.3 (GPU)
   Mistral 7B v0.3 Instruct — fast, high quality, 32k context
   Parameters: 7.0B | Context: 32k
   Min VRAM: 4.0 GB
   Download: sovstack pull mistralai/Mistral-7B-Instruct-v0.3

📌 TinyLlama/TinyLlama-1.1B-Chat-v1.0 (CPU)
   TinyLlama 1.1B Chat — minimal RAM, runs on anything
   Parameters: 1.1B | Context: 2k
   Min RAM: 3.0 GB
   Download: sovstack pull TinyLlama/TinyLlama-1.1B-Chat-v1.0
```

#### models refresh

Fetch latest models from remote registry and update cache.

```bash
./sovstack models refresh
```

**Flags:**

| Flag | Type | Description |
|------|------|-------------|
| `--registry` | string | Remote registry URL |

**Examples:**

```bash
# Refresh models from default registry
./sovstack models refresh

# Refresh from custom registry
./sovstack models refresh --registry https://my-registry.io/models.yaml
```

**Output:**

```
Fetching models from remote registry...
✅ Successfully fetched 42 models from remote registry
   Cached at: ~/.sovereignstack/models-remote.json
   Cache expires in 24 hours

Run 'sovstack models list' to see all available models
```

**Note:** Cache is valid for 24 hours. Refresh updates the cache.

#### models clear-cache

Remove cached remote models (will re-fetch on next `refresh` or `list`).

```bash
./sovstack models clear-cache
```

**Examples:**

```bash
# Clear cache to force fresh fetch
./sovstack models clear-cache
```

**Output:**

```
✓ Remote models cache cleared
```

---

### How Model Discovery Works

1. **Detect Hardware:** SovereignStack detects your GPUs, CPU cores, and RAM
2. **Load Local Models:** Reads bundled `models.yaml` (always available)
3. **Try Remote:** If internet available, fetches latest models and caches for 24h
4. **Merge:** Combines local + remote (remote takes precedence)
5. **Filter:** Shows only models compatible with your hardware
6. **Sort:** Orders by parameter count
7. **Display:** Shows model details with download commands

### Filtering Logic

**GPU Systems:**
- Shows GPU-only models + GPU/CPU hybrid models
- Hides CPU-optimized models
- Filters by VRAM requirements

**CPU-Only Systems:**
- Shows CPU-optimized models + GPU/CPU hybrid models
- Hides GPU-only models
- Filters by system RAM requirements

**Override:** Use `--all` to see everything

### Model Sources (Priority Order)

1. **Remote Cache** — `~/.sovereignstack/models-remote.json` (highest, overrides all)
2. **Bundled Local** — `models.yaml` in binary directory
3. **System Config** — `/etc/sovereignstack/models.yaml`
4. **User Config** — `~/.sovereignstack/models.yaml`
5. **Project Local** — `./models.local.yaml` (lowest priority, git-ignored)

---

## Environment Variables

SovereignStack respects the following environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `SOVEREIGNSTACK_CACHE_DIR` | `~/.sovereignstack/models/` | Model cache location |
| `HF_TOKEN` | - | Hugging Face API token for gated models |
| `SOVSTACK_AUDIT_KEY` | - | Encryption key for gateway audit logs |

Example:
```bash
export SOVEREIGNSTACK_CACHE_DIR=/mnt/large-ssd/models
export HF_TOKEN=hf_abc123xyz...
./sovstack pull meta-llama/Llama-3.1-70B-Instruct
```
