# SovereignStack CLI

A CLI tool that automates the deployment of private, production-grade LLM inference servers on bare metal or VPS. Supports both **GPU and CPU deployments** with intelligent hardware-aware model selection.

## Prerequisites

- Ubuntu 20.04+ or similar Linux distribution
- **NVIDIA GPU (optional)** - works on CPU-only systems
- Docker installed
- Go 1.19+ (for building from source)
- 1GB RAM minimum (CPU-only), 4GB+ recommended for efficient models

## Installation

### Automated Installation

Run the installation script on a fresh Ubuntu server:

```bash
wget https://raw.githubusercontent.com/gayanclife/sovereignstack/main/install.sh
chmod +x install.sh
sudo ./install.sh
```

This will install Docker, NVIDIA drivers (if GPU present), and Go.

### Build from source

```bash
git clone https://github.com/gayanclife/sovereignstack.git
cd sovereignstack
go build -o sovstack .
sudo mv sovstack /usr/local/bin/
```

### Or download pre-built binary

```bash
# Download from releases page
wget https://github.com/gayanclife/sovereignstack/releases/download/v0.1.0/sovstack-linux-amd64
chmod +x sovstack-linux-amd64
sudo mv sovstack-linux-amd64 /usr/local/bin/sovstack
```

## Usage

### Initialize the server

Run this on a fresh Ubuntu server to perform hardware checks and see available models:

```bash
sovstack init
```

This will:
- Detect NVIDIA GPUs (if present) and available VRAM
- Check CUDA driver installation
- Display system CPU cores and total RAM
- Show compatible models for your hardware
- Provide installation instructions if dependencies are missing

**Example output on GPU system:**
```
✓ Detected 1 GPU(s):
  GPU 1: NVIDIA A100 (40384 MB VRAM)
✓ CUDA installed: 11.8
✓ System: 64 CPU cores, 256.0 GB RAM

--- Available Models for Your Hardware ---
✓ 6 model(s) compatible with your hardware:
  • meta-llama/Llama-2-7b-hf (GPU)
  • meta-llama/Llama-2-13b-hf (GPU)
  • mistralai/Mistral-7B-v0.1 (GPU)
  • distilbert-base-uncased (CPU)
  • TinyLlama/TinyLlama-1.1B (CPU)
  • microsoft/phi-2 (CPU)
```

**Example output on CPU-only system:**
```
✗ No NVIDIA GPUs detected
✓ System: 4 CPU cores, 15.0 GB RAM

--- Available Models for Your Hardware ---
✓ 3 model(s) compatible with your hardware:
  • distilbert-base-uncased (CPU)
    Min RAM: 0.5 GB
  • TinyLlama/TinyLlama-1.1B (CPU)
    Min RAM: 3.0 GB
  • microsoft/phi-2 (CPU)
    Min RAM: 6.0 GB
```

### Pull a model

Download model weights from Hugging Face:

```bash
sovstack pull meta-llama/Llama-2-7b-chat-hf
```

### Deploy a model

Start the inference server with intelligent hardware compatibility checking:

```bash
# Deploy a GPU model
sovstack deploy meta-llama/Llama-2-7b-chat-hf

# Or deploy a CPU-optimized model
sovstack deploy TinyLlama/TinyLlama-1.1B
```

The tool will validate the model is compatible with your hardware before deployment:

**✓ Success on compatible hardware:**
```
Deploying model: meta-llama/Llama-2-7b-chat-hf
✓ Model meta-llama/Llama-2-7b-chat-hf is compatible with your hardware
Deployment initiated...
API endpoint available at: http://localhost:8000/v1/chat/completions
```

**✗ Automatic suggestion on incompatible hardware:**
```
Deploying model: meta-llama/Llama-2-7b-chat-hf
✗ Model 'meta-llama/Llama-2-7b-chat-hf' is not compatible with detected hardware

No NVIDIA GPUs detected. This system can run CPU-optimized models only:
  • distilbert-base-uncased (requires 0.5 GB RAM)
  • TinyLlama/TinyLlama-1.1B (requires 3.0 GB RAM)
  • microsoft/phi-2 (requires 6.0 GB RAM)
```

Once deployed, the API will be available at `http://localhost:8000/v1/chat/completions`

## API Usage

Once deployed, use the OpenAI-compatible API:

```python
import openai

client = openai.OpenAI(
    base_url="http://localhost:8000/v1",
    api_key="not-needed"
)

response = client.chat.completions.create(
    model="meta-llama/Llama-2-7b-chat-hf",
    messages=[{"role": "user", "content": "Hello!"}]
)
```

## Supported Models

SovereignStack uses a **configuration-driven model system** that's easy to extend. Models are loaded dynamically from `models.yaml` based on your hardware.

### Built-in Models

**GPU-Optimized:**
- Meta Llama 2 7B (13GB FP16, 3GB AWQ)
- Meta Llama 2 13B (26GB FP16, 6GB AWQ)
- Mistral 7B (13GB FP16, 3GB AWQ, 32k context)

**CPU-Optimized:**
- DistilBERT (250MB, requires 512MB RAM)
- TinyLlama 1.1B (2GB, requires 3GB RAM)
- Microsoft Phi-2 (5GB, requires 6GB RAM)

### Contributing Models

Models are defined in `models.yaml` - **no code changes needed** to add new models!

**Quick example to add a new GPU model:**

```yaml
gpu_models:
  - name: my-awesome-model/7b
    repo: huggingface-org/my-model
    description: "My awesome model"
    parameters: "7B"
    context_length: 4096
    hardware_target: gpu
    sizes: {none: 13858000000, awq: 3200000000, ...}
    required_vram_gb: {none: 14, awq: 4, ...}
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for full model specification guide.

### Model Loading Precedence

1. **Bundled** (`models.yaml`) - Default models in SovereignStack
2. **System** (`/etc/sovereignstack/models.yaml`) - System-wide customization
3. **User** (`~/.sovereignstack/models.yaml`) - Personal model registry  
4. **Project** (`./models.local.yaml`) - Per-project models

Later sources override earlier ones, enabling easy customization.

## Monitoring

Start the monitoring stack:

```bash
docker-compose up -d prometheus grafana node-exporter
```

Access Grafana at `http://localhost:3000` (admin/admin) to monitor:
- Token-per-second (TPS)
- GPU temperature
- Memory usage
- System metrics

## Security

The tool sets up secure networking to ensure the API is only accessible privately without exposing public ports.

## Security

The tool sets up secure networking to ensure the API is only accessible privately without exposing public ports.

## Hardware Compatibility

### CPU-Only Deployment

SovereignStack works on CPU-only systems without any GPU. Recommended models:

- **DistilBERT** (~66M params) - Fast, good for embeddings, minimal RAM (512MB)
- **TinyLlama** (~1.1B params) - Good balance, ~3GB RAM
- **Phi-2** (~2.7B params) - Larger capacity, requires ~6GB RAM

For optimal performance:
```bash
# Check available RAM
free -h

# Run sovstack init to see compatible models
sovstack init

# Deploy the appropriate model for your hardware
sovstack deploy TinyLlama/TinyLlama-1.1B
```

### GPU Deployment

For NVIDIA GPUs, ensure CUDA drivers are installed:

```bash
# Check GPU detection
nvidia-smi

# Run sovstack init to verify
sovstack init

# Deploy a GPU model
sovstack deploy meta-llama/Llama-2-7b-chat-hf
```

## Troubleshooting

### CUDA not detected (GPU systems)
```bash
# Install NVIDIA drivers
sudo apt update
sudo apt install nvidia-driver-XXX
# Reboot and run sovstack init again
```

### Docker permission denied
```bash
sudo usermod -aG docker $USER
# Logout and login again
```

### Model pull fails
Ensure you have a Hugging Face token:
```bash
export HF_TOKEN=your_token_here
```

## Development

```bash
go mod download
go run main.go
```

## License

MIT