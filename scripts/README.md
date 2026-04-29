# SovereignStack Scripts

Helper scripts for managing the SovereignStack main stack.

## Available Scripts

### start-management.sh

**Purpose:** Build and start the Management API Docker container

**Features:**
- Automatic environment setup (.env creation)
- Docker and docker-compose validation
- Health check verification
- Automatic image building (with optional force rebuild)
- Real-time logging option
- Status checking and health monitoring

**Usage:**

```bash
# Make script executable (first time only)
chmod +x scripts/start-management.sh

# Start with cached Docker image (fastest)
./scripts/start-management.sh

# Start with fresh rebuild
./scripts/start-management.sh --build

# Start and watch logs
./scripts/start-management.sh --logs

# Check if it's running
./scripts/start-management.sh --status

# Check API health
./scripts/start-management.sh --health

# Stop the container
./scripts/start-management.sh --stop

# Show help
./scripts/start-management.sh --help
```

**Output Example:**

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🚀 SovereignStack Management API Startup
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

ℹ Checking environment...
✓ Created .env file
✓ Docker is installed
✓ Docker Compose is available

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Starting Management API Container
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

ℹ Waiting for service to be healthy...
✓ Management API is healthy

🎉 Management API is running!

Endpoints:
  Health:        http://localhost:8888/api/health
  Running Models: http://localhost:8888/api/models/running

Test it:
  curl http://localhost:8888/api/health
  curl http://localhost:8888/api/models/running | jq .
```

**Aliases (Optional)**

Add to your shell profile (`.bash_profile`, `.zshrc`, etc.) for easy access:

```bash
# In ~/.zshrc or ~/.bash_profile
alias ss-mgmt='~/Projects/sstack/sovereignstack/scripts/start-management.sh'

# Then use:
ss-mgmt                # Start
ss-mgmt --build --logs # Rebuild and watch logs
ss-mgmt --status       # Check status
```

## Quick Start

### 1. First Time Setup

```bash
cd ~/Projects/sstack/sovereignstack

# Make script executable
chmod +x scripts/start-management.sh

# Start the management API
./scripts/start-management.sh

# Script will automatically:
# - Create .env file from .env.example
# - Check Docker installation
# - Build the Docker image
# - Start the container
# - Verify health
```

### 2. Verify It's Running

```bash
# Option A: Use the script
./scripts/start-management.sh --status

# Option B: Direct commands
curl http://localhost:8888/api/health
curl http://localhost:8888/api/models/running | jq .

# Option C: Docker commands
docker-compose ps management
docker-compose logs management
```

### 3. Integrate with Visibility Platform

Once management API is running on port 8888, the visibility platform can discover models:

```bash
cd ~/Projects/sstack/sovereignstack-platform

# Ensure MAIN_STACK_API_URL is set correctly
cat .env | grep MAIN_STACK_API_URL
# Should be: MAIN_STACK_API_URL=http://localhost:8888

# Start visibility platform
docker compose up -d
```

## Advanced Usage

### Build Without Starting

```bash
# Just build the image
docker-compose build management

# Or with script
./scripts/start-management.sh --build
# (and then stop it with --stop if you don't want it running)
```

### View Logs

```bash
# Real-time logs
./scripts/start-management.sh --logs

# Or via docker-compose
docker-compose logs -f management

# Last 50 lines
docker-compose logs management | tail -50

# Filter for errors
docker-compose logs management | grep -i error
```

### Troubleshooting Commands

```bash
# Check if container is running
docker-compose ps management

# Inspect container
docker-compose exec management sh

# Restart container
docker-compose restart management

# Remove container and rebuild
docker-compose down management
./scripts/start-management.sh --build

# Check port usage
lsof -i :8888  # On macOS
netstat -tulpn | grep 8888  # On Linux
```

### Manual Docker Commands

```bash
# Alternative to script (if you prefer docker-compose directly)

# Start
docker-compose up -d management

# Stop
docker-compose down management

# Rebuild
docker-compose build --no-cache management

# View logs
docker-compose logs -f management

# Status
docker-compose ps management
```

## Configuration

### Environment Variables

The script reads from `.env` file. Key variables:

```bash
# Management API port (default: 8888)
MANAGEMENT_PORT=8888

# HuggingFace token (for model downloads)
HF_TOKEN=hf_xxxxxxxxxxxxx

# GPU configuration
CUDA_VISIBLE_DEVICES=0
```

### Changing the Port

```bash
# Edit .env
nano .env

# Change:
MANAGEMENT_PORT=8888  # → 9999

# Restart
./scripts/start-management.sh --build
```

## Integration with Other Services

### With Model Deployment

```bash
# Start management API
./scripts/start-management.sh

# In another terminal, deploy a model
./sovstack deploy distilbert-base-uncased --type cpu

# Management API immediately exposes it
curl http://localhost:8888/api/models/running

# Visibility platform discovers it automatically
```

### With Monitoring Stack

```bash
# Start all services including Prometheus and Grafana
docker-compose up -d

# Then check dashboards
# - Prometheus: http://localhost:9090
# - Grafana: http://localhost:3000
```

## Performance Tips

### Speed Up Image Build

First build takes longer (~2-5 minutes). Subsequent builds use cache:

```bash
# Fast (uses cache): ~10 seconds
./scripts/start-management.sh

# Slow (rebuilds everything): ~3-5 minutes
./scripts/start-management.sh --build

# Very slow (no cache): ~5-10 minutes
docker-compose build --no-cache management
```

### Optimize Startup Time

```bash
# Pre-build image at setup time
./scripts/start-management.sh

# Then subsequent starts are instant
docker-compose up -d management
```

## Documentation

For more information, see:

- [DOCKER_COMPOSE_SERVICES.md](../docs/DOCKER_COMPOSE_SERVICES.md) - Details on all Docker services
- [MANAGEMENT_API.md](../docs/MANAGEMENT_API.md) - API endpoint documentation
- [MAIN_STACK_INTEGRATION.md](../docs/MAIN_STACK_INTEGRATION.md) - Visibility platform integration
