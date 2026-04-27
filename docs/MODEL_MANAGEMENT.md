# Model Management Guide

*Documentation for downloading, caching, and managing models in SovereignStack.*

## Table of Contents

- Model Discovery
- Downloading Models
- Caching Strategy
- Quantization
- Model Registry
- Troubleshooting

---

## Model Discovery

### View Available Models

See all models compatible with your hardware:

```bash
./sovstack init
```

The output shows models your hardware can run with their memory requirements.

### Browse Hugging Face

Browse all available models at [huggingface.co/models](https://huggingface.co/models).

Filter by:
- **Task:** Text Generation, Chat, Instruct
- **Size:** 3B, 7B, 13B, 70B, etc.
- **License:** Open-weight models

### Model Selection Guide

**For GPU (RTX 4090, A100, H100):**
- 70B models: Llama 3.1 70B, Mixtral 8x22B
- 34B models: Code Llama 34B, Qwen 32B
- 13B models: Mistral, Llama 2 13B
- 7B models: Mistral 7B, Phi-3, TinyLlama

**For Limited VRAM (8-16 GB):**
- Quantized 7B models (AWQ, GPTQ)
- Phi-3 (3.8B)
- TinyLlama (1.1B)

**For CPU-Only:**
- Phi-3 Mini (3.8B)
- TinyLlama (1.1B)
- Gemma 2B

---

## Downloading Models

### Basic Download

```bash
./sovstack pull mistralai/Mistral-7B-Instruct-v0.3
```

### With Hugging Face Token

For gated models (require license acceptance):

```bash
# Set token
export HF_TOKEN=hf_abc123xyz...

# Download gated model
./sovstack pull meta-llama/Llama-3.1-70B-Instruct
```

### Resume Interrupted Downloads

Downloads are resumable. If interrupted:

```bash
# Run the same command again
./sovstack pull mistralai/Mistral-7B-Instruct-v0.3
# Downloads resume from last checkpoint
```

---

## Caching Strategy

### Default Cache Location

Models are cached in:
- **Linux/Mac:** `~/.sovereignstack/models/`
- **Custom:** Set `SOVEREIGNSTACK_CACHE_DIR`

### Custom Cache Directory

Use a faster disk (SSD) for better performance:

```bash
export SOVEREIGNSTACK_CACHE_DIR=/mnt/nvme-ssd/models
./sovstack pull mistralai/Mistral-7B-Instruct-v0.3
```

### View Cached Models

```bash
./sovstack status
```

Shows:
```
📦 Cached Models
  mistralai/Mistral-7B-Instruct-v0.3 (5.0 GB)
  meta-llama/Llama-3.1-8B-Instruct (4.5 GB)
```

### Clear Cache

Remove a model:

```bash
./sovstack remove mistralai/Mistral-7B-Instruct-v0.3
```

---

## Quantization

*(Coming Soon)*

### Quantization Basics

### Auto-Selection

### Manual Override

---

## Model Registry

*(Coming Soon)*

### Built-in Models

### Custom Registry

### Community Contributions

---

## Troubleshooting

### Download Hangs

**Problem:** Download seems stuck.

**Solution:** Check network connection and try resuming:

```bash
# Ctrl+C to stop
# Run again to resume
./sovstack pull mistralai/Mistral-7B-Instruct-v0.3
```

### Insufficient VRAM

**Problem:** Model won't deploy due to VRAM.

**Solution:** Try a smaller model or quantized variant:

```bash
# Check available VRAM
./sovstack status

# Download quantized version (smaller)
./sovstack pull mistralai/Mistral-7B-Instruct-v0.3

# Or deploy with aggressive quantization
./sovstack up mistralai/Mistral-7B-Instruct-v0.3 --quantization int8
```

### Gated Model Access

**Problem:** 401 Unauthorized downloading a model.

**Solution:**

1. Accept license on Hugging Face: [meta-llama/Llama-3.1-8B-Instruct](https://huggingface.co/meta-llama/Llama-3.1-8B-Instruct)
2. Create API token: [huggingface.co/settings/tokens](https://huggingface.co/settings/tokens)
3. Set token and retry:

```bash
export HF_TOKEN=hf_abc123xyz...
./sovstack pull meta-llama/Llama-3.1-8B-Instruct
```

### Disk Space Issues

**Problem:** Not enough disk space for model.

**Solution:**

```bash
# Check disk usage
df -h

# Use larger disk for cache
export SOVEREIGNSTACK_CACHE_DIR=/mnt/large-storage/models
./sovstack pull mistralai/Mistral-7B-Instruct-v0.3
```

---

**See Also:**
- [Quick Start Guide](./QUICKSTART.md)
- [Command Reference](./COMMANDS.md) — `pull`, `up`, `status`, `remove`
- [Gateway & Security Setup](./GATEWAY_SECURITY.md) *(Coming Soon)*
