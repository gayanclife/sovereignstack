# Quick Start Guide

Get SovereignStack running in 5 minutes on a fresh Ubuntu machine.

## Prerequisites

- Ubuntu 20.04 or later
- **GPU:** NVIDIA GPU + CUDA (optional — GPU accelerates inference ~10-50x)
- **CPU-only:** Supported! Ideal for testing and small models (TinyLlama 1.1B, Phi-3 Mini 3.8B)
- 8 GB RAM minimum (16 GB+ recommended)
- 50 GB free disk space (for model cache)
- Internet connection (for initial setup and model downloads)

## Step 1: Download SovereignStack

Download the latest binary from the releases page or build from source:

```bash
# Build from source (requires Go 1.21+)
git clone https://github.com/gayanclife/sovereignstack.git
cd sovereignstack
go build -o sovstack .
```

Or use the prebuilt binary:
```bash
wget https://github.com/gayanclife/sovereignstack/releases/latest/sovereignstack-linux-amd64
chmod +x sovereignstack-linux-amd64
sudo mv sovereignstack-linux-amd64 /usr/local/bin/sovstack
```

## Step 2: Run the Provisioner

The `init` command detects your hardware and optionally installs prerequisites.

**Note:** If you don't have a GPU, SovereignStack will recognize you're on CPU-only and skip unnecessary GPU-related prerequisites. You only need Docker!

### For GPU Deployments

```bash
./sovstack init
```

Output:
```
Running pre-flight checks...

System: ubuntu
Sudo access: yes

✓ Detected 1 GPU(s):
  GPU 1: NVIDIA RTX 4090 (24576 MB VRAM)
✓ NVIDIA Driver: 545.29.06
✓ CUDA: 12.1
✓ Docker: 24.0.6
✓ NVIDIA Container Toolkit installed
✓ System: 16 CPU cores, 64.0 GB RAM

✓ All prerequisites installed successfully!
```

**If any prerequisites are missing**, the tool will offer to install them:

```bash
./sovstack init
# If issues found:
# Fix automatically? [y/N]: y
```

For check-only mode without prompting:
```bash
./sovstack init --check
```

---

## CPU-Only Quickstart

If you don't have a GPU, follow this simplified path to get started quickly:

### Step 2a: Just Install Docker

```bash
./sovstack init
# Or just check: ./sovstack init --check
```

You only need Docker. CUDA and GPU prerequisites are automatically skipped.

### Step 3a: Download a Small Model

For CPU, use tiny models that run without GPU:

```bash
# TinyLlama 1.1B (smallest, ~600MB)
./sovstack pull TinyLlama/TinyLlama-1.1B-Chat-v1.0

# Or Phi-3 Mini 3.8B (better quality, ~2GB)
./sovstack pull microsoft/Phi-3-mini-4k-instruct
```

### Step 4a: Deploy and Test

```bash
./sovstack up TinyLlama/TinyLlama-1.1B-Chat-v1.0
```

Test it:
```bash
curl http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "TinyLlama/TinyLlama-1.1B-Chat-v1.0",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 50
  }'
```

That's it! You now have a running LLM inference server on CPU. ✓

---

## GPU Quickstart

### Step 3: Download a Model

Choose a model suitable for your hardware and download it from Hugging Face:

```bash
# Download Mistral 7B (12 GB model, ~7 GB after quantization)
./sovstack pull mistralai/Mistral-7B-Instruct-v0.3
```

Progress output:
```
Downloading mistralai/Mistral-7B-Instruct-v0.3...
  [=====>        ] 45% 2.3 GB/5.1 GB  (2.1 MB/s)
```

The tool automatically:
- Detects your available VRAM
- Selects the best quantization (AWQ > FP16 > GPTQ > INT8)
- Caches the model locally
- Resumes interrupted downloads

## Step 4: Deploy the Model

Start the inference server with a single command:

```bash
./sovstack up mistralai/Mistral-7B-Instruct-v0.3
```

Output:
```
Deployment Plan:
  Model:         mistralai/Mistral-7B-Instruct-v0.3
  Quantization:  awq
  Required VRAM: 5.0 GB
  Available VRAM: 23.5 GB
  Notes: Excellent fit for GPU

Starting deployment...
✓ Model deployed successfully
  API endpoint: http://localhost:8000/v1/chat/completions
  Run 'sovstack gateway' to start the secure proxy
```

The inference server is now running in a Docker container and ready to accept requests.

## Step 5: Test the API

Query the inference server directly (or through the gateway):

```bash
curl http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mistralai/Mistral-7B-Instruct-v0.3",
    "messages": [
      {"role": "user", "content": "What is the capital of France?"}
    ],
    "max_tokens": 100
  }'
```

Response:
```json
{
  "id": "chatcmpl-123",
  "object": "text_completion",
  "created": 1234567890,
  "model": "mistralai/Mistral-7B-Instruct-v0.3",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "The capital of France is Paris."
      }
    }
  ]
}
```

## Step 6: (Optional) Add Security with Gateway

Start the gateway proxy for API key authentication and rate limiting:

```bash
# Generate an API key
./sovstack keys add my-app

# Output:
# API key for 'my-app': sk_abc123xyz...

# Start the gateway
./sovstack gateway
```

The gateway now proxies requests on `http://localhost:8001` with authentication:

```bash
curl http://localhost:8001/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk_abc123xyz..." \
  -d '{
    "model": "mistralai/Mistral-7B-Instruct-v0.3",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

## Step 7: Monitor Status

Check the health and resource usage of your deployment:

```bash
./sovstack status
```

Output:
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

## Next Steps

- **Deploy another model:** `./sovstack pull <model>` and `./sovstack up <model>`
- **Manage API keys:** See [Command Reference](./COMMANDS.md#keys)
- **Configure gateway:** See [Gateway & Security Setup](./GATEWAY_SECURITY.md) *(Coming Soon)*
- **Troubleshoot:** Check logs with `docker logs vllm-<model-name>`

## Common Issues

### "No NVIDIA GPUs detected"
Make sure your GPU is supported and NVIDIA drivers are installed:
```bash
nvidia-smi
```

### "CUDA not installed"
Run `./sovstack init` to auto-install CUDA, or manually install from:
https://docs.nvidia.com/cuda/cuda-installation-guide-linux/

### "Docker not installed"
Run `./sovstack init` to auto-install Docker, or manually install from:
https://docs.docker.com/engine/install/ubuntu/

### "Model download hangs"
The download is resumable. If interrupted, running `pull` again will continue from where it left off. Check your internet connection if it seems stuck.

### "Inference is slow"
Check GPU utilization:
```bash
watch -n 1 nvidia-smi
```

If GPU usage is low, the model quantization might be too aggressive. Try deploying without quantization (though it may require more VRAM):
```bash
./sovstack up mistralai/Mistral-7B-Instruct-v0.3 --quantization none
```

## What's Next?

- Explore other models from [models.yaml](../models.yaml)
- Read the full [Command Reference](./COMMANDS.md)
- Set up the [Gateway & Security](./GATEWAY_SECURITY.md) for production use

Enjoy your private AI inference! 🚀
