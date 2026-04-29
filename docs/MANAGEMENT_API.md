# SovereignStack Management API

The Management API provides a REST interface to query the status of running models and system information. It's designed to be consumed by monitoring and visibility platforms (like the SovereignStack Visibility Platform).

## Quick Start

### Using Docker Compose

```bash
# Copy environment template
cp .env.example .env

# Start management API service
docker-compose up -d management

# Verify it's running
curl http://localhost:8888/api/health
```

### Using CLI

```bash
# Run management API directly
./sovstack management --port 8888
```

---

## API Endpoints

### GET `/api/health`

Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "ready": true
}
```

---

### GET `/api/models/running`

List all running SovereignStack models.

**Response:**
```json
{
  "version": "1.0",
  "models": [
    {
      "name": "distilbert-base-uncased",
      "container_id": "abc123def456",
      "type": "cpu",
      "status": "running",
      "port": 8000
    },
    {
      "name": "TinyLlama-TinyLlama-1.1B-Chat-v1.0",
      "container_id": "def789ghi012",
      "type": "cpu",
      "status": "running",
      "port": 8001
    }
  ],
  "count": 2
}
```

**Query Parameters:**
- None

**Status Codes:**
- `200 OK` - Successfully returned running models
- `500 Internal Server Error` - Docker query failed

---

## Configuration

### Environment Variables

Set in `.env`:

```bash
# Port for management API (default: 8888)
MANAGEMENT_PORT=8888

# HuggingFace token (if using models from HF)
HF_TOKEN=hf_...

# GPU configuration
CUDA_VISIBLE_DEVICES=0
```

### Docker Compose

Default service configuration:

```yaml
management:
  build:
    context: .
    dockerfile: Dockerfile.management
  ports:
    - "8888:8888"
  volumes:
    - /var/run/docker.sock:/var/run/docker.sock:ro
    - ~/.sovereignstack:/root/.sovereignstack:ro
```

---

## Integration with Visibility Platform

The Visibility Platform (commercial monitoring solution) queries this API to:
1. Get list of running models
2. Correlate with Docker container metrics
3. Collect hardware and GPU metrics
4. Calculate costs and usage

**Configuration in Visibility Platform:**

```bash
# .env
MAIN_STACK_API_URL=http://localhost:8888
```

---

## Troubleshooting

### API not accessible

```bash
# Check if container is running
docker ps | grep sovereignstack-management

# View logs
docker-compose logs management

# Test health endpoint
curl -v http://localhost:8888/api/health
```

### "Docker permission denied" error

```bash
# Add user to docker group (Linux)
sudo usermod -aG docker $USER
newgrp docker

# Or run with sudo
sudo docker-compose up management
```

### Models not showing up

```bash
# Verify models are running
./sovstack status --running

# Check docker sock is mounted correctly
docker-compose exec management ls -l /var/run/docker.sock
```

---

## API Versioning

The API includes a `version` field in responses for backward compatibility:

```json
{
  "version": "1.0",
  "models": [...]
}
```

Future changes will increment the version number. Consumers should:
- Check the version field
- Handle new fields gracefully (ignore unknown fields)
- Support multiple versions if needed

---

## Performance

- Response time: < 100ms (typical)
- Default port: 8888
- Health check interval: 30s
- Startup time: < 5s

---

## Security Considerations

**Current Implementation:**
- No authentication (design trade-off for ease of use)
- Assumes trusted network (run on same machine or private network)
- Read-only operations only (no modification endpoints)

**For Production:**
- Run behind a reverse proxy with authentication
- Restrict network access via firewall
- Use VPN for remote monitoring
- Consider adding API key authentication if exposed publicly

---

## Future Enhancements

- Per-model performance metrics
- Model container resource usage (CPU, memory, network)
- Request latency histograms
- Model uptime and reliability metrics
- Custom webhooks for events
