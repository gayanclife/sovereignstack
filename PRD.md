# Product Requirements Document (PRD) for SovereignStack CLI (MVP)

## Project Overview

**Project Title:** SovereignStack CLI (MVP)

**Objective:**  
Build a Go-based CLI tool that automates the deployment of private, production-grade LLM inference servers on Bare Metal or Virtual Private Servers (VPS). The goal is to provide a "one-command" experience that rivals AWS Bedrock but runs on local hardware.

**Target User:**  
DevOps engineers and System Administrators who need to host models like Llama 3 or Mistral internally for cost or privacy reasons.

## Core Functional Requirements

### 1. Hardware Pre-flight Check
- **Detect NVIDIA GPUs and available VRAM:** Automatically scan the system for NVIDIA GPUs and report available VRAM.
- **Verify CUDA driver installation:** Check if CUDA drivers are installed; provide instructions or automated installation for missing dependencies.

### 2. Automated Inference Engine Setup
- **Deploy vLLM or Ollama via Docker containers:** Automate the deployment of vLLM (preferred for production-grade throughput) or Ollama (for easy local setup).
- **Configure OpenAI-compatible API endpoint:** Set up the API endpoint to be compatible with OpenAI's API for seamless integration.

### 3. Model Management
- **Command: `sovstack pull [model-name]`** – Fetches model weights from Hugging Face or a private registry.
- **Command: `sovstack deploy [model-name]`** – Starts the container with optimized GPU parameters (e.g., setting `--gpu-memory-utilization` based on detected VRAM).

### 4. Local Observability
- **Integration with Prometheus/Grafana:** Provide lightweight monitoring to track token-per-second (TPS) and GPU temperature.

### 5. Secure Networking (Zero-Trust)
- **Automatic setup of Tailscale or Wireguard tunnel:** Ensure the API is accessible privately without opening public ports.

## Technical Stack

- **Language:** Go (for a single binary executable)
- **Containerization:** Docker / Docker Compose
- **Orchestration:** K3s (optional for MVP, start with Docker Compose)
- **Inference Engine:** vLLM

## Project Directory Structure

```
sovereignstack/
├── cmd/                # CLI Command logic (Cobra)
│   ├── root.go         # The main 'sovstack' command
│   ├── init.go         # 'sovstack init' - Hardware check
│   └── deploy.go       # 'sovstack deploy' - Container logic
├── internal/           # Private library code (The "Engine")
│   ├── hardware/       # CPU, RAM, GPU detection logic
│   ├── engine/         # Docker/vLLM orchestration
│   └── tunnel/         # Tailscale/Networking logic
├── pkg/                # Publicly sharable logic (Optional)
├── main.go             # Entry point
├── go.mod              # Go dependencies
├── README.md           # Documentation
└── docker-compose.yml  # Docker Compose configuration
```

## Core CLI Logic

Implement the command parser using Cobra for Go:
- Root command: `sovstack`
- Subcommands: `init`, `pull`, `deploy`
- Handle command-line arguments and flags appropriately.

## Automation Scripts

Write shell scripts or integrate into Go code for:
- Hardware detection and CUDA verification
- Docker container deployment and configuration
- Model pulling and deployment with GPU optimization

## README Documentation

Provide clear instructions on:
- How to build and install the CLI
- Running `sovstack init` on a fresh Ubuntu server
- Usage examples for `pull` and `deploy` commands
- Prerequisites and troubleshooting

## Implementation Plan

1. Set up Go module and basic project structure
2. Implement CLI framework with Cobra
3. Develop hardware detection logic
4. Create Docker orchestration for vLLM
5. Implement model management commands
6. Add observability integration
7. Set up secure networking
8. Write comprehensive README and documentation
9. Testing and validation

## Success Criteria

- Single binary executable that can be run on Ubuntu servers
- Successful deployment of LLM models with one command
- OpenAI-compatible API endpoint accessible securely
- Monitoring dashboard for performance metrics
- Zero public ports exposed for security