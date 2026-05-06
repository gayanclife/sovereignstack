# Monitoring Setup Summary

## What Was Fixed/Improved

### 1. ✅ Prometheus Configuration (`monitoring/prometheus.yml`)

**Before:** Minimal config, only scraping vLLM and node-exporter

**After:** Complete monitoring stack with proper labels and intervals

**Changes:**
- ✅ Added **cAdvisor scrape config** to collect Docker container metrics
- ✅ Added proper **evaluation_interval** for alerting
- ✅ Added **external_labels** for cluster/environment identification
- ✅ Added **relabel_configs** for cleaner instance names
- ✅ Added **prometheus self-monitoring** job

**Result:** Prometheus now scrapes:
1. **vLLM** (port 8000) — Model inference metrics
2. **Node-Exporter** (port 9100) — Host system metrics
3. **cAdvisor** (port 8080) — Docker container metrics ⭐ **NEW**
4. **Prometheus** (port 9090) — Self-monitoring

---

### 2. ✅ Docker Compose Configuration (`docker-compose.yml`)

**Before:** Basic monitoring services without persistence

**After:** Production-ready monitoring stack

**Changes:**
- ✅ Added **cAdvisor service** for Docker container metrics ⭐ **NEW**
- ✅ Added **prometheus_data volume** for metric persistence
- ✅ Configured **retention time** (30 days of metrics)
- ✅ Added **restart policies** to all monitoring services
- ✅ Added **container names** for consistency
- ✅ Added **depends_on** for service ordering
- ✅ Improved comments explaining each service

**New Services:**
```yaml
cadvisor:
  image: gcr.io/cadvisor/cadvisor:latest
  ports:
    - "8080:8080"
  volumes:
    - /:/rootfs:ro
    - /var/run:/var/run:rw
    - /sys:/sys:ro
    - /var/lib/docker/:/var/lib/docker:ro
  privileged: true
```

---

## Documentation Created

### 1. **MONITORING.md** (500+ lines)
Complete reference covering:

| Topic | Coverage |
|-------|----------|
| Overview | 4-component monitoring stack |
| Node-Exporter | What it does, metrics, calculations |
| cAdvisor | Docker container metrics, configuration |
| Prometheus | Time-series DB, PromQL, queries |
| Grafana | UI, dashboards, alerts |
| vLLM Monitoring | Model-specific metrics |
| Troubleshooting | Common issues and fixes |
| Best Practices | Production setup tips |

### 2. **MONITORING_QUICKSTART.md** (5-minute setup)
Step-by-step guide:
1. Start services
2. Check Prometheus targets
3. Access Grafana
4. Add data source
5. Import dashboard

### 3. **MONITORING_SUMMARY.md** (this file)
Overview of changes and setup

---

## How Node-Exporter Works

### Data Sources (Read from Linux Kernel)
```
/proc/stat         → CPU metrics
/proc/meminfo      → Memory metrics
/proc/diskstats    → Disk I/O
/proc/net/dev      → Network metrics
/sys/class/        → Hardware sensors
```

### Key Metrics Collected
```promql
node_cpu_seconds_total           # CPU time (user/system/idle)
node_memory_MemTotal_bytes       # Total RAM
node_memory_MemAvailable_bytes   # Available RAM
node_disk_io_reads_completed     # Disk reads
node_disk_io_writes_completed    # Disk writes
node_network_receive_bytes_total # Network RX
node_network_transmit_bytes_total# Network TX
```

### Calculating Common Metrics
```promql
# CPU usage %
100 * (1 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m])))

# Memory usage %
100 * (1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)

# Disk usage %
100 * (1 - node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"})
```

---

## How cAdvisor Works (Docker Metrics)

### Data Sources
```
/var/lib/docker/          → Docker container data
/var/run/docker.sock      → Docker daemon API
/sys/fs/cgroup/           → Linux cgroup metrics
/proc/                    → Process information
```

### Key Metrics per Container
```promql
container_cpu_usage_seconds_total         # CPU time used
container_memory_usage_bytes              # Current memory
container_memory_max_usage_bytes          # Peak memory
container_network_receive_bytes_total     # Network RX
container_network_transmit_bytes_total    # Network TX
container_fs_reads_bytes_total            # Disk reads
container_fs_writes_bytes_total           # Disk writes
```

### Example: Monitor vLLM Container
```promql
# Memory usage (GB)
container_memory_usage_bytes{container_name="vllm"} / 1024 / 1024 / 1024

# CPU usage (%)
rate(container_cpu_usage_seconds_total{container_name="vllm"}[1m]) * 100

# Network RX (Mbps)
rate(container_network_receive_bytes_total{container_name="vllm"}[1m]) * 8 / 1000 / 1000
```

---

## Monitoring Architecture

```
┌───────────────────────────────────────────────────────┐
│                   Grafana (3001)                      │
│              Dashboards & Visualization               │
└─────────────────────────┬─────────────────────────────┘
                          │
        ┌─────────────────┼─────────────────┐
        │                 │                 │
        ↓                 ↓                 ↓
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│ Prometheus   │  │  Alerting    │  │  Querying    │
│   (9090)     │  │   Rules      │  │   UI         │
└──────────────┘  └──────────────┘  └──────────────┘
        │
  ┌─────┼─────┬──────────┐
  ↓     ↓     ↓          ↓
vLLM  Node  cAdvisor  Prom
(8000)(9100)(8080)    (9090)
  │     │      │        │
  ├─────┼──────┴────────┤
  ↓     ↓               ↓
Model Host Container   Self
Inf.  Metrics Metrics Monitor
```

**Data Flow:**
1. Each exporter collects metrics from its data source
2. Prometheus scrapes metrics every 10–15 seconds
3. Grafana queries Prometheus for visualization
4. Alerting rules evaluate conditions

---

## What Each Component Answers

| Question | Component | Metric |
|----------|-----------|--------|
| How much CPU is being used? | Node-Exporter | `node_cpu_seconds_total` |
| How much RAM is available? | Node-Exporter | `node_memory_MemAvailable_bytes` |
| Is disk full? | Node-Exporter | `node_filesystem_avail_bytes` |
| How fast is the network? | Node-Exporter | `node_network_*_bytes_total` |
| Is vLLM responsive? | vLLM | `vllm_requests_total` |
| How fast are responses? | vLLM | `vllm_request_latency_seconds` |
| How many tokens generated? | vLLM | `vllm_tokens_generated_total` |
| Docker container memory? | cAdvisor | `container_memory_usage_bytes` |
| Docker container CPU? | cAdvisor | `container_cpu_usage_seconds_total` |
| Is Prometheus healthy? | Prometheus | `up` metric |

---

## Starting the Monitoring Stack

### One Command
```bash
docker compose up -d prometheus node-exporter cadvisor grafana
```

### Verify Services
```bash
docker compose ps | grep -E 'prometheus|grafana|node-exporter|cadvisor'
```

### Check Prometheus Targets
Open http://localhost:9090/targets

All should be green (up):
- ✅ vllm
- ✅ node
- ✅ cadvisor
- ✅ prometheus

---

## Accessing Dashboards

| Service | URL | Username | Password |
|---------|-----|----------|----------|
| Prometheus | http://localhost:9090 | — | — |
| Grafana | http://localhost:3001 | admin | admin |
| Node-Exporter | http://localhost:9100/metrics | — | — |
| cAdvisor | http://localhost:8080/metrics | — | — |

---

## Performance Notes

### Resource Usage
- **Prometheus:** ~200 MB RAM (15-day retention)
- **Grafana:** ~100 MB RAM
- **Node-Exporter:** ~10 MB RAM
- **cAdvisor:** ~50 MB RAM
- **Total:** ~360 MB RAM

### Storage Usage
- **Prometheus data:** ~500 MB per week (for 4 metrics sources)
- **30-day retention:** ~2 GB disk space

### Scrape Overhead
- **Total scrape rate:** ~1000 samples/minute from all sources
- **CPU impact:** <1% CPU during scrapes

---

## Production Recommendations

### 1. Change Grafana Password
```bash
docker compose exec grafana grafana-cli admin reset-admin-password newpassword
```

### 2. Add Persistent Volumes
```yaml
volumes:
  - ./monitoring/prometheus:/var/lib/prometheus  # Local backup
  - grafana_data:/var/lib/grafana
```

### 3. Configure Alerting
Create alert rules for:
- CPU > 80% for 5 min
- Memory > 85% for 5 min
- Disk > 85%
- vLLM requests latency > 2s P99
- Container down

### 4. Backup Strategy
```bash
# Weekly backup
docker exec sovereignstack-prometheus tar czf /prometheus-backup.tar.gz /prometheus
docker cp sovereignstack-prometheus:/prometheus-backup.tar.gz ./backups/
```

---

## Troubleshooting Checklist

### Prometheus targets showing RED
- [ ] Services started? `docker compose ps`
- [ ] Ports accessible? `curl http://localhost:9100/metrics`
- [ ] Logs show errors? `docker compose logs prometheus`
- [ ] Restart services: `docker compose restart`

### Grafana can't reach Prometheus
- [ ] Data source URL is `http://prometheus:9090` (not localhost)
- [ ] Prometheus container is running
- [ ] Restart Grafana: `docker compose restart grafana`

### cAdvisor not showing containers
- [ ] Privileged mode enabled: `privileged: true`
- [ ] Docker socket accessible: `/var/run/docker.sock`
- [ ] Restart cAdvisor: `docker compose restart cadvisor`

### No metric data in Grafana
- [ ] Wait 2–3 minutes for data collection
- [ ] Scrape targets are UP in Prometheus
- [ ] Query Prometheus directly: http://localhost:9090

---

## Further Reading

- **Full guide:** [MONITORING.md](./MONITORING.md)
- **Quick start:** [MONITORING_QUICKSTART.md](./MONITORING_QUICKSTART.md)
- **Services reference:** [DOCKER_COMPOSE_SERVICES.md](./DOCKER_COMPOSE_SERVICES.md)
- **Prometheus docs:** https://prometheus.io/docs/
- **Grafana docs:** https://grafana.com/docs/grafana/
- **cAdvisor project:** https://github.com/google/cadvisor

---

## Summary

✅ **Prometheus Configuration:** Updated to scrape Docker container metrics via cAdvisor  
✅ **Docker Compose:** Updated with cAdvisor, persistent volumes, and restart policies  
✅ **Node-Exporter:** Fully documented (host system metrics)  
✅ **cAdvisor:** Fully documented (Docker container metrics)  
✅ **Documentation:** 3 comprehensive guides created  
✅ **Ready to Use:** Can start monitoring in < 5 minutes  

**Next Steps:**
1. Run: `docker compose up -d prometheus node-exporter cadvisor grafana`
2. Open: http://localhost:3001 (Grafana)
3. Follow: [MONITORING_QUICKSTART.md](./MONITORING_QUICKSTART.md)
