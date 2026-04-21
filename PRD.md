# Product Requirements Document (PRD) for SovereignStack CLI (MVP)

## Project Overview

**Project Title:** SovereignStack

**Objective:**  
Create a unified, open-source CLI and orchestration engine that allows developers to deploy private, production-ready AI inference clusters. The tool must prioritize a "Local-First" approach but use an abstraction layer that makes it 100% compatible with major cloud providers (AWS, Azure, GCP) in future phases.

**Target User:**  
DevOps engineers, System Administrators, and AI practitioners who need to host models like Llama 3 or Mistral internally for cost, privacy, or regulatory compliance reasons.

## Architecture: Provider-Based Design

SovereignStack uses a **Provider-based architecture** where the core logic is decoupled from the underlying infrastructure. The "Brain" of the software doesn't know about hardware; it only knows how to talk to a "Provider" interface.

### Directory Structure

```
sovereignstack/
├── cli/                    # CLI entry point (Cobra-based)
│   ├── cmd/
│   │   ├── root.go
│   │   ├── init.go
│   │   ├── pull.go
│   │   ├── up.go
│   │   └── status.go
│   └── main.go
├── core/                   # Core business logic
│   ├── model/             # Model management
│   ├── gateway/           # HTTP reverse proxy and audit logging
│   ├── audit/             # Encrypted SQLite audit logs
│   └── config/            # Configuration management
├── internal/
│   ├── provider/          # Provider interface & implementations
│   │   ├── interface.go   # Standard Provider interface
│   │   ├── onprem/        # On-premises implementation (Docker/K3s)
│   │   ├── aws/           # (Phase 2)
│   │   ├── azure/         # (Phase 2)
│   │   └── gcp/           # (Phase 2)
│   ├── hardware/          # Hardware detection
│   └── docker/            # Docker utilities
├── pkg/                    # Public packages
├── main.go                 # Application entry point
├── go.mod
└── README.md
```

## Core Functional Requirements (Phase 1)

### A. The "Engine Room" (Inference)

- **vLLM as Default Backend:** Use vLLM for high throughput and OpenAI-compatible API.
- **Automatic Quantization:** Detect host VRAM and automatically suggest or apply the best quantization (AWQ, GPTQ) so models fit hardware.
- **Flexible Model Support:** Support multiple models with dynamic configuration based on available resources.

### B. The "Safety Net" (Local Gateway)

- **Lightweight Reverse Proxy:** Built in Go, sits in front of the AI inference engine.
- **Encrypted Audit Logging:** Every request/response logged to an encrypted SQLite database at rest (foundation for future "Medical" specialization).
- **Rate Limiting:** Token-based bucket limits prevent GPU hogging by single users.
- **Request/Response Validation:** Sanitize and validate all inputs before passing to inference engine.

### C. The "DevOps Shovel" (Orchestration)

- **`sovstack init`:** Detects NVIDIA drivers, installs Container Toolkit, sets up local Docker network.
- **`sovstack pull [model]`:** Downloads models from Hugging Face, saves to structured `/models` directory.
- **`sovstack up`:** Orchestrates containers, networking, and proxy in one command.
- **`sovstack status`:** Reports cluster health, resource usage, running models.

## Technical Stack (Phase 1)

- **Language:** Go 1.21+ (for a single binary executable)
- **Containerization:** Docker / Docker Compose
- **Inference Engine:** vLLM (OpenAI-compatible)
- **Gateway:** Custom Go reverse proxy
- **Audit Storage:** SQLite with encryption (optional: PostgreSQL for Phase 2)
- **Monitoring:** Prometheus/Grafana (optional for MVP)
- **Testing:** testcontainers-go for integration tests

## Provider Interface (Core Abstraction)

All providers must implement the following interface:

```go
type Provider interface {
    // Initialize sets up the infrastructure (networks, directories, etc.)
    Initialize(ctx context.Context, config Config) error
    
    // RunModel deploys a model to the inference engine
    RunModel(ctx context.Context, modelConfig ModelConfig) error
    
    // Status reports cluster health and resource usage
    Status(ctx context.Context) (ClusterStatus, error)
    
    // Stop halts all running services
    Stop(ctx context.Context) error
}
```

This design allows seamless switching between on-prem, AWS, Azure, and GCP implementations in future phases.

## CLI Commands

```bash
# Initialize the system (detect hardware, setup networks)
sovstack init [--provider onprem]

# Pull a model from Hugging Face
sovstack pull [model-name] [--cache-dir /path]

# Start the inference cluster
sovstack up [--model model-name] [--quantization awq]

# Check cluster status
sovstack status

# Stop all services
sovstack down
```

## Implementation Roadmap

### Phase 1 (MVP - Current Focus)
- [ ] Task 1: Define project structure and Provider interface
- [ ] Task 2: Implement OnPrem provider with Docker/K3s support
- [ ] Task 3: Build core/gateway reverse proxy with audit logging
- [ ] Task 4: Implement model management and quantization detection
- [ ] Task 5: Create CLI commands (init, pull, up, status)
- [ ] Task 6: Add hardware detection and CUDA verification

### Phase 2 (Cloud Providers)
- [ ] AWS provider implementation
- [ ] Azure provider implementation
- [ ] GCP provider implementation
- [ ] Multi-region support

### Phase 3 (Advanced Features)
- [ ] Medical specialization with HIPAA audit trails
- [ ] Multi-model orchestration
- [ ] Advanced rate limiting and quotas
- [ ] Federation between clusters

## Phase 1 Success Criteria

- Single binary executable that builds and runs on Ubuntu 20.04+
- `sovstack init` successfully detects NVIDIA hardware and CUDA
- `sovstack pull` downloads models from Hugging Face correctly
- `sovstack up` deploys vLLM with automatic quantization
- Gateway proxy logs all requests to encrypted SQLite database
- OpenAI-compatible API accessible at `http://localhost:8000`
- Rate limiting prevents GPU hogging
- Zero public ports exposed (localhost only)
- All code documented and tested

## Non-Functional Requirements

- **Performance:** Gateway adds <50ms latency per request
- **Security:** Audit logs encrypted at rest, TLS for future cloud providers
- **Maintainability:** Clean separation between core and providers
- **Extensibility:** Adding new providers requires only implementing the interface
- **Testing:** >80% code coverage, integration tests with testcontainers-go