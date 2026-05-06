# SovereignStack — Product Requirements Document

## Vision

**SovereignStack is the open-source Private AI Control Plane.**

Deploy any open-weight AI model on any hardware — local PC, remote server, or cloud VM — fully air-gapped. Then see everything it does: cost, usage, performance, and compliance, in one place.

---

## The Problem

Enterprises in regulated industries (healthcare, finance, government, defense) cannot use public AI APIs like OpenAI or AWS Bedrock. The data cannot leave their network. So they face two problems:

**Problem 1 — Deployment is still too hard.**
Setting up a private LLM inference stack means wrestling with NVIDIA drivers, CUDA versions, quantization, Docker networking, and inference servers (vLLM/TGI). It takes a skilled DevOps engineer days. There is no simple, repeatable tool for it.

**Problem 2 — Once deployed, they are blind.**
Even when enterprises successfully deploy a model, they have no visibility into what it is doing. Who is using it? What is it costing? Is sensitive data being sent to an unapproved model? Can they prove compliance to an auditor? There is no answer to any of these questions today.

---

## The Solution

SovereignStack is two layers built on top of each other:

**Layer 1 — Open-Source CLI (Free)**
One command to go from bare metal or a blank VM to a running, production-grade LLM inference stack. Handles NVIDIA drivers, quantization, inference server setup, and a secure gateway automatically. Community-driven and free forever.

**Layer 2 — AI Visibility Platform (Commercial)**
A management plane that gives enterprises complete visibility into their private AI infrastructure. Operational health, real cost vs cloud alternatives, team usage attribution, compliance audit trails, and security anomaly detection — all in one dashboard.

---

## Why This Survives the AI Hype Cycle

AI models will be commoditized. Deployment will become trivial. The companies chasing the model layer will struggle.

The infrastructure management layer does not go away. In five years, whether enterprises are running Llama 3 or a model that doesn't exist yet, they will still need to know:
- What AI is running on their infrastructure
- What it is costing them
- Who is using it and for what
- Whether it is compliant with their industry regulations

This is the same pattern as Kubernetes. Containers were the "exciting moment." Kubernetes became the durable business because it solved the management problem above the containers. SovereignStack is the Kubernetes of private AI infrastructure.

---

## The Moat

**1. Air-Gap Speciality**
AWS, Azure, and GCP are internet-first. Their systems assume connectivity. Regulated enterprises — hospitals, banks, defense contractors — require offline-first infrastructure. The big clouds cannot serve this well because it contradicts their business model. SovereignStack is built offline-first from day one.

**2. Visibility Creates Lock-In**
Once an enterprise has months of usage, cost, and compliance data flowing through the AI Visibility Platform, switching costs are high. This is the same stickiness that Datadog and Splunk built their businesses on.

**3. Community Distribution**
The open-source CLI creates bottom-up adoption. DevOps engineers discover it, use it personally or at work, and champion it internally. The enterprise then pays for the Visibility Platform. This is the HashiCorp / Grafana playbook.

**4. Neutral Party**
AWS will never make it easy to run AI on a Dell server. Their goal is managed service lock-in. SovereignStack has no infrastructure to sell — it is the neutral deployment layer that works everywhere.

---

## Target User

**Primary — DevOps Engineers at Regulated Enterprises**
They install SovereignStack on company infrastructure. They are the champion who brings it into the organisation. Simple enough that one engineer can get it running in an afternoon, powerful enough to run production workloads.

**Secondary — Technical Individuals and Small Teams**
Developers, researchers, or small companies who want private AI without the complexity. They use the free CLI. They become the community that builds the ecosystem.

---

## Business Model

**Tier 1 — Open-Source CLI (Free)**
Full deployment capability, forever free. Community-driven. This is the seed that builds trust and distribution.

**Tier 2 — AI Visibility Platform (Enterprise, $5k–$20k/year per organisation)**
The management dashboard with all five visibility dimensions. This is the commercial product. Priced per organisation, not per node, to avoid penalising growth.

**Tier 3 — Professional Services (Bootstrap Revenue)**
Implementation partnerships with regulated enterprises. Install the stack, configure compliance modules, train the team. This funds development without needing VC money in the early stage. Particularly viable starting in Australia where there is a gap in this market.

---

## The Product

### Layer 1 — Open-Source CLI

#### Core Commands

```bash
# Provision any machine: detects and installs NVIDIA drivers, CUDA, Docker
sovstack init

# Browse and download models from Hugging Face with auto-quantization
sovstack pull <model-name>

# Deploy a model to the inference server (auto-selects best quantization for available VRAM)
sovstack up <model-name>

# Show running models, GPU utilization, health status
sovstack status

# Start the secure gateway (auth, rate limiting, audit logging)
sovstack gateway

# Remove a deployed model or clear cache
sovstack remove <model-name>
```

#### Key Capabilities

**Zero-Touch Installer**
`sovstack init` handles the full provisioning pipeline on a blank machine: NVIDIA driver detection and installation, CUDA version management, NCCL for multi-GPU communication, Docker and NVIDIA Container Toolkit setup. Works on Ubuntu 20.04+. No manual driver hunting.

**Model Store**
`sovstack pull` downloads any open-weight model from Hugging Face. Automatically selects the best quantization (AWQ > FP16 > GPTQ > INT8) based on available VRAM. Stores models in a structured local cache with persistent metadata. Works fully offline after the initial download.

**Inference Engine**
Uses vLLM as the default backend — OpenAI-compatible API, high throughput, production-grade. Single command to go from model files to a running `/v1/chat/completions` endpoint.

**Secure Gateway**
A lightweight Go reverse proxy that sits in front of the inference server. API key authentication, per-user rate limiting (token bucket), request/response audit logging with correlation IDs. All traffic stays local.

**Hardware-Aware**
Reads actual GPU VRAM, CPU cores, and system RAM. Recommends which models will fit. Prevents deploying a model that will OOM. Works on single GPU, multi-GPU, and CPU-only machines.

**Offline-First**
Designed to work with zero internet connectivity after initial setup. No telemetry, no license checks, no phone-home. A requirement for air-gapped enterprise environments.

---

### Layer 2 — AI Visibility Platform (Commercial)

The management plane that answers every question a CTO, compliance officer, or security team will ask about their private AI infrastructure.

#### 1. Operational Visibility
- Real-time dashboard: which models are running, on which machines, with what uptime
- GPU utilization, VRAM usage, temperature, and power draw per node
- Inference latency, throughput (tokens/sec), and queue depth
- Automatic alerting when a model is down or degraded

#### 2. Financial Visibility
- Real cost of running models locally: electricity, hardware amortization, GPU utilization
- Side-by-side comparison: "This month you processed 50M tokens locally. The same volume on OpenAI GPT-4 would have cost $X. On AWS Bedrock: $Y."
- Cost per team, cost per model, cost per request
- ROI dashboard showing cumulative savings since deployment

#### 3. Usage Visibility
- Who is using which model, how much, and when — attributed to teams and individual users
- Request volume trends, peak usage times, model popularity
- Unused model detection (models deployed but not being called)
- Usage forecasting for capacity planning

#### 4. Compliance Visibility
- Immutable audit log: every request, every response, every user, every timestamp
- Exportable compliance reports for HIPAA, SOC 2, ISO 27001, FedRAMP
- Data retention policies per regulation
- Role-based access controls: the HR model is only accessible to the HR team
- Evidence packages for auditors, generated on demand

#### 5. Security Visibility
- Anomaly detection: unusual query volumes, off-hours access, abnormal request patterns
- Data leakage signals: detection of PII or sensitive patterns in prompts
- Failed authentication tracking and alerting
- Model access policy enforcement with violation logging
- Multi-deployment federation: unified security view across all SovereignStack nodes in an organisation

---

## Architecture

```
sovereignstack/
├── cmd/                        # CLI commands (Cobra)
│   ├── init.go                 # Zero-touch provisioning
│   ├── pull.go                 # Model download and cache
│   ├── up.go                   # Model deployment
│   ├── status.go               # Health and resource status
│   ├── gateway.go              # Secure proxy startup
│   └── remove.go               # Model removal
├── core/
│   ├── types.go                # Shared types
│   ├── engine/                 # Deployment orchestrator
│   ├── model/                  # Model registry, cache, quantization
│   ├── gateway/                # Reverse proxy, auth, rate limiting
│   └── audit/                  # Audit logging
├── internal/
│   ├── hardware/               # GPU/CPU/RAM detection
│   ├── docker/                 # Container lifecycle management
│   ├── provider/               # Provider interface (onprem, cloud)
│   │   ├── interface.go
│   │   ├── onprem/             # Docker/K3s implementation
│   │   ├── aws/                # Phase 3
│   │   ├── azure/              # Phase 3
│   │   └── gcp/                # Phase 3
│   └── tunnel/                 # Secure remote access
├── visibility/                 # AI Visibility Platform (commercial layer)
│   ├── collector/              # Metrics and event collection agent
│   ├── api/                    # REST API for dashboard
│   ├── store/                  # Encrypted SQLite / PostgreSQL
│   └── reports/                # Compliance report generators
├── models.yaml                 # Community model registry
└── docker-compose.yml          # Monitoring stack (Prometheus/Grafana)
```

### Provider Interface

All deployment targets implement a single interface. The core logic never knows whether it is talking to a local Docker daemon, a remote server, or a cloud provider.

```go
type Provider interface {
    Initialize(ctx context.Context, config Config) error
    RunModel(ctx context.Context, modelConfig ModelConfig) error
    Status(ctx context.Context) (ClusterStatus, error)
    Stop(ctx context.Context) error
    GetLogs(ctx context.Context, service string, lines int) ([]string, error)
}
```

---

## Model Registry

Models are defined in `models.yaml` and loaded from multiple sources in precedence order:

1. `models.yaml` — bundled defaults (community-maintained)
2. `/etc/sovereignstack/models.yaml` — system-wide additions
3. `~/.sovereignstack/models.yaml` — user-specific models
4. `./models.local.yaml` — project-local overrides

No code changes required to add a new model. Community contributions happen through YAML pull requests.

**Current GPU Models:**
- Meta Llama 3 8B / 70B
- Mistral 7B v0.3 (32k context)
- Mixtral 8x7B
- Qwen2.5 7B / 14B / 72B

**Current CPU Models:**
- TinyLlama 1.1B
- Microsoft Phi-3 Mini
- Google Gemma 2B

---

## Current Implementation Status

### Built
- Hardware detection (GPU/CPU/CUDA/RAM via nvidia-smi and /proc/meminfo)
- Model registry with YAML loading and multi-source precedence
- Quantization calculator (VRAM-aware, AWQ > FP16 > GPTQ > INT8 priority)
- Model cache with persistent metadata
- vLLM container orchestration (docker run with GPU bindings)
- Reverse proxy gateway with API key auth and rate limiting
- In-memory audit logger with correlation IDs

### Not Yet Built (Phase 1 Remaining)
- Actual model download from Hugging Face (`pull` command is a stub)
- Complete deployment pipeline (`deploy` command validates but does not deploy)
- Encrypted SQLite audit log persistence (currently in-memory only)
- `sovstack init` auto-installer for NVIDIA drivers and CUDA
- Provider interface and OnPrem provider implementation
- Integration between gateway and deployed inference server

### Not Yet Built (Phase 2 — Visibility Platform)
- Metrics collection agent
- Visibility dashboard and API
- Financial comparison engine
- Compliance report generators
- Security anomaly detection

---

## Roadmap

### Phase 1 — Open-Source CLI (Current)

**Goal:** One command from blank machine to running LLM. Works anywhere. Fully air-gapped.

- [ ] Implement `sovstack pull` — real Hugging Face download with progress, checksum, quantization
- [ ] Complete `sovstack up` — full deployment pipeline using EngineRoom.Deploy()
- [ ] Implement `sovstack init` — zero-touch NVIDIA/CUDA/Docker provisioning
- [ ] Encrypted SQLite audit log (replacing in-memory logger)
- [ ] OnPrem provider implementation (Docker orchestration)
- [ ] End-to-end integration test: blank Ubuntu → running `/v1/chat/completions`
- [ ] Update model registry to current models (Llama 3, Mistral v0.3, Phi-3)
- [ ] Documentation and quickstart guide

### Phase 2 — AI Visibility Platform (Commercial Layer)

**Goal:** Enterprises can see everything their private AI is doing.

- [ ] Metrics collection agent (embedded in CLI, opt-in)
- [ ] Operational dashboard (GPU, latency, uptime)
- [ ] Financial comparison engine (local cost vs OpenAI/Bedrock)
- [ ] Usage analytics and team attribution
- [ ] Encrypted persistent audit log with compliance report export
- [ ] RBAC — model-level access policies per team/user
- [ ] Security anomaly detection
- [ ] Multi-node federation (unified view across deployments)

### Phase 3 — Cloud Providers and Compliance Modules

**Goal:** Deploy SovereignStack on cloud GPU instances, not just bare metal. Industry-specific compliance packages.

- [ ] AWS provider (EC2 GPU instances)
- [ ] Azure provider (Azure NC-series VMs)
- [ ] GCP provider (A100/H100 instances)
- [ ] HIPAA compliance module
- [ ] SOC 2 compliance module
- [ ] FedRAMP compliance module

---

## Phase 1 Success Criteria

- `sovstack init` runs on a blank Ubuntu 22.04 machine and correctly installs NVIDIA drivers, CUDA, and Docker
- `sovstack pull llama3` downloads the model from Hugging Face with progress display
- `sovstack up llama3` starts a vLLM container and serves `http://localhost:8000/v1/chat/completions`
- `sovstack gateway` proxies requests with API key auth and logs every request to encrypted SQLite
- `sovstack status` shows GPU utilization, running models, and memory usage
- Single binary, no runtime dependencies beyond Docker
- Works fully offline after initial model download
- Builds and runs on Ubuntu 20.04+

---

## Non-Functional Requirements

- **Simplicity:** Any DevOps engineer should be operational in under 30 minutes from a blank machine
- **Offline-First:** Zero internet dependency after initial setup
- **Performance:** Gateway adds less than 50ms latency per request
- **Security:** Audit logs encrypted at rest (AES-256), no telemetry without explicit opt-in
- **Extensibility:** New models via YAML, new providers via interface implementation
- **Portability:** Single binary, cross-platform (Linux primary, macOS for development)
- **Testing:** Integration tests using testcontainers-go, >80% coverage on core packages
