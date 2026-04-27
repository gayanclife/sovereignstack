sovereignstack/
├── cli/
│   └── cmd/                        # CLI Commands (Cobra)
│       ├── root.go                 # Root command
│       ├── init.go                 # Hardware check
│       ├── deploy.go               # Model deployment
│       └── pull.go                 # Model pulling
│
├── core/                           # Core business logic (Engine Room)
│   ├── types.go                    # Data structures (ModelMetadata, InferenceRequest, etc.)
│   ├── model/
│   │   ├── manager.go              # Model lifecycle management (pull, cache, validate)
│   │   └── quantization.go         # Automatic quantization detection and suggestion
│   └── engine/
│       └── orchestrator.go         # Main inference engine orchestrator (EngineRoom)
│
├── internal/
│   ├── provider/                   # Provider interface (for cloud support in Phase 2)
│   │   └── interface.go            # Standard provider interface
│   ├── hardware/
│   │   └── hardware.go             # GPU/CPU detection, VRAM analysis, CUDA verification
│   └── docker/
│       └── vllm.go                 # Docker vLLM orchestration and management
│
├── pkg/                            # Public packages (reusable components)
│
├── main.go                         # Application entry point
├── go.mod                          # Go dependencies
├── go.sum                          # Dependency lock file
├── README.md                       # Documentation
├── PRD.md                          # Product Requirements Document
├── .gitignore                      # Git ignore rules
└── docker-compose.yml              # Docker Compose configuration


## Engine Room Architecture

### Core Components

**1. Model Manager (core/model/manager.go)**
   - Downloads and caches models from Hugging Face
   - Validates local model availability
   - Manages model registry and metadata
   - Supports common models: Llama 2 (7B, 13B), Mistral 7B

**2. Quantization Engine (core/model/quantization.go)**
   - Analyzes available VRAM
   - Suggests optimal quantization (AWQ, GPTQ, INT8, FP16)
   - Calculates memory requirements per quantization type
   - Formula: (params * bits_per_param / 8) + 15% overhead
   - Priority: AWQ > FP16 > GPTQ > INT8

**3. Hardware Detection (internal/hardware/hardware.go)**
   - Detects NVIDIA GPUs using nvidia-smi
   - Reports GPU VRAM, temperature, CUDA capability
   - Verifies CUDA driver installation
   - Checks Docker availability
   - Reports CPU cores and system RAM

**4. Docker/vLLM Orchestrator (internal/docker/vllm.go)**
   - Manages vLLM container lifecycle
   - Automatic GPU binding
   - Configures memory utilization parameters
   - Health checks and container status monitoring
   - Log retrieval

**5. Engine Room Orchestrator (core/engine/orchestrator.go)**
   - Main inference engine coordinator
   - Deployment planning and execution
   - Model deployment with automatic quantization
   - System monitoring and status reporting
   - Integrates all components


## Quantization Logic

The quantization system automatically analyzes available VRAM and recommends the best option:

- **FP16 (Full Precision)**: Best quality, ~2 bytes per parameter
- **AWQ (INT4)**: 4-bit activation-aware quantization, minimal quality loss
- **GPTQ (INT4)**: Post-training quantization, good trade-off
- **INT8**: Integer 8-bit, moderate quality reduction

Example: 7B parameter model
- FP16: ~14GB VRAM required
- AWQ: ~3.5GB VRAM required  
- INT8: ~7GB VRAM required


## Model Registry

Pre-configured models with size and VRAM requirements:

1. **meta-llama/Llama-2-7b-hf** (7B params)
   - FP16: 14GB, AWQ: 4GB, INT8: 8GB

2. **meta-llama/Llama-2-13b-hf** (13B params)
   - FP16: 28GB, AWQ: 7GB, INT8: 15GB

3. **mistralai/Mistral-7B-v0.1** (7B params)
   - Extended context (32K), similar VRAM to Llama 7B
