# Main Stack Docker Compose Services Guide

This document explains the purpose and configuration of each container in the SovereignStack main stack's `docker-compose.yml` file.

## Quick Reference

| Service | Port | Purpose | Required |
|---------|------|---------|----------|
| **management** | 8888 | Model discovery & REST API | ✅ Yes |
| **vllm** | 8000 | Model inference engine | ✅ Yes |
| **node-exporter** | 9100 | System metrics collector | ⚠️ Optional |
| **prometheus** | 9090 | Metrics time-series database | ⚠️ Optional |
| **grafana** | 3000 | Metrics dashboard UI | ⚠️ Optional |

---

## Service Details

### 1. Management (Port 8888) ✅ **REQUIRED**

**Purpose:** REST API that exposes running models and system information

**Used by:** Visibility Platform (for model discovery)

**Key Features:**
- Queries Docker daemon for running model containers
- Exposes `/api/health` health check endpoint
- Exposes `/api/models/running` endpoint returning running models as JSON
- Auto-restarts on failure
- Includes health checks every 30 seconds

**Configuration:**
```yaml
build:
  context: .
  dockerfile: Dockerfile.management
ports:
  - "8888:8888"
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro  # Read Docker daemon
  - ~/.sovereignstack:/root/.sovereignstack:ro    # Config access
```

**Endpoints:**
```bash
# Health check
curl http://localhost:8888/api/health
# {"status":"ok","ready":true}

# List running models
curl http://localhost:8888/api/models/running
# {
#   "version": "1.0",
#   "models": [...],
#   "count": 2
# }
```

**Depends on:** vllm (waits for vllm to start first)

---

### 2. vLLM (Port 8000) ✅ **REQUIRED**

**Purpose:** Large Language Model inference engine for running AI models

**Image:** `vllm/vllm-openai:latest` (official vLLM with OpenAI API compatibility)

**Used by:**
- Model inference requests from users
- Management API to query running models
- System metrics collection

**Key Features:**
- Runs on GPU (requires NVIDIA GPU + nvidia-docker)
- OpenAI-compatible API
- Dynamic batching for high throughput
- Configurable model name via environment
- Shared memory (1GB) for model weights
- HuggingFace cache mounting

**Configuration:**
```yaml
image: vllm/vllm-openai:latest
ports:
  - "8000:8000"
volumes:
  - ~/.cache/huggingface:/root/.cache/huggingface
environment:
  - CUDA_VISIBLE_DEVICES=0              # GPU device ID
  - HF_TOKEN=${HF_TOKEN}                # HuggingFace auth
command: --model ${MODEL_NAME} --gpu-memory-utilization 0.9 --host 0.0.0.0 --port 8000
```

**Environment Variables:**
- `MODEL_NAME` - Model to load (e.g., `meta-llama/Llama-2-7b`)
- `HF_TOKEN` - HuggingFace token for gated models
- `CUDA_VISIBLE_DEVICES` - Which GPU(s) to use

**Example Usage:**
```bash
# Make inference request (OpenAI compatible)
curl http://localhost:8000/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "meta-llama/Llama-2-7b",
    "prompt": "Once upon a time",
    "max_tokens": 100
  }'
```

**Requirements:**
- NVIDIA GPU with CUDA support
- nvidia-docker runtime
- At least 20GB VRAM for 7B models

---

### 3. Node Exporter (Port 9100) ⚠️ **Optional**

**Purpose:** Collects system-level metrics (CPU, memory, disk, network)

**Image:** `prom/node-exporter`

**Used by:** Prometheus (scrapes metrics every 15 seconds)

**Key Features:**
- Reads system statistics from `/proc` and `/sys`
- Exposes Prometheus-format metrics
- No state or configuration needed
- Runs on host OS directly

**Configuration:**
```yaml
volumes:
  - /proc:/host/proc:ro          # Read CPU, memory, processes
  - /sys:/host/sys:ro            # Read disk, network
  - /:/rootfs:ro                 # Read filesystem
command:
  - --path.procfs=/host/proc
  - --path.rootfs=/rootfs
  - --path.sysfs=/host/sys
```

**Metrics Exposed:**
```
node_cpu_seconds_total         # CPU time
node_memory_MemTotal_bytes     # Total RAM
node_memory_MemAvailable_bytes # Free RAM
node_disk_io_reads_completed   # Disk reads
node_network_receive_bytes     # Network in
node_network_transmit_bytes    # Network out
```

**Usage:**
```bash
# View raw metrics
curl http://localhost:9100/metrics | grep node_memory_MemAvailable_bytes
# node_memory_MemAvailable_bytes 16GB...
```

**When to Use:**
- ✅ When monitoring system performance
- ✅ When tracking resource utilization over time
- ❌ If only monitoring model inference metrics
- ❌ On resource-constrained systems

---

### 4. Prometheus (Port 9090) ⚠️ **Optional**

**Purpose:** Time-series database for metrics storage and querying

**Image:** `prom/prometheus`

**Used by:**
- Node Exporter (scrapes system metrics)
- vLLM metrics (if enabled)
- Grafana (queries metrics for dashboards)

**Key Features:**
- Scrapes metrics every 15 seconds (configurable)
- Stores metrics for 15 days by default
- Full-text search and alerting support
- HTTP API for metric queries

**Configuration:**
```yaml
volumes:
  - ./monitoring/prometheus.yml:/etc/prometheus/prometheus.yml
command:
  - --config.file=/etc/prometheus/prometheus.yml
```

**Sample prometheus.yml:**
```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'node'
    static_configs:
      - targets: ['node-exporter:9100']
  
  - job_name: 'vllm'
    static_configs:
      - targets: ['vllm:8000']
```

**Query Examples:**
```bash
# Via HTTP API
curl 'http://localhost:9090/api/v1/query?query=node_memory_MemAvailable_bytes'

# CPU usage (percentage)
curl 'http://localhost:9090/api/v1/query?query=100*(1-avg(rate(node_cpu_seconds_total{mode=%22idle%22}[5m])))'

# Memory usage (percentage)
curl 'http://localhost:9090/api/v1/query?query=100*(1-(node_memory_MemAvailable_bytes/node_memory_MemTotal_bytes))'
```

**When to Use:**
- ✅ For historical metrics retention
- ✅ For alerting on metric thresholds
- ✅ For long-term trend analysis
- ❌ If only doing real-time monitoring
- ❌ On low-disk systems (stores time-series data)

---

### 5. Grafana (Port 3000) ⚠️ **Optional**

**Purpose:** Visual dashboards for monitoring metrics and system health

**Image:** `grafana/grafana`

**Used by:**
- Developers/operators monitoring system health
- On-call engineers tracking performance

**Key Features:**
- Beautiful dashboard visualizations (graphs, gauges, tables)
- Multiple data sources (Prometheus, etc.)
- Alerting rules and notifications
- User management and role-based access
- Built-in templates and plugins

**Configuration:**
```yaml
ports:
  - "3000:3000"
environment:
  - GF_SECURITY_ADMIN_PASSWORD=admin  # Default password (CHANGE THIS!)
volumes:
  - grafana_data:/var/lib/grafana     # Persistent dashboard storage
```

**Access:**
```
URL: http://localhost:3000
Default User: admin
Default Password: admin
```

**Dashboard Setup:**
1. Add Prometheus as data source
   - Settings → Data Sources → Add
   - Type: Prometheus
   - URL: http://prometheus:9090
   
2. Create dashboard
   - Create → Dashboard
   - Add Panel
   - Select Prometheus queries

**Example Queries:**
```promql
# CPU usage percentage
100*(1-avg(rate(node_cpu_seconds_total{mode="idle"}[5m])))

# Memory usage percentage
100*(1-(node_memory_MemAvailable_bytes/node_memory_MemTotal_bytes))

# Disk usage percentage
100*(1-(node_filesystem_avail_bytes/node_filesystem_size_bytes))

# Network traffic (bytes/sec)
rate(node_network_receive_bytes_total[1m])
```

**When to Use:**
- ✅ For visual monitoring dashboards
- ✅ For team visibility into system health
- ✅ For presentations and reports
- ❌ If metrics only accessed programmatically
- ❌ On systems with UI access restrictions

---

## Service Dependencies

```
┌─────────────────────────────────────────┐
│         Docker Daemon                   │
│  (containers, networks, volumes)        │
└──────────────┬──────────────────────────┘
               │
       ┌───────┴───────┐
       │               │
   ┌───▼─────┐    ┌───▼──────┐
   │ vLLM    │    │Management│  (required)
   │(model)  │    │(REST API)│
   └────┬────┘    └──────────┘
        │              │
        │              └─────────────────┐
        │                                │
   ┌────▼────────────┐         ┌─────────▼──────┐
   │ Node Exporter   │         │ Visibility     │
   │(metrics)        │         │ Platform       │
   └────┬────────────┘         │(queries API)   │
        │                      └────────────────┘
        │
   ┌────▼────────────┐
   │ Prometheus      │  (optional)
   │(time-series DB) │
   └────┬────────────┘
        │
   ┌────▼────────────┐
   │ Grafana         │  (optional)
   │(dashboards)     │
   └─────────────────┘
```

---

## Startup Scenarios

### Scenario 1: Visibility Platform Only (Recommended for Production)
**Start only required containers:**
```bash
docker-compose up -d management vllm
```

**What you get:**
- ✅ Model serving (vLLM)
- ✅ API for model discovery (Management)
- ✅ Visibility Platform can monitor models
- ❌ No metrics dashboards
- ❌ No historical data

---

### Scenario 2: Full Monitoring Stack (Development/Debugging)
**Start all containers:**
```bash
docker-compose up -d
```

**What you get:**
- ✅ Model serving
- ✅ API for model discovery
- ✅ Visibility Platform integration
- ✅ System metrics collection
- ✅ Metrics storage and querying
- ✅ Visual dashboards

**URLs:**
- Models: http://localhost:8888/api/models/running
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000

---

### Scenario 3: Minimal (CPU-only, No GPU)
**Start without vLLM (if you have your own model serving):**
```bash
docker-compose up -d management
```

**Use case:**
- Models already running elsewhere
- Only need model discovery API
- Management container queries existing containers

---

## Performance Impact

| Service | CPU | Memory | Disk | GPU |
|---------|-----|--------|------|-----|
| management | <5% | 50MB | 100MB | No |
| vllm | 20-60% | 8-20GB | 5-10GB | Yes (Primary) |
| node-exporter | <1% | 10MB | 10MB | No |
| prometheus | 5-10% | 100-500MB | 1-10GB | No |
| grafana | 2-5% | 100MB | 100MB | No |

**Total overhead (excl. vllm):** ~50MB RAM, <200MB disk, <15% CPU

---

## Troubleshooting

### Management API won't start
```bash
# Check logs
docker-compose logs management

# Common issues:
# - Port 8888 already in use: change MANAGEMENT_PORT in .env
# - Docker socket permission denied: run with sudo or add user to docker group
# - vLLM not ready: management waits for vLLM, check vLLM logs
```

### No metrics in Prometheus
```bash
# Check Prometheus targets
curl http://localhost:9090/api/v1/targets

# Verify node-exporter is running
curl http://localhost:9100/metrics
```

### Grafana won't show data
```bash
# Verify Prometheus data source is configured
curl http://localhost:9090/api/v1/query?query=up

# If no results, metrics aren't being scraped
# Check node-exporter is running and reachable from prometheus
```

---

## Security Notes

**⚠️ Default Configurations Are NOT Production-Ready:**

1. **Grafana** - Change default password (admin/admin)
   ```bash
   docker-compose exec grafana grafana-cli admin reset-admin-password <newpassword>
   ```

2. **Management API** - No authentication required
   - For production, add API key requirement
   - Restrict network access via firewall

3. **Prometheus** - No authentication, accessible on port 9090
   - Consider behind reverse proxy in production

4. **HuggingFace Token** - Store in .env, never commit
   ```bash
   # .env
   HF_TOKEN=hf_xxxxxxxxxxxxx  # Keep secret!
   ```

---

## Next Steps

- **Visibility Platform Setup:** See [MAIN_STACK_INTEGRATION.md](./MAIN_STACK_INTEGRATION.md)
- **Model Management:** See [Model Commands](./MODEL_COMMANDS.md)
- **Production Deployment:** See [PRODUCTION_GUIDE.md](./PRODUCTION_GUIDE.md)
