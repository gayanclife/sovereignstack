# SovereignStack Documentation

Welcome to the SovereignStack documentation. This guide covers everything you need to know to deploy and manage private LLM inference on your own infrastructure.

## Getting Started

- **[Quick Start Guide](./QUICKSTART.md)** — Deploy your first model in 5 minutes on a fresh Ubuntu machine
- **[CLI Reference](./CLI_REFERENCE.md)** — Complete command documentation with examples
- **[Configuration Guide](./CONFIGURATION.md)** — Configure cache location, logging, and Hugging Face tokens

## Setup Guides

- **[Model Management Guide](./MODEL_MANAGEMENT.md)** — Download, cache, and deploy models from Hugging Face
- **[Gateway & Security Setup](./GATEWAY_SECURITY.md)** — API keys, authentication, rate limiting, and audit logging
- **[Dynamic Model Discovery](./MODEL_DISCOVERY.md)** — How to find models compatible with your hardware (local + remote)

## User Guides

- **[Contributing Guide](./CONTRIBUTING.md)** — How to add new models to the registry

## Architecture & Development

- **[Product Requirements](./PRD.md)** — Detailed feature specifications and design
- **[System Structure](./STRUCTURE.md)** — Codebase organization and architecture
- **[Hybrid Registry Implementation](./HYBRID_REGISTRY_IMPLEMENTATION.md)** — Technical details on model registry system

## Key Concepts

### What is SovereignStack?

SovereignStack is an open-source CLI tool that makes it simple to deploy private LLM inference servers on any hardware — local PC, remote server, or cloud VM — fully air-gapped.

### Core Features

- **Zero-Touch Installer** — `sovstack init` detects your hardware and installs NVIDIA drivers, CUDA, and Docker automatically
- **Model Downloads** — `sovstack pull` downloads open-weight models from Hugging Face with automatic quantization
- **One-Command Deployment** — `sovstack up` launches a production-grade vLLM inference server with a single command
- **Secure Gateway** — Optional reverse proxy with API key authentication, rate limiting, and audit logging
- **Hardware-Aware** — Automatically selects the best quantization to fit your available VRAM

### Supported Models

SovereignStack works with any open-weight model on Hugging Face, including:
- **Meta Llama 3.1** (8B, 70B)
- **Mistral 7B v0.3** (32K context)
- **Mixtral 8x7B** (MoE)
- **Qwen 2.5** (7B, 14B, 72B)
- **And many more...**

## Command Overview

```bash
sovstack init           # Detect hardware, install prerequisites
sovstack pull <model>   # Download a model from Hugging Face
sovstack up <model>     # Deploy a model to vLLM inference server
sovstack status         # Show running models and GPU utilization
sovstack gateway        # Start the secure reverse proxy
sovstack keys add <id>  # Generate an API key for a user
sovstack remove <model> # Stop and remove a deployed model
```

## Requirements

- **OS:** Ubuntu 20.04+ or Debian 11+
- **GPU:** NVIDIA GPU with CUDA 12.1+ (for GPU deployment)
- **RAM:** 8 GB minimum (more for larger models)
- **Disk:** 100 GB recommended (for model cache)

CPU-only deployment is supported for smaller models.

## System Architecture

```
┌─────────────────────────────────────┐
│  SovereignStack CLI (sovstack)      │
│  ├─ init    (provision machine)     │
│  ├─ pull    (download models)       │
│  ├─ up      (deploy inference)      │
│  ├─ status  (monitor health)        │
│  ├─ gateway (secure proxy)          │
│  └─ keys    (manage API keys)       │
└────────────────┬────────────────────┘
                 │
    ┌────────────┴────────────┐
    │                         │
┌───▼──────────────────┐  ┌──▼─────────────────┐
│  vLLM Container      │  │  Gateway Proxy     │
│  (inference engine)  │  │  (auth + rate      │
│                      │  │   limiting)        │
└──────────────────────┘  └────────────────────┘
```

## Getting Help

- Check the [Quick Start Guide](./QUICKSTART.md) for common setup issues
- See [Command Reference](./COMMANDS.md) for detailed command syntax
- Review logs: `docker logs vllm-<model-name>`

## License

SovereignStack is licensed under the Apache License 2.0. See the LICENSE file in the root directory for details.
