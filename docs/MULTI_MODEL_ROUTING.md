# Multi-Model Routing Gateway (Phase 3)

## Overview

Phase 3 adds **dynamic model discovery and per-model backend routing**. The gateway now:
- Automatically discovers deployed models from the management service
- Routes requests to model-specific backend ports
- Strips model prefixes from request paths before forwarding
- Periodically refreshes the model registry (every 30 seconds)

This enables running multiple model instances on different ports and routing transparently based on the requested model.

---

## How It Works

### Model Discovery Flow

```
┌─────────────────────────────────────────────────────────────┐
│  Gateway Startup (Phase 3)                                  │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ↓
┌─────────────────────────────────────────────────────────────┐
│  Create ModelRouter                                          │
│  url: management-url (default: http://localhost:8888)        │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ↓
         ┌─────────────────────────────┐
         │ Start Discovery Process     │
         │ - Poll every 30 seconds     │
         │ - Fetch /api/v1/models/running │
         └────────────┬────────────────┘
                      │
                      ↓
        ┌──────────────────────────────┐
        │ GET http://localhost:8888    │
        │ /api/v1/models/running          │
        └────────────┬─────────────────┘
                     │
                     ↓
       ┌─────────────────────────────────┐
       │ Management Service Response     │
       │ {                               │
       │   "models": [                   │
       │     {                           │
       │       "model_name": "mistral-7b",
       │       "port": 8000,             │
       │       "status": "running",      │
       │       "type": "gpu"             │
       │     },                          │
       │     {                           │
       │       "model_name": "phi-3",    │
       │       "port": 8001,             │
       │       "status": "running",      │
       │       "type": "cpu"             │
       │     }                           │
       │   ]                             │
       │ }                               │
       └────────────┬────────────────────┘
                    │
                    ↓
      ┌──────────────────────────────────┐
      │ Update Registry                  │
      │ mistral-7b → localhost:8000      │
      │ phi-3 → localhost:8001           │
      └─────────────────────────────────┘
```

### Request Routing Flow

```
Request from User Alice
POST http://localhost:8001/models/mistral-7b/v1/chat/completions

       ↓
  1. Authenticate ✓
       ↓
  2. Check Access Control ✓
       ↓
  3. Check Quota ✓
       ↓
  4. Check Rate Limit ✓
       ↓
  5. MODEL ROUTING (NEW - Phase 3)
     Extract model name from path:
       /models/mistral-7b/v1/chat/completions
         ↓
       Model: "mistral-7b"
         ↓
     Look up in registry:
       modelRouter.GetBackend("mistral-7b")
         ↓
     Found: ModelBackend{
       Name: "mistral-7b",
       URL: "http://localhost:8000",
       Port: 8000,
       Type: "gpu"
     }
         ↓
     Route to http://localhost:8000
     Strip /models/mistral-7b prefix:
       /models/mistral-7b/v1/chat/completions
         ↓
       /v1/chat/completions
         ↓
  6. Forward to Backend
     POST http://localhost:8000/v1/chat/completions
       ↓
  7. Return Response 200 OK
```

---

## Setup: Deploy Multiple Models

### Prerequisites

Ensure you have multiple models running on different ports:

```bash
# Terminal 1: Deploy mistral-7b on port 8000
docker run -p 8000:8000 vllm:latest --model mistral-7b

# Terminal 2: Deploy phi-3 on port 8001
docker run -p 8001:8000 vllm:latest --model phi-3

# Terminal 3: Deploy llama-3 on port 8002
docker run -p 8002:8000 vllm:latest --model llama-3-8b
```

Or using SovereignStack management:

```bash
sovstack deploy mistral-7b --port 8000
sovstack deploy phi-3 --port 8001
sovstack deploy llama-3-8b --port 8002
```

### Start Gateway with Model Router

```bash
# Default: polls http://localhost:8888/api/v1/models/running every 30s
sovstack gateway \
  --backend http://localhost:8000 \
  --management-url http://localhost:8888 \
  --keys ~/.sovereignstack/keys.json

# Output:
# Model Router: Enabled (Phase 3, polling http://localhost:8888 every 30s)
# Registered Models: 3
```

### Optional: Custom Management URL

```bash
# If management service runs on different host
sovstack gateway \
  --backend http://localhost:8000 \
  --management-url http://remote-management.example.com:9000 \
  --keys ~/.sovereignstack/keys.json
```

---

## Testing Phase 3

### Test Case 1: Model-Based Routing

```bash
# Setup: 3 models running on different ports (see Prerequisites)

# Start gateway
sovstack gateway \
  --management-url http://localhost:8888 \
  --keys ~/.sovereignstack/keys.json

# Test: Route to mistral-7b (port 8000)
curl -H "X-API-Key: sk_alice_..." \
     -d '{"model":"mistral-7b","messages":[...]}' \
     http://localhost:8001/models/mistral-7b/v1/chat/completions

# Expected: Request forwarded to http://localhost:8000/v1/chat/completions
# Response: 200 OK from mistral-7b backend

# Test: Route to phi-3 (port 8001)
curl -H "X-API-Key: sk_alice_..." \
     -d '{"model":"phi-3","messages":[...]}' \
     http://localhost:8001/models/phi-3/v1/chat/completions

# Expected: Request forwarded to http://localhost:8001/v1/chat/completions
# Response: 200 OK from phi-3 backend
```

### Test Case 2: Model Not Found

```bash
# Request a model that isn't registered
curl -H "X-API-Key: sk_alice_..." \
     -d '{"model":"unknown-model","messages":[...]}' \
     http://localhost:8001/models/unknown-model/v1/chat/completions

# Expected: Falls back to default backend (--backend flag)
# If default backend doesn't have the model: 404 or 500 from backend
```

### Test Case 3: Gateway Fallback (No Route Prefix)

```bash
# Old-style request without /models/ prefix
curl -H "X-API-Key: sk_alice_..." \
     -d '{"model":"mistral-7b","messages":[...]}' \
     http://localhost:8001/v1/chat/completions

# Expected: Routes to default backend (--backend flag)
# This maintains backward compatibility with Phase 1/2
```

### Test Case 4: Model Discovery Refresh

```bash
# Start gateway
sovstack gateway --management-url http://localhost:8888

# In another terminal, deploy a new model
sovstack deploy new-model --port 8003

# Wait up to 30 seconds (discovery refresh interval)

# Check models are updated
# New requests to /models/new-model/... will be routed to :8003
```

---

## Configuration

### Request Path Formats Supported

**Phase 3 Multi-Model Format:**
```
/models/{model-name}/v1/chat/completions
/models/{model-name}/v1/completions
/models/{model-name}/v1/embeddings
/models/{model-name}/v1/models
```

**Legacy Format (Backward Compatible):**
```
/v1/chat/completions
/v1/completions
/v1/embeddings
```

### Gateway Flags (Phase 3)

```bash
--management-url string
    Management service URL for model discovery
    (default "http://localhost:8888")
    
    Used to poll: GET /api/v1/models/running
    Every 30 seconds
```

### Environment Variables

No new environment variables needed. Model discovery URL is configurable via `--management-url` flag.

---

## Implementation Details

### Code Files

**`core/gateway/router.go` (180 lines)**
- `ModelBackend` struct — model name, URL, port, type, status
- `ModelRouter` struct — maintains registry and discovery process
- Methods:
  - `NewModelRouter(managementURL)` — create router
  - `StartDiscovery()` — begin periodic polling
  - `Stop()` — gracefully stop discovery
  - `GetBackend(modelName)` — O(1) lookup
  - `ListModels()` — get all registered models
  - `GetModelCount()` — count registered models
  - `refresh()` — poll management service

**`core/gateway/proxy.go` (modified)**
- Updated `director()` function to check model router
- Extract model name from path
- Route to model-specific backend if found
- Strip `/models/{model-name}` prefix before forwarding
- Fall back to default backend if model not found

**`cmd/gateway.go` (modified)**
- Add `--management-url` flag
- Create and start `ModelRouter`
- Wire into `GatewayConfig`
- Print model count on startup
- Stop router on shutdown

### Request Flow in Code

```go
// core/gateway/proxy.go :: director()

// If model router enabled, check for model-based routing
if gw.modelRouter != nil {
    modelName := extractModelNameFromPath(req.URL.Path)
    if modelName != "" {
        if backend, exists := gw.modelRouter.GetBackend(modelName); exists {
            // Route to model-specific backend
            targetURL = backend.URL
            // Strip the /models/{model-name} prefix
            req.URL.Path = stripModelPrefixFromPath(req.URL.Path, modelName)
        }
    }
}

// Continue with normal request forwarding
req.URL.Scheme = targetURL.Scheme
req.URL.Host = targetURL.Host
// ...
```

### Helper Functions

**`extractModelNameFromPath(path string) string`**
- Parses paths like `/models/{model-name}/v1/...`
- Returns model name (e.g., "mistral-7b")
- Returns empty string if not in model-routed format

**`stripModelPrefixFromPath(path, modelName string) string`**
- Removes `/models/{model-name}` prefix
- `/models/mistral-7b/v1/chat/completions` → `/v1/chat/completions`
- Used before forwarding to backend

---

## Performance Characteristics

### Discovery Polling
- **Frequency:** Every 30 seconds (hardcoded)
- **Method:** HTTP GET to management service
- **Timeout:** 5 seconds per request
- **Impact:** Minimal (background goroutine, non-blocking)

### Model Routing
- **Lookup Time:** O(1) hash map — <100ns
- **Path Parsing:** <1µs per request
- **Per-Request Overhead:** ~1-2µs

### Memory Usage
- Per-model: ~300 bytes (ModelBackend struct)
- Example: 100 models = ~30KB
- Negligible compared to gateway memory footprint

### Concurrency
- Thread-safe via RWMutex
- Multiple requests can route simultaneously
- Registry updates don't block lookups

---

## Common Patterns

### Pattern 1: Model Type Separation

```
GPU Models (expensive):
  - mistral-7b → :8000 (GPU A)
  - llama-3-70b → :8001 (GPU B)

CPU Models (cheap):
  - phi-3 → :8002 (CPU)
  - qwen-7b → :8003 (CPU)

Routing in action:
  curl /models/mistral-7b/... → 8000 (GPU, slower but powerful)
  curl /models/phi-3/... → 8002 (CPU, faster for small tasks)
```

### Pattern 2: Model Sharding

```
Large Model (multiple instances for load balancing):
  - mistral-7b#1 → :8000
  - mistral-7b#2 → :8001
  - mistral-7b#3 → :8002

Note: Current implementation routes by exact name match.
Load balancing across instances would require Phase 4+ enhancement.
```

### Pattern 3: Gradual Model Rollout

```
Canary deployment:
  - Old version (stable) → :8000
  - New version (canary) → :8001

All requests to /models/mistral-7b/ → 8000 (stable)
Test requests to /models/mistral-7b-canary/ → 8001 (new)

After validation, remove old, rename new.
```

---

## Troubleshooting

### Models Not Discovered

**Symptom:** "Registered Models: 0" on startup

**Diagnosis:**
```bash
# Check management service is running
curl http://localhost:8888/api/v1/models/running
# Should return JSON list of models

# Check management URL is correct
sovstack gateway --management-url http://management-host:port
```

**Solution:**
1. Ensure management service is running
2. Verify `--management-url` points to correct host/port
3. Check firewall allows gateway → management communication

### Request Routes to Default Backend Instead of Model

**Symptom:** Request to `/models/mistral-7b/...` routes to `--backend` instead of model-specific port

**Diagnosis:**
1. Model discovery hasn't run yet (wait up to 30 seconds)
2. Model name doesn't match exactly (case-sensitive)
3. Model status not "running" in management service

**Solution:**
```bash
# Check if model is running
curl http://localhost:8888/api/v1/models/running | jq '.models[]'

# Verify exact model name and status
# Ensure model name matches request path exactly
```

### Model Discovery Polling Fails

**Symptom:** "Model Router: Enabled (Phase 3...)" but model list stays empty

**Diagnosis:**
1. Management service unreachable
2. Network latency causing timeout (5-second limit)
3. JSON response format mismatch

**Solution:**
```bash
# Test connectivity
curl -v http://localhost:8888/api/v1/models/running
# Check response is valid JSON with "models" array

# Check gateway logs for errors
# Gateway silently continues if refresh fails (keeps old registry)
```

### Path Stripping Not Working

**Symptom:** Backend receives `/models/mistral-7b/v1/chat/completions` instead of `/v1/chat/completions`

**Diagnosis:**
1. Model router disabled (happens if no `--management-url` provided, but router still created)
2. Model name not exactly matching path

**Solution:**
```bash
# Verify model name in path matches registered model exactly
# /models/{exact-name-here}/v1/...

# Check model name has no special characters
# Whitespace, hyphens, numbers are OK
# Underscores, dots usually OK too
```

---

## Gateway Request Pipeline (Phase 1-3)

Complete request flow with all phases integrated:

```
1. Extract API Key
2. ✓ Authenticate (Phase 1)
3. Extract Model Name
4. ✓ Check Access Control (Phase 2)
5. ✓ Check Token Quota (Phase 2b)
6. ✓ ROUTE TO MODEL (Phase 3) ← NEW
   - Look up model in registry
   - Route to model-specific backend
   - Strip /models/{name} prefix
7. Check Rate Limit
8. Forward to Backend
9. Log Request
10. Return Response
11. Log Response
```

---

## Next Phase: Phase 4 - Gateway Prometheus Metrics

Phase 4 will add:
1. Prometheus metrics endpoint (`GET /metrics`)
2. Per-model request counts and latencies
3. Per-user metrics
4. Error rate tracking
5. Model discovery metrics (models, last refresh, etc.)

**Dependency:** Phase 3 complete ✓

---

## Migration from Phase 2

Phase 2 had single backend routing. Phase 3 adds dynamic multi-model routing:

### Before (Phase 2)
```bash
# Only one backend possible
sovstack gateway --backend http://localhost:8000

# All requests go to same backend
curl /v1/chat/completions  # → localhost:8000
curl /v1/embeddings         # → localhost:8000
```

### After (Phase 3)
```bash
# Enable multi-model routing
sovstack gateway \
  --backend http://localhost:8000 \     # fallback only
  --management-url http://localhost:8888 # model discovery

# Route to different backends by model
curl /models/mistral-7b/v1/chat/completions  # → localhost:8000
curl /models/phi-3/v1/chat/completions       # → localhost:8001
curl /models/llama-3/v1/embeddings           # → localhost:8002

# Old format still works (backward compatible)
curl /v1/chat/completions  # → localhost:8000 (fallback)
```

---

## Related Documentation

- [API Key Management](./KEYS_MANAGEMENT.md) — User creation
- [Gateway Access Control](./GATEWAY_ACCESS_CONTROL.md) — Model permissions (Phase 2)
- [Token Quotas](./TOKEN_QUOTAS.md) — Per-user token limits (Phase 2b)
- [Architecture](./ARCHITECTURE_AUTH.md) — Overall design patterns
