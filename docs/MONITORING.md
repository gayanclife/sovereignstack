# SovereignStack Monitoring Guide

Complete reference for monitoring the main stack using Prometheus, Node-Exporter, cAdvisor, and Grafana.

---

## Overview

The monitoring stack consists of four complementary components:

```
┌─────────────────────────────────────────┐
│  Grafana (localhost:3001)               │
│  Web UI for dashboards & visualizations │
└────────────────┬────────────────────────┘
                 │
                 ↓
┌─────────────────────────────────────────┐
│  Prometheus (localhost:9090)            │
│  Metrics storage & time-series DB       │
└───────┬─────────────┬──────────────┬────┘
        │             │              │
        ↓             ↓              ↓
    vLLM         Node-Exporter    cAdvisor
  (port 8000)    (port 9100)      (port 8080)
    Metrics      Host Metrics    Container Metrics
```

### What Each Component Does

| Component | Port | Purpose | Metrics |
|-----------|------|---------|---------|
| **vLLM** | 8000 | LLM inference engine | Request latency, throughput, token rates |
| **Node-Exporter** | 9100 | Host system metrics | CPU, RAM, disk, network I/O |
| **cAdvisor** | 8080 | Docker container metrics | Container CPU, memory, I/O, network |
| **Prometheus** | 9090 | Metrics database | Collects & stores metrics from above |
| **Grafana** | 3001 | Dashboarding UI | Visualizes metrics from Prometheus |

---

## Quick Start (5 minutes)

### 1. Start the Monitoring Stack

```bash
cd /home/gayangunapala/Projects/sstack/sovereignstack
docker compose up -d prometheus node-exporter cadvisor grafana
```

### 2. Verify Services Are Running

```bash
docker compose ps | grep -E 'prometheus|grafana|node-exporter|cadvisor'
```

Expected output:
```
sovereignstack-prometheus    prom/prometheus:latest    Up (healthy)
sovereignstack-grafana       grafana/grafana:latest    Up
sovereignstack-node-exporter prom/node-exporter:latest Up
sovereignstack-cadvisor      gcr.io/cadvisor/cadvisor  Up
```

### 3. Access Prometheus

Open http://localhost:9090 and navigate to Status → Targets

You should see all scrape targets:
- ✅ vllm (up)
- ✅ node (up)
- ✅ cadvisor (up)
- ✅ prometheus (up)

### 4. Access Grafana

Open http://localhost:3001

- **Username:** admin
- **Password:** admin (you'll be prompted to change it)

### 5. Add Prometheus as Data Source

1. Go to Configuration → Data Sources → Add Data Source
2. Select Prometheus
3. URL: `http://prometheus:9090`
4. Click "Save & Test" (should show "Data source is working")

### 6. Import Pre-built Dashboard

1. Click **+** → Import
2. Import ID: **1860** (Node Exporter Full)
3. Select Data Source: Prometheus
4. Click "Import"

You should now see real-time system metrics!

---

## Understanding Node-Exporter

### What It Does

Node-Exporter is a Prometheus exporter that collects **host system metrics** from the Linux machine.

**Metrics collected:**
- CPU usage (user, system, idle)
- Memory (total, used, available)
- Disk I/O (reads, writes, latency)
- Network I/O (bytes in/out, errors)
- Process metrics
- Temperature sensors
- And 50+ more collectors

### Data Sources

Node-Exporter reads from Linux kernel interfaces:

```
/proc/stat       → CPU metrics
/proc/meminfo    → Memory metrics
/proc/diskstats  → Disk I/O
/proc/net/dev    → Network metrics
/sys/class/      → Hardware sensors, temperature
/etc/mtab        → Filesystem mounts
```

### Configuration in docker-compose.yml

```yaml
node-exporter:
  image: prom/node-exporter:latest
  volumes:
    - /proc:/host/proc:ro          # Read-only proc filesystem
    - /sys:/host/sys:ro            # Read-only sys filesystem
    - /:/rootfs:ro                 # Read-only root filesystem
  command:
    - '--path.procfs=/host/proc'   # Where /proc is mounted
    - '--path.rootfs=/rootfs'      # Where root is mounted
    - '--path.sysfs=/host/sys'     # Where /sys is mounted
    - '--collector.filesystem.mount-points-exclude=^/(sys|proc|dev|host|etc)($$|/)'
```

### Key Node-Exporter Metrics

```prometheus
# CPU Metrics
node_cpu_seconds_total{cpu="0",mode="user"}     # CPU time in user space
node_cpu_seconds_total{cpu="0",mode="system"}   # CPU time in kernel
node_cpu_seconds_total{cpu="0",mode="idle"}     # CPU idle time

# Memory Metrics
node_memory_MemTotal_bytes                      # Total RAM
node_memory_MemAvailable_bytes                  # Available RAM
node_memory_MemFree_bytes                       # Free RAM

# Disk Metrics
node_disk_io_reads_completed_total{device="sda"}    # Total reads
node_disk_io_writes_completed_total{device="sda"}   # Total writes
node_disk_reads_time_seconds_total{device="sda"}    # Time spent reading
node_disk_writes_time_seconds_total{device="sda"}   # Time spent writing

# Network Metrics
node_network_receive_bytes_total{device="eth0"}     # Bytes received
node_network_transmit_bytes_total{device="eth0"}    # Bytes transmitted
node_network_receive_errs_total{device="eth0"}      # Receive errors

# Filesystem Metrics
node_filesystem_size_bytes{fstype="ext4",mountpoint="/"}    # Total size
node_filesystem_avail_bytes{fstype="ext4",mountpoint="/"}   # Available space
node_filesystem_files{fstype="ext4",mountpoint="/"}         # Inode count
```

### Calculating Common Metrics from Node-Exporter

#### CPU Utilization Percentage

```prometheus
# CPU usage in last 5 minutes (all cores)
100 * (1 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m])))

# Per-core CPU usage
100 * (1 - rate(node_cpu_seconds_total{mode="idle",cpu="0"}[5m]))
```

#### Memory Usage Percentage

```prometheus
# RAM usage percentage
100 * (1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)

# RAM used in GB
node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes) / 1024 / 1024 / 1024
```

#### Disk Usage Percentage

```prometheus
# Disk usage on root partition
100 * (node_filesystem_size_bytes{mountpoint="/"} - node_filesystem_avail_bytes{mountpoint="/"}) 
    / node_filesystem_size_bytes{mountpoint="/"}
```

#### Network Bandwidth

```prometheus
# Download bandwidth (Mbps) in last 1 minute
rate(node_network_receive_bytes_total[1m]) * 8 / 1000 / 1000

# Upload bandwidth (Mbps) in last 1 minute
rate(node_network_transmit_bytes_total[1m]) * 8 / 1000 / 1000
```

#### Disk I/O Performance

```prometheus
# Read latency (ms)
rate(node_disk_reads_time_seconds_total[1m]) / rate(node_disk_io_reads_completed_total[1m]) * 1000

# Write latency (ms)
rate(node_disk_writes_time_seconds_total[1m]) / rate(node_disk_io_writes_completed_total[1m]) * 1000
```

---

## Understanding cAdvisor

### What It Does

cAdvisor (Container Advisor) is Google's container metrics exporter for Docker.

**Metrics collected per container:**
- CPU usage and limits
- Memory usage and limits
- Network I/O
- Block I/O (disk reads/writes)
- Container lifecycle events

### How It Works

cAdvisor runs in a container and reads from:

```
/var/lib/docker/              → Docker container data
/var/run/docker.sock          → Docker daemon socket
/sys/fs/cgroup/               → Linux cgroup metrics
/proc/                        → Process information
```

### Configuration in docker-compose.yml

```yaml
cadvisor:
  image: gcr.io/cadvisor/cadvisor:latest
  volumes:
    - /:/rootfs:ro                # Root filesystem (read-only)
    - /var/run:/var/run:rw        # Docker runtime (read-write)
    - /sys:/sys:ro                # Sysfs (read-only)
    - /var/lib/docker/:/var/lib/docker:ro  # Docker data
  privileged: true              # Needed to read cgroup metrics
  command:
    - '--port=8080'
    - '--housekeeping_interval=10s'  # Metrics collection interval
```

### Key cAdvisor Metrics

```prometheus
# Container CPU
container_cpu_usage_seconds_total{container_name="vllm"}      # Total CPU time used
container_cpu_cfs_throttled_seconds_total{container_name="vllm"}  # Time throttled

# Container Memory
container_memory_usage_bytes{container_name="vllm"}           # Current memory
container_memory_max_usage_bytes{container_name="vllm"}       # Peak memory
container_memory_limit_bytes{container_name="vllm"}           # Memory limit

# Container Network
container_network_receive_bytes_total{container_name="vllm"}  # Bytes received
container_network_transmit_bytes_total{container_name="vllm"} # Bytes transmitted

# Container I/O
container_fs_reads_bytes_total{container_name="vllm"}         # Bytes read from disk
container_fs_writes_bytes_total{container_name="vllm"}        # Bytes written to disk
```

### Calculating Container Metrics

#### Container CPU Percentage

```prometheus
# CPU usage percentage (0-100% per core)
rate(container_cpu_usage_seconds_total{container_name="vllm"}[1m]) * 100
```

#### Container Memory Percentage

```prometheus
# Memory usage as percentage of limit
100 * container_memory_usage_bytes{container_name="vllm"} 
    / container_memory_limit_bytes{container_name="vllm"}
```

#### Container Network Bandwidth

```prometheus
# Network RX Mbps
rate(container_network_receive_bytes_total{container_name="vllm"}[1m]) * 8 / 1000 / 1000

# Network TX Mbps
rate(container_network_transmit_bytes_total{container_name="vllm"}[1m]) * 8 / 1000 / 1000
```

---

## Understanding Prometheus

### What It Does

Prometheus is a **time-series metrics database** that:
1. Scrapes metrics from exporters (vLLM, Node-Exporter, cAdvisor)
2. Stores metrics with timestamps
3. Provides query language (PromQL) to analyze metrics
4. Evaluates alert rules

### Configuration (prometheus.yml)

```yaml
global:
  scrape_interval: 15s        # How often to scrape targets
  evaluation_interval: 15s    # How often to evaluate alerts

scrape_configs:
  - job_name: 'vllm'
    scrape_interval: 10s      # Override global interval
    static_configs:
      - targets: ['localhost:8000']  # What to scrape
    metrics_path: '/metrics'  # Where to scrape from
```

### Accessing Prometheus

**Web UI:** http://localhost:9090

**Key sections:**
- **Graph** → Query metrics with PromQL
- **Alerts** → View triggered alerts
- **Status → Targets** → See scrape targets (up/down)
- **Status → Configuration** → View current config
- **Status → Flags** → View runtime flags

### PromQL Query Examples

```promql
# Current CPU usage
node_cpu_seconds_total

# CPU usage rate in last 5 minutes
rate(node_cpu_seconds_total[5m])

# Memory available (bytes)
node_memory_MemAvailable_bytes

# Memory available in GB
node_memory_MemAvailable_bytes / 1024 / 1024 / 1024

# Total requests per second (from vLLM)
rate(vllm_requests_total[1m])

# P99 request latency (milliseconds)
histogram_quantile(0.99, vllm_request_latency_seconds) * 1000

# Model throughput (tokens per second)
rate(vllm_tokens_generated_total[1m])

# Docker container memory usage
container_memory_usage_bytes{container_name="vllm"}

# Count of running containers
count(container_memory_usage_bytes)
```

### Common PromQL Functions

| Function | Purpose | Example |
|----------|---------|---------|
| `rate()` | Per-second rate of increase | `rate(node_cpu_seconds_total[5m])` |
| `increase()` | Total increase over time | `increase(requests_total[1h])` |
| `avg()` | Average across labels | `avg(node_cpu_seconds_total)` |
| `sum()` | Sum across labels | `sum(container_memory_usage_bytes)` |
| `max()` / `min()` | Maximum/minimum | `max(cpu_usage)` |
| `histogram_quantile()` | Percentile from histogram | `histogram_quantile(0.95, request_duration)` |
| `topk()` | Top N results | `topk(5, requests_total)` |
| `on()` | Label-based join | `metric1 on(job) metric2` |

### Data Retention

Default: **15 days** (configured in docker-compose.yml)

```yaml
command:
  - '--storage.tsdb.retention.time=30d'  # Keep 30 days of data
```

Change by editing `docker-compose.yml` and restarting:

```bash
docker compose down prometheus
docker compose up -d prometheus
```

---

## Understanding Grafana

### What It Does

Grafana is a **visualization platform** that:
1. Queries Prometheus for metrics
2. Displays data in real-time dashboards
3. Creates alerts from metrics
4. Supports templating and variables

### Access Grafana

**URL:** http://localhost:3001

**Default credentials:**
- Username: `admin`
- Password: `admin`

> Change password on first login!

### Building a Dashboard

#### 1. Create a New Dashboard

Dashboard → New → New Dashboard → Add Panel

#### 2. Configure Data Source

1. Click panel title → Edit
2. Data Source: Select "Prometheus"
3. Metrics: Choose metric from dropdown

#### 3. Write PromQL Query

Example: CPU usage percentage

```promql
100 * (1 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m])))
```

#### 4. Configure Visualization

- **Graph:** Time-series line/area chart
- **Gauge:** Single value gauge (0-100%)
- **Stat:** Large single-value display
- **Table:** Tabular data
- **Heatmap:** 2D time-series distribution

#### 5. Add Threshold/Alerts

- Yellow threshold: 70%
- Red threshold: 90%

### Pre-built Dashboards

Grafana marketplace has pre-built dashboards for common monitoring:

1. Go to Dashboards → Import
2. Enter dashboard ID or upload JSON
3. Popular IDs:
   - **1860** — Node Exporter Full (system metrics)
   - **14282** — Docker Container Metrics (cAdvisor)
   - **3662** — Prometheus (self-monitoring)

### Creating Alerts

1. Click Panel → Edit → Alert
2. Set condition: e.g., "CPU > 80%"
3. Set evaluation period: e.g., "for 5 minutes"
4. Configure notification channel (email, Slack, webhook)

---

## Monitoring vLLM

### vLLM Metrics

vLLM exports Prometheus metrics on `/metrics` endpoint (port 8000).

**Key metrics:**

```prometheus
# Request metrics
vllm_requests_total                    # Total requests processed
vllm_request_latency_seconds           # Request latency histogram
vllm_request_latency_seconds_bucket    # Latency percentiles

# Token metrics
vllm_tokens_generated_total            # Total tokens generated
vllm_tokens_input_total                # Total input tokens
vllm_tokens_output_total               # Total output tokens

# Model metrics
vllm_model_input_tokens_total          # Tokens by model
vllm_model_inference_duration_seconds  # Time spent on inference

# Queue metrics
vllm_queue_length                      # Current queue depth
vllm_queue_waiting_time_seconds        # Time waiting in queue
```

### vLLM Prometheus Scrape Config

```yaml
- job_name: 'vllm'
  scrape_interval: 10s
  static_configs:
    - targets: ['localhost:8000']
  metrics_path: '/metrics'
```

### Useful vLLM Queries

```promql
# Request throughput (req/s)
rate(vllm_requests_total[1m])

# P50 latency (ms)
histogram_quantile(0.50, vllm_request_latency_seconds) * 1000

# P99 latency (ms)
histogram_quantile(0.99, vllm_request_latency_seconds) * 1000

# Token generation rate (tok/s)
rate(vllm_tokens_generated_total[1m])

# Queue depth
vllm_queue_length

# GPU memory usage (via container metrics)
container_memory_usage_bytes{container_name="vllm"} / 1024 / 1024 / 1024
```

---

## Monitoring Docker Containers

### What cAdvisor Provides

cAdvisor automatically discovers and monitors **all running containers**.

```promql
# List all containers
container_memory_usage_bytes

# Count containers by status
count(container_memory_usage_bytes)

# Memory usage by container (top 10)
topk(10, container_memory_usage_bytes)

# CPU usage by container
rate(container_cpu_usage_seconds_total[1m]) * 100

# Network I/O by container
rate(container_network_receive_bytes_total[1m])
```

### Creating Container Monitoring Dashboard

1. New Panel: Container Memory Usage
   ```promql
   container_memory_usage_bytes{container_name!=""}
   ```

2. New Panel: Container CPU Usage
   ```promql
   rate(container_cpu_usage_seconds_total{container_name!=""}[1m]) * 100
   ```

3. New Panel: Container Network RX
   ```promql
   rate(container_network_receive_bytes_total{container_name!=""}[1m])
   ```

### Alerting on Container Metrics

Example: Alert if vLLM container memory > 30GB

```yaml
- alert: VLLMHighMemory
  expr: container_memory_usage_bytes{container_name="vllm"} > 30 * 1024 * 1024 * 1024
  for: 5m
  annotations:
    summary: "vLLM memory usage is high ({{ $value | humanize }})"
```

---

## Troubleshooting

### Prometheus says "No targets found"

**Check:** Are all scrape targets running?

```bash
curl -s http://localhost:8000/metrics | head    # vLLM
curl -s http://localhost:9100/metrics | head    # Node-Exporter
curl -s http://localhost:8080/metrics | head    # cAdvisor
```

If any fail, restart that service:
```bash
docker compose restart vllm     # or node-exporter, cadvisor, etc.
```

### Prometheus metrics not updating

**Check:**
1. Are scrapers running? `docker compose ps`
2. Is prometheus storing data? `docker exec sovereignstack-prometheus df /prometheus`
3. Look at Prometheus logs: `docker compose logs prometheus`

### cAdvisor not showing container metrics

**Cause:** cAdvisor needs privileged mode and Docker socket access

**Fix:** Verify docker-compose.yml has:
```yaml
cadvisor:
  privileged: true
  volumes:
    - /var/run/docker.sock  # (not in current config, may need to add)
    - /var/lib/docker:/var/lib/docker:ro
```

If not present, restart:
```bash
docker compose down cadvisor
docker compose up -d cadvisor
```

### Grafana can't reach Prometheus

**Check:** Data Source configuration
1. Go to Configuration → Data Sources
2. Click Prometheus data source
3. URL should be: `http://prometheus:9090` (not localhost)

**If still fails:**
```bash
docker compose exec grafana curl http://prometheus:9090
```

If connection refused:
```bash
docker compose restart prometheus grafana
```

---

## Best Practices

### 1. Retention Policy

Keep metrics for 30 days (balance between storage and historical data):

```yaml
prometheus:
  command:
    - '--storage.tsdb.retention.time=30d'
```

### 2. Scrape Intervals

- **vLLM metrics:** 10 seconds (fast-changing)
- **Node metrics:** 15 seconds (stable)
- **cAdvisor metrics:** 10 seconds (container activity)

### 3. Resource Limits

Add to docker-compose.yml for production:

```yaml
prometheus:
  deploy:
    resources:
      limits:
        memory: 2G
      reservations:
        memory: 1G

grafana:
  deploy:
    resources:
      limits:
        memory: 500M
```

### 4. Data Backup

Backup Prometheus data regularly:

```bash
docker exec sovereignstack-prometheus tar czf /tmp/prometheus-backup.tar.gz /prometheus
docker cp sovereignstack-prometheus:/tmp/prometheus-backup.tar.gz ./backups/
```

### 5. Alerting Strategy

Create alerts for:
- vLLM container memory > 30GB
- Model request latency P99 > 5s
- Node CPU > 80% for 5 minutes
- Node disk usage > 85%
- Container down (not in cAdvisor)

---

## Advanced: Custom Metrics

### Exposing Custom Metrics from Your App

If you want to add custom metrics:

1. Create a metrics endpoint in your application (e.g., port 8081)
2. Add scrape config in prometheus.yml:

```yaml
- job_name: 'my-app'
  static_configs:
    - targets: ['localhost:8081']
  metrics_path: '/metrics'
```

3. Use Prometheus client library to expose metrics:

```go
// Go example
import "github.com/prometheus/client_golang/prometheus"

var requestDuration = prometheus.NewHistogram(
    prometheus.HistogramOpts{
        Name: "request_duration_seconds",
        Help: "Request latency",
    },
)

// In your handler:
start := time.Now()
// ... handle request ...
requestDuration.Observe(time.Since(start).Seconds())
```

---

## Related Documentation

- [DOCKER_COMPOSE_SERVICES.md](./DOCKER_COMPOSE_SERVICES.md) — Service configuration details
- [MANAGEMENT_API.md](./MANAGEMENT_API.md) — Management API reference
- [Prometheus Documentation](https://prometheus.io/docs/)
- [Grafana Documentation](https://grafana.com/docs/grafana/)

---

## Support

- **Prometheus Queries:** Use Prometheus UI → Graph tab to test queries
- **Grafana Dashboards:** Browse grafana.com/dashboards for pre-built dashboards
- **Alerts:** Test with `curl http://localhost:9090/api/v1/query?query=up`
