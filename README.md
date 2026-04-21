# SovereignStack CLI

A CLI tool that automates the deployment of private, production-grade LLM inference servers on bare metal or VPS.

## Prerequisites

- Ubuntu 20.04+ or similar Linux distribution
- NVIDIA GPU with CUDA support
- Docker installed
- Go 1.19+ (for building from source)

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

Run this on a fresh Ubuntu server to perform hardware checks:

```bash
sovstack init
```

This will:
- Detect NVIDIA GPUs and available VRAM
- Check CUDA driver installation
- Provide instructions if dependencies are missing

### Pull a model

Download model weights from Hugging Face:

```bash
sovstack pull meta-llama/Llama-2-7b-chat-hf
```

### Deploy a model

Start the inference server:

```bash
sovstack deploy meta-llama/Llama-2-7b-chat-hf
```

The API will be available at `http://localhost:8000/v1/chat/completions`

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

## Troubleshooting

### CUDA not detected
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