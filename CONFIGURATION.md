# SovereignStack Configuration Guide

How to configure SovereignStack for your environment.

## Quick Start

```bash
# View current configuration
sovstack config list

# Set where models are stored
sovstack config set cache-dir /mnt/models

# Store Hugging Face token (encrypted)
sovstack config set hf-token hf_xxx

# Set log directory
sovstack config set log-dir /var/log/sovereignstack
```

## Configuration Options

### cache-dir

**Purpose:** Directory where model files are downloaded and cached.

**Default:**
- `~/.sovereignstack/models`

**Considerations:**
- Models can be 5-40+ GB each
- Use a disk with sufficient space
- Should be on a fast storage device for optimal performance

**Examples:**

```bash
# Use a large NVMe drive
sovstack config set cache-dir /mnt/nvme/models

# Use network storage (NFS)
sovstack config set cache-dir /mnt/nfs/sovereignstack-models

# Use Docker volume
sovstack config set cache-dir /var/lib/sovereignstack/models

# Temporary location for testing
sovstack config set cache-dir /tmp/models
```

**Verification:**

```bash
# Check current setting
sovstack config get cache-dir

# Check available space
du -sh $(sovstack config get cache-dir)

# Verify writable
touch "$(sovstack config get cache-dir)/.test" && rm "$(sovstack config get cache-dir)/.test"
```

---

### log-dir

**Purpose:** Directory where audit logs are written.

**Default Behavior (OS-specific):**
- **Linux:** Tries `/var/log/sovereignstack`, falls back to `~/.sovereignstack/logs` if not writable
- **macOS:** Tries `~/Library/Logs/sovereignstack`, falls back to `~/.sovereignstack/logs`
- **Windows:** Tries `%APPDATA%/sovereignstack/logs`, falls back to `~/.sovereignstack/logs`

**Examples:**

```bash
# Standard Linux location (system logs)
sovstack config set log-dir /var/log/sovereignstack

# Custom location for enterprise logging
sovstack config set log-dir /opt/sovereignstack/logs

# Separate audit directory
sovstack config set log-dir /var/audit/sovereignstack

# Temporary location (for testing)
sovstack config set log-dir /tmp/sovstack-logs
```

**Permissions:**

The log directory is automatically created with:
- Owner: Current user
- Permissions: 0755 (readable by all, writable by owner)
- Log file: 0600 (readable/writable by owner only)

**Monitoring Logs:**

```bash
# View recent logs
tail -f $(sovstack config get log-dir)/audit.log

# Filter by action
grep "model_download" $(sovstack config get log-dir)/audit.log | jq .

# Monitor with jq for pretty printing
tail -f $(sovstack config get log-dir)/audit.log | jq .

# Count downloads
grep -c "model_download" $(sovstack config get log-dir)/audit.log
```

**Log Retention:**

Logs grow over time. Implement rotation in your environment:

```bash
# With logrotate (Linux)
cat > /etc/logrotate.d/sovereignstack <<EOF
$(sovstack config get log-dir)/audit.log {
    daily
    missingok
    rotate 30
    compress
    delaycompress
}
EOF
```

---

### hf-token

**Purpose:** Hugging Face API token for downloading gated models.

**Default:** Not configured

**Security:**
- Stored encrypted (AES-256-GCM)
- Encryption key derived from home directory (per-user)
- Config file has restrictive permissions (0600)
- Never logged in plain text

**Getting a Token:**

1. Visit https://huggingface.co/settings/tokens
2. Create a new token with "read" permissions
3. Configure it:

```bash
sovstack config set hf-token hf_xxx
```

**For Gated Models:**

Llama 2 and other gated models require accepting the license:

1. Visit model page (e.g., https://huggingface.co/meta-llama/Llama-2-7b-hf)
2. Click "I have read and agree to the License Agreement"
3. Configure token: `sovstack config set hf-token hf_xxx`
4. Download: `sovstack pull meta-llama/Llama-2-7b-hf`

**Verification:**

```bash
# Check if token is configured
sovstack config get hf-token
# Output: (set, encrypted)

# View encrypted token in config file
cat ~/.sovereignstack/config.json | jq .hf_token
# Output: "2aB9YoGRM5ufw0/Cqo0cfIKyE4CDboEBVu1gEfGORPOxwzeXTHgcJdZd0z"
```

---

## Configuration File

Location: `~/.sovereignstack/config.json`

**Example:**
```json
{
  "cache_dir": "/mnt/models",
  "log_dir": "/var/log/sovereignstack",
  "hf_token": "2aB9YoGRM5ufw0/Cqo0cfIKyE4CDboEBVu1gEfGORPOxwzeXTHgcJdZd0z"
}
```

**File Permissions:**
```
-rw------- 1 user user config.json
```

**Notes:**
- Only readable/writable by the owner
- Auto-created on first config command
- Can be manually edited if needed

---

## Environment Variable Overrides

Environment variables take precedence over config file:

```bash
# Override cache directory
SOVEREIGNSTACK_CACHE_DIR=/tmp/models sovstack pull gpt2

# Override log directory
SOVEREIGNSTACK_LOG_DIR=/tmp/logs sovstack status

# Override HF token
HF_TOKEN=hf_different_token sovstack pull meta-llama/Llama-2-7b-hf

# Combine multiple overrides
export SOVEREIGNSTACK_CACHE_DIR=/mnt/models
export SOVEREIGNSTACK_LOG_DIR=/var/log/sovereignstack
export HF_TOKEN=hf_xxx

sovstack pull mistralai/Mistral-7B-v0.1
```

**Use Cases:**
- CI/CD pipelines (temporary storage)
- Multiple users on same system
- Docker containers
- Development/testing environments

---

## System-Wide Configuration

For multi-user systems, set defaults in `/etc/sovereignstack/config`:

```bash
sudo mkdir -p /etc/sovereignstack

cat > /etc/sovereignstack/config <<EOF
# SovereignStack system configuration
# These can be overridden by individual users

# Shared cache (all users)
SOVEREIGNSTACK_CACHE_DIR=/var/cache/sovereignstack

# Shared logs
SOVEREIGNSTACK_LOG_DIR=/var/log/sovereignstack
EOF
```

Then in shell profile:
```bash
source /etc/sovereignstack/config
```

---

## Docker Configuration

When running in Docker:

```dockerfile
FROM ubuntu:22.04

# Set cache directory (in container)
ENV SOVEREIGNSTACK_CACHE_DIR=/models
ENV SOVEREIGNSTACK_LOG_DIR=/logs

# Create directories
RUN mkdir -p /models /logs

WORKDIR /app
COPY . .

# Build
RUN go build -o sovstack .

# Mount as volumes
ENTRYPOINT ["./sovstack"]
```

**Run with volumes:**
```bash
docker run -it \
  -v /host/models:/models \
  -v /host/logs:/logs \
  sovereignstack:latest \
  pull gpt2
```

---

## Kubernetes ConfigMap

For Kubernetes deployments:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: sovereignstack-config
data:
  cache-dir: "/models"
  log-dir: "/var/log/sovereignstack"
---
apiVersion: v1
kind: Secret
metadata:
  name: sovereignstack-secrets
type: Opaque
stringData:
  hf-token: "hf_YOUR_TOKEN"
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: sovereignstack-models
spec:
  capacity:
    storage: 100Gi
  accessModes:
    - ReadWriteOnce
  hostPath:
    path: /mnt/models
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: sovereignstack-models-pvc
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 100Gi
```

---

## Performance Optimization

### Disk Selection

**For model caching:**
- Fast NVMe drives (2000+ MB/s read)
- Local storage (avoid network drives for model files)
- Sufficient space (models: 5-40GB+, growth: ~100GB/month)

**Example setup:**
```bash
# Use fast disk for cache
sovstack config set cache-dir /mnt/nvme/sovereignstack

# Use slower disk for logs (logging is I/O-light)
sovstack config set log-dir /mnt/sata/logs/sovereignstack
```

### Network Configuration

For remote storage:

```bash
# Mount NFS or SMB
sudo mount -t nfs server:/export/models /mnt/models

# Configure
sovstack config set cache-dir /mnt/models

# Verify performance
time sovstack pull gpt2
```

---

## Troubleshooting

### Permission Denied

```bash
# Check if config directory exists
ls -la ~/.sovereignstack/

# Fix permissions
chmod 700 ~/.sovereignstack
chmod 600 ~/.sovereignstack/config.json

# Try again
sovstack config list
```

### Cannot Write to Log Directory

```bash
# Check log directory is writable
touch /var/log/sovereignstack/test
# If fails, use fallback:
sovstack config set log-dir ~/.sovereignstack/logs
```

### Configuration Not Applied

```bash
# Check current config
sovstack config list

# Environment variables override config
unset SOVEREIGNSTACK_CACHE_DIR

# Try again
sovstack config get cache-dir
```

### Token Not Working

```bash
# Verify token is stored
sovstack config get hf-token
# Should show: (set, encrypted)

# Test with explicit token
HF_TOKEN=hf_xxx sovstack pull meta-llama/Llama-2-7b-hf

# If works: your stored token might be wrong
# If fails: token might be expired
```

---

## Migration

### Moving Cache to New Disk

```bash
# 1. Set new location
sovstack config set cache-dir /mnt/large-disk/models

# 2. Move existing models
mv ~/.sovereignstack/models/* /mnt/large-disk/models/

# 3. Verify
sovstack status

# 4. Cleanup
rmdir ~/.sovereignstack/models
```

### Consolidating Logs

```bash
# 1. Set new log directory
sovstack config set log-dir /var/log/sovereignstack

# 2. Move existing logs
sudo mv ~/.sovereignstack/logs/* /var/log/sovereignstack/

# 3. Fix permissions
sudo chown $USER:$USER /var/log/sovereignstack/*
sudo chmod 600 /var/log/sovereignstack/audit.log

# 4. Verify
tail -f /var/log/sovereignstack/audit.log
```

---

## Compliance and Audit

### Log Format

Audit logs are JSON for easy parsing:

```json
{
  "timestamp": "2026-04-27T20:05:05+10:00",
  "action": "model_download",
  "user": "gayangunapala",
  "details": "model=gpt2 2 files",
  "status": "success"
}
```

### Parsing Logs

```bash
# List all actions
jq '.action' /var/log/sovereignstack/audit.log | sort | uniq -c

# Filter by date
jq 'select(.timestamp > "2026-04-27")' /var/log/sovereignstack/audit.log

# Find failures
jq 'select(.status == "failed")' /var/log/sovereignstack/audit.log

# Export to CSV
jq -r '[.timestamp, .action, .user, .status] | @csv' /var/log/sovereignstack/audit.log
```

### Log Retention Policy

Implement retention based on compliance needs:

```bash
# Keep 90 days of logs
find /var/log/sovereignstack -name "audit.log*" -mtime +90 -delete

# Or use logrotate (recommended)
sudo cat > /etc/logrotate.d/sovereignstack <<EOF
/var/log/sovereignstack/audit.log {
    daily
    rotate 90
    compress
    delaycompress
    notifempty
    create 0600 $USER $USER
}
EOF
```
