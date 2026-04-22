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

## Hardware-Aware Model Selection (MVP Feature)

SovereignStack now supports both **GPU and CPU deployments** with intelligent model filtering:

- **GPU Systems:** Recommends full-size models (Llama 2 7B/13B, Mistral 7B) with quantization options
- **CPU-Only Systems:** Recommends lightweight CPU-optimized models that fit available RAM
- **Automatic Validation:** Prevents deploying incompatible models and suggests alternatives
- **Configurable Model Registry:** Add new models via `models.yaml` without code changes

### Supported Models

**GPU-Optimized (Default):**
- Meta Llama 2 7B (13GB FP16, 3GB AWQ)
- Meta Llama 2 13B (26GB FP16, 6GB AWQ)
- Mistral 7B (13GB FP16, 3GB AWQ, 32k context)

**CPU-Optimized:**
- DistilBERT (250MB, requires 512MB RAM)
- TinyLlama 1.1B (2GB, requires 3GB RAM)
- Microsoft Phi-2 (5GB, requires 6GB RAM)

### Dynamic Model Registry

Models are loaded from **`models.yaml`** (configuration-driven, not hardcoded):

```yaml
gpu_models:
  - name: meta-llama/Llama-2-7b-hf
    repo: meta-llama/Llama-2-7b-hf
    parameters: "7B"
    hardware_target: gpu
    # ... specification continues
```

**Model Loading Precedence:**
1. Bundled (`models.yaml`) - Default models in SovereignStack
2. System-wide (`/etc/sovereignstack/models.yaml`) - System administrator customization
3. User-specific (`~/.sovereignstack/models.yaml`) - Personal model registry
4. Project-local (`./models.local.yaml`) - Per-project override

This design allows:
- **Community Contributions:** Users add models by editing YAML (no code changes needed)
- **Flexibility:** Different models per system/user/project
- **Fallback Safety:** Hardcoded defaults used if YAML files unavailable
- **Open-Source Ready:** Easy for the community to extend with custom models

## Implementation Roadmap

### Phase 1 (MVP - In Progress)
- [x] Task 1: Define project structure and Provider interface
- [x] Task 2: Implement hardware detection (GPU/CPU/CUDA/RAM)
- [x] Task 3: Add CPU-support with hardware-aware model filtering
- [x] Task 4: Implement model management and quantization detection
- [x] Task 5: Create CLI commands (init, pull, deploy) with hardware validation
- [x] Task 6: System RAM detection and model compatibility checking
- [x] Task 7: Dynamic model loading with YAML configuration
  - YAML-based model registry (`models.yaml`)
  - Multiple config source support (project/system/user/bundled)
  - Fallback to hardcoded defaults when YAML unavailable
  - Community-friendly model contribution system
  - Per-project customization (`models.local.yaml`)
- [ ] Task 8: Build core/gateway reverse proxy with audit logging
- [ ] Task 9: Implement OnPrem provider with Docker/K3s support

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

## Detailed Phase 1 Tasks

### Task 1: Define Project Structure and Provider Interface

**Objective:**  
Establish the foundational architecture and define the Provider interface that all implementations must follow.

**Deliverables:**

1. **Directory Structure Setup**
   - Create all necessary directories as specified in the architecture section
   - Add `.gitkeep` files to empty directories
   - Create a `STRUCTURE.md` documenting the layout

2. **Provider Interface (`internal/provider/interface.go`)**
   ```go
   type Config struct {
       Provider string                 // "onprem", "aws", "azure", "gcp"
       ModelDir string                 // Directory for cached models
       VRAMLimit uint64                // GPU VRAM limit in bytes
       GPUCount int                    // Number of GPUs to use
       EnableAuditLogging bool         // Enable audit log
       AuditDBPath string              // Path to SQLite audit DB
   }
   
   type ModelConfig struct {
       Name string                     // Model name (e.g., "meta-llama/Llama-2-7b")
       Quantization string             // Quantization type: "none", "awq", "gptq", "int8"
       MaxTokens int                   // Maximum tokens for inference
       Temperature float32             // Generation temperature
   }
   
   type ClusterStatus struct {
       Status string                   // "running", "stopped", "error"
       GPUUtilization []float32        // Utilization per GPU
       MemoryUsage []uint64            // Memory usage per GPU
       ActiveModels []string           // Currently running models
       RequestsPerSecond float32        // Current RPS
   }
   
   type Provider interface {
       // Initialize sets up the infrastructure
       Initialize(ctx context.Context, config Config) error
       
       // RunModel deploys a model to the inference engine
       RunModel(ctx context.Context, modelConfig ModelConfig) error
       
       // Status reports cluster health
       Status(ctx context.Context) (ClusterStatus, error)
       
       // Stop halts all services
       Stop(ctx context.Context) error
       
       // GetLogs retrieves service logs
       GetLogs(ctx context.Context, service string, lines int) ([]string, error)
   }
   ```

3. **OnPrem Provider Skeleton** (`internal/provider/onprem/provider.go`)
   - Create type `OnPremProvider struct`
   - Implement interface methods (stubs for now)
   - Add Docker client initialization

4. **Type Definitions** (`core/types.go`)
   - Define request/response types
   - Create audit log entry structures
   - Define configuration structures

**Acceptance Criteria:**

- [ ] All directories created and documented
- [ ] Provider interface compiles without errors
- [ ] OnPrem provider implements all interface methods (can be stubs)
- [ ] No external API calls made in stubs
- [ ] Code follows Go conventions (naming, formatting)
- [ ] All types have documentation comments

---

### Task 2: Implement OnPrem Provider with Docker/K3s Support

**Objective:**  
Build the on-premises provider that handles Docker orchestration, hardware detection, and local model deployment.

**Deliverables:**

1. **Docker Integration** (`internal/docker/docker.go`)
   ```go
   type DockerClient struct {
       client *client.Client
   }
   
   func (dc *DockerClient) CreateNetwork(ctx context.Context, name string) error
   func (dc *DockerClient) CreateVolume(ctx context.Context, name string) error
   func (dc *DockerClient) RunContainer(ctx context.Context, config ContainerConfig) (string, error)
   func (dc *DockerClient) GetContainerStatus(ctx context.Context, containerID string) (Status, error)
   func (dc *DockerClient) GenerateDockerCompose(config ComposeConfig) ([]byte, error)
   ```

2. **Hardware Detection** (`internal/hardware/detection.go`)
   - Implement GPU detection using `nvidia-smi`
   - Query GPU VRAM, driver version, CUDA compatibility
   - Detect CPU cores, total RAM, available disk space
   - Return hardware profile as `HardwareInfo` struct

3. **OnPrem Provider Implementation** (`internal/provider/onprem/provider.go`)
   ```go
   func (op *OnPremProvider) Initialize(ctx context.Context, config Config) error {
       // 1. Detect hardware
       // 2. Verify CUDA/Docker installation
       // 3. Create Docker network
       // 4. Create volumes for models
       // 5. Generate docker-compose.yml
   }
   
   func (op *OnPremProvider) RunModel(ctx context.Context, modelConfig ModelConfig) error {
       // 1. Validate quantization fits in VRAM
       // 2. Pull vLLM image
       // 3. Run vLLM container with model
       // 4. Verify container health
   }
   
   func (op *OnPremProvider) Status(ctx context.Context) (ClusterStatus, error) {
       // Query Docker containers and GPU metrics
   }
   
   func (op *OnPremProvider) Stop(ctx context.Context) error {
       // Stop and remove all containers
   }
   ```

4. **Docker Compose Generator** (`internal/provider/onprem/compose.go`)
   - Generate docker-compose.yml dynamically
   - Include vLLM, Prometheus, Grafana services
   - Bind GPU devices appropriately

5. **Integration Tests** (`internal/provider/onprem/provider_test.go`)
   - Use testcontainers-go for testing
   - Test Initialize(), RunModel(), Status() flows

**Acceptance Criteria:**

- [ ] Hardware detection returns accurate GPU info
- [ ] Docker network and volumes created successfully
- [ ] docker-compose.yml generated with correct GPU bindings
- [ ] vLLM container starts and responds to health check
- [ ] Status() returns accurate GPU utilization
- [ ] Stop() cleanly removes all containers
- [ ] Integration tests pass with testcontainers-go
- [ ] All errors logged with context
- [ ] <50ms overhead on Docker API calls

---

### Task 3: Build Core/Gateway Reverse Proxy with Audit Logging

**Objective:**  
Implement an HTTP reverse proxy that sits before vLLM, logs all requests/responses, applies rate limiting, and validates inputs.

**Deliverables:**

1. **Audit Logger** (`core/audit/audit.go`)
   ```go
   type AuditEntry struct {
       ID        string    `json:"id"`
       Timestamp time.Time `json:"timestamp"`
       UserID    string    `json:"user_id"`
       Method    string    `json:"method"`
       Path      string    `json:"path"`
       Request   []byte    `json:"request"`
       Response  []byte    `json:"response"`
       StatusCode int      `json:"status_code"`
       Duration  int64     `json:"duration_ms"`
       IPAddress string    `json:"ip_address"`
       Error     string    `json:"error,omitempty"`
   }
   
   type AuditLogger interface {
       Log(ctx context.Context, entry AuditEntry) error
       Query(ctx context.Context, filter AuditFilter) ([]AuditEntry, error)
   }
   ```

2. **SQLite Audit Storage** (`core/audit/sqlite.go`)
   - Encrypt audit data at rest using AES-256
   - Create SQLite schema with audit log table
   - Implement Log() and Query() methods
   - Add migration support for schema updates

3. **Rate Limiter** (`core/gateway/ratelimit.go`)
   ```go
   type RateLimiter struct {
       buckets map[string]*TokenBucket  // Per-user buckets
       limit   int                      // Tokens per minute
   }
   
   func (rl *RateLimiter) Allow(userID string) bool
   ```

4. **HTTP Reverse Proxy** (`core/gateway/proxy.go`)
   ```go
   type Proxy struct {
       upstream *url.URL
       audit    core.AuditLogger
       limiter  *RateLimiter
   }
   
   func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
       // 1. Extract user ID / IP
       // 2. Check rate limit
       // 3. Validate request format
       // 4. Proxy to vLLM
       // 5. Log request/response to audit DB
       // 6. Return response
   }
   ```

5. **Gateway Middleware** (`core/gateway/middleware.go`)
   - Request validation middleware (JSON validation)
   - Response interceptor for logging
   - Error handling middleware
   - CORS middleware (for localhost)

6. **Database Setup** (`core/audit/migrations.go`)
   - Create SQLite schema for audit logs
   - Define encryption key derivation (PBKDF2)
   - Implement data retention policy (e.g., 90 days)

7. **Unit Tests** (`core/gateway/proxy_test.go`, `core/audit/audit_test.go`)
   - Mock vLLM backend
   - Test rate limiting behavior
   - Test audit logging accuracy
   - Test encryption/decryption

**Acceptance Criteria:**

- [ ] Proxy successfully forwards requests to vLLM
- [ ] All requests logged to encrypted SQLite DB
- [ ] Rate limiter blocks excessive requests
- [ ] Request/response validation works correctly
- [ ] Gateway latency <50ms
- [ ] Audit logs encrypted at rest
- [ ] Audit logs queryable by timestamp/user
- [ ] Error responses logged with full context
- [ ] 90+ test coverage for gateway module
- [ ] Proxy runs on localhost:8001 (vLLM on localhost:8000)

---

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