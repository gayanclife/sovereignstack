# Monitoring Quick Start (5 Minutes)

Get the monitoring stack (Prometheus + Grafana + Node-Exporter + cAdvisor) running in 5 minutes.

---

## Step 1: Start the Monitoring Stack

```bash
cd /home/gayangunapala/Projects/sstack/sovereignstack
docker compose up -d prometheus node-exporter cadvisor grafana
```

Verify all services are running:
```bash
docker compose ps
```

Expected output:
```
sovereignstack-prometheus         prom/prometheus:latest      Up (healthy)
sovereignstack-grafana            grafana/grafana:latest      Up
sovereignstack-node-exporter      prom/node-exporter:latest   Up
sovereignstack-cadvisor           gcr.io/cadvisor/cadvisor    Up
```

---

## Step 2: Check Prometheus Targets

Open http://localhost:9090

1. Click **Status** → **Targets**
2. You should see 4 targets:
   - ✅ **vllm** (up)
   - ✅ **node** (up)
   - ✅ **cadvisor** (up)
   - ✅ **prometheus** (up)

If any show red (down), check the logs:
```bash
docker compose logs prometheus
docker compose logs node-exporter
docker compose logs cadvisor
```

---

## Step 3: Access Grafana

Open http://localhost:3001

**Login:**
- Username: `admin`
- Password: `admin`

(You'll be prompted to change the password on first login)

---

## Step 4: Add Prometheus Data Source

1. Click **Configuration** (gear icon) → **Data Sources**
2. Click **Add data source**
3. Select **Prometheus**
4. **URL:** `http://prometheus:9090`
5. Click **Save & Test**

You should see: ✅ "Data source is working"

---

## Step 5: Import Node-Exporter Dashboard

1. Click **+** → **Import**
2. **Import via grafana.com:** Enter `1860`
3. Click **Load**
4. **Data source:** Select Prometheus
5. Click **Import**

You should now see real-time system metrics!

---

## What You're Monitoring

### Node-Exporter (System Metrics)

What: CPU, RAM, disk, network usage on the **host machine**

Where: http://localhost:9100/metrics

Useful metrics:
```promql
# CPU usage percentage
100 * (1 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m])))

# Memory usage percentage
100 * (1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)

# Disk usage percentage
100 * (1 - node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"})
```

### cAdvisor (Container Metrics)

What: CPU, memory, network for **all Docker containers**

Where: http://localhost:8080/metrics

Useful metrics:
```promql
# vLLM container memory (GB)
container_memory_usage_bytes{container_name="vllm"} / 1024 / 1024 / 1024

# vLLM container CPU (%)
rate(container_cpu_usage_seconds_total{container_name="vllm"}[1m]) * 100

# All containers
container_memory_usage_bytes{container_name!=""}
```

### vLLM (Model Metrics)

What: Request latency, throughput, token rates from the **LLM engine**

Where: http://localhost:8000/metrics

Useful metrics:
```promql
# Request throughput (req/s)
rate(vllm_requests_total[1m])

# P99 latency (ms)
histogram_quantile(0.99, vllm_request_latency_seconds) * 1000

# Token generation rate (tok/s)
rate(vllm_tokens_generated_total[1m])
```

---

## Common Queries in Prometheus

Open http://localhost:9090 and try these queries in the **Graph** tab:

| What | Query | Example |
|------|-------|---------|
| CPU usage % | `100 * (1 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m])))` | Shows: 42.5% |
| Memory free (GB) | `node_memory_MemAvailable_bytes / 1024 / 1024 / 1024` | Shows: 8.2 GB |
| Disk used % | `100 * (1 - node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"})` | Shows: 45% |
| vLLM requests/sec | `rate(vllm_requests_total[1m])` | Shows: 12.3 req/s |
| vLLM tokens/sec | `rate(vllm_tokens_generated_total[1m])` | Shows: 450 tok/s |
| vLLM P99 latency (ms) | `histogram_quantile(0.99, vllm_request_latency_seconds) * 1000` | Shows: 850 ms |
| Container count | `count(container_memory_usage_bytes{container_name!=""})` | Shows: 5 containers |

---

## Create Your First Dashboard

1. Click **+** → **Dashboard** → **Add Panel**
2. In the Query section:
   - Data Source: Prometheus
   - Metric: `node_memory_MemAvailable_bytes / 1024 / 1024 / 1024`
3. Click **Apply**
4. In the top-right, click **Save** and give it a name

---

## Port Reference

| Service | Port | URL | Purpose |
|---------|------|-----|---------|
| Prometheus | 9090 | http://localhost:9090 | Query metrics, see targets |
| Grafana | 3001 | http://localhost:3001 | Dashboards & visualizations |
| Node-Exporter | 9100 | http://localhost:9100/metrics | System metrics |
| cAdvisor | 8080 | http://localhost:8080 | Container metrics |
| vLLM | 8000 | http://localhost:8000/metrics | Model metrics |

---

## Troubleshooting

### Q: Prometheus shows "No targets up"
**A:** Wait 30 seconds for services to start. If still down, check if services are running:
```bash
docker compose logs prometheus node-exporter cadvisor
```

### Q: Grafana says "Bad Gateway" when testing data source
**A:** Restart Grafana:
```bash
docker compose restart grafana
```

### Q: Dashboard shows no data
**A:** Wait 2–3 minutes for Prometheus to collect data from targets.

### Q: cAdvisor shows no container metrics
**A:** Restart cAdvisor:
```bash
docker compose restart cadvisor
```

---

## Next Steps

- **Read full guide:** [MONITORING.md](./MONITORING.md)
- **Understand Node-Exporter:** See [MONITORING.md - Node-Exporter](./MONITORING.md#understanding-node-exporter) section
- **Understand cAdvisor:** See [MONITORING.md - cAdvisor](./MONITORING.md#understanding-cadvisor) section
- **Create alerts:** See [MONITORING.md - Creating Alerts](./MONITORING.md#creating-alerts)
- **Custom metrics:** See [MONITORING.md - Custom Metrics](./MONITORING.md#advanced-custom-metrics)

---

## One-Liner: Full Setup

```bash
cd /home/gayangunapala/Projects/sstack/sovereignstack && \
docker compose up -d prometheus node-exporter cadvisor grafana && \
sleep 5 && \
echo "✅ Prometheus: http://localhost:9090" && \
echo "✅ Grafana: http://localhost:3001 (admin/admin)" && \
echo "✅ Node-Exporter: http://localhost:9100/metrics" && \
echo "✅ cAdvisor: http://localhost:8080/metrics"
```

Done! 🎉
