# Contributing to SovereignStack

## Model Registry Contributions

SovereignStack uses a **configuration-driven model system** to make it easy for the community to add support for new models without modifying code.

### How to Add a New Model

Models are defined in `models.yaml` (or custom YAML files). To contribute a model:

1. **Edit the model registry file:**
   - Project bundled: `models.yaml`
   - System-wide: `/etc/sovereignstack/models.yaml`
   - User-specific: `~/.sovereignstack/models.yaml`
   - Project-specific: `./models.local.yaml`

2. **Add your model to the appropriate section:**

```yaml
gpu_models:
  - name: my-awesome-model/7b               # Unique identifier
    repo: my-awesome-model/7b-hf            # Hugging Face repo ID
    description: "A brief description"
    parameters: "7B"                        # 7B, 13B, 1.1B, 66M, etc.
    context_length: 4096                    # Max token context
    hardware_target: gpu                    # gpu, cpu, or both
    minimum_system_ram_gb: 0                # Min RAM for CPU models (0 for GPU-only)
    sizes:
      none: 13858000000                     # Bytes for full precision
      int8: 7456000000                      # Bytes for INT8 quantization
      awq: 3200000000                       # Bytes for AWQ quantization
      gptq: 3200000000                      # Bytes for GPTQ quantization
    required_vram_gb:
      none: 14                              # GB needed in full precision
      int8: 8                               # GB needed with INT8
      awq: 4                                # GB needed with AWQ
      gptq: 4                               # GB needed with GPTQ
    default_quantization: awq               # Recommended quantization
    tags: [llm, chat, instruct, 7b]         # For discovery/filtering
```

### Model Categories

**GPU Models:**
- Large LLMs that require NVIDIA GPU
- Set `hardware_target: gpu`
- Set `minimum_system_ram_gb: 0`

**CPU Models:**
- Small, lightweight models optimized for CPU
- Set `hardware_target: cpu`
- Set `minimum_system_ram_gb` to the minimum RAM required
- Set `required_vram_gb` values to 0

**Hybrid Models:**
- Models that work efficiently on both GPU and CPU
- Set `hardware_target: both`
- Provide both CPU RAM and GPU VRAM requirements

### Model Loading Precedence

Models are loaded in this order (later sources override earlier):

1. **Bundled** (`models.yaml`) - Default models included with SovereignStack
2. **System-wide** (`/etc/sovereignstack/models.yaml`) - For system administrators
3. **User-specific** (`~/.sovereignstack/models.yaml`) - Personal model registry
4. **Project-local** (`./models.local.yaml`) - For specific projects

### Example: Adding a New CPU Model

```yaml
cpu_models:
  - name: qwen/qwen-1.8b
    repo: qwen/qwen-1.8b
    description: "Alibaba Qwen 1.8B - fast and efficient"
    parameters: "1.8B"
    context_length: 2048
    hardware_target: cpu
    minimum_system_ram_gb: 2
    sizes:
      none: 3600000000      # ~3.6GB FP16
      int8: 1800000000      # ~1.8GB INT8
    required_vram_gb:
      none: 0
      int8: 0
    default_quantization: int8
    tags: [cpu, llm, efficient]
```

### Example: Adding a GPU Model

```yaml
gpu_models:
  - name: meta-llama/Llama-2-70b-hf
    repo: meta-llama/Llama-2-70b-hf
    description: "Meta's Llama 2 70B - powerful model for complex tasks"
    parameters: "70B"
    context_length: 4096
    hardware_target: gpu
    minimum_system_ram_gb: 0
    sizes:
      none: 140000000000    # ~140GB FP16
      awq: 35000000000      # ~35GB INT4
      gptq: 35000000000     # ~35GB INT4
    required_vram_gb:
      none: 160
      awq: 40
      gptq: 40
    default_quantization: awq
    tags: [llm, chat, powerful, 70b]
```

### Submitting a PR to Add Models

1. **Fork** the repository
2. **Edit** `models.yaml` with your new model(s)
3. **Test** locally:
   ```bash
   ./sovstack init  # Should list your new model
   ```
4. **Submit a PR** with:
   - Model name and description
   - Source (Hugging Face repo ID)
   - Tested on your hardware
   - Benchmarks (optional but appreciated)

### Model Specification Guidelines

#### Size Accuracy
- Use realistic values from actual model files or official documentation
- Include overhead (typically +5-10% for runtime)
- Provide all quantization options, even if not all are common

#### VRAM Requirements
- Test on actual hardware when possible
- Include framework overhead (PyTorch, vLLM, etc.)
- Provide conservative estimates to avoid OOM errors

#### Parameter Counts
- Use exact parameter counts from model cards
- Format: "7B", "13B", "1.1B", "66M", etc.
- Use powers of 10 (B = billion, M = million, K = thousand)

### Model Validation

Before submitting, verify:

```bash
# 1. Models load correctly
./sovstack init

# 2. Model appears in appropriate category
./sovstack init | grep "your-model-name"

# 3. Deploy command recognizes it
./sovstack deploy your-model-name --help

# 4. Check hardware compatibility
# - GPU model shows on GPU systems
# - CPU model only shows if system RAM >= minimum_system_ram_gb
```

### Adding a Custom Model Locally

For development or testing, create `models.local.yaml`:

```yaml
gpu_models:
  - name: my-custom-model
    repo: my-org/my-custom-model
    # ... rest of model definition
```

This file is ignored by git and won't interfere with bundled models.

### Questions?

- Check existing models in `models.yaml` for examples
- Open an issue if you find inconsistencies or errors
- Join our community discussions for model recommendations

## Code Contributions

See [ARCHITECTURE.md](ARCHITECTURE.md) for contributing code changes.
