# Phase 9: vLLM Metrics Endpoint in Management API

## Overview

Phase 9 adds a `/api/v1/models/{name}/metrics` endpoint to the management service
that proxies the Prometheus metrics endpoint of a specific vLLM model
container. This allows the platform backend to scrape inference-level metrics
(token throughput, KV cache usage, prefill/decode times, etc.) per-model
without having to discover container ports itself.

## Endpoint

```
GET /api/v1/models/{name}/metrics
```

**Authentication:** None (read-only operation, internal network only)

**Path Parameters:**
- `name` — Model name as registered in the running models list (e.g., `mistral-7b`)

**Response:**
- **200 OK** — Prometheus text format metrics (passed through from vLLM)
- **404 Not Found** — Model is not currently running
- **405 Method Not Allowed** — Only `GET` is supported
- **502 Bad Gateway** — vLLM container is running but `/metrics` is unreachable
- **503 Service Unavailable** — Model has no exposed port
- **500 Internal Server Error** — Failed to query Docker for running models

**Content-Type:** `text/plain; version=0.0.4` (forwarded from vLLM)

---

## Example

```bash
# Discover running models
curl http://localhost:8888/api/v1/models/running

# Fetch vLLM metrics for a specific model
curl http://localhost:8888/api/v1/models/mistral-7b/metrics
```

**Sample Response:**
```
# HELP vllm:num_requests_running Number of requests currently running on GPU.
# TYPE vllm:num_requests_running gauge
vllm:num_requests_running{model_name="mistral-7b"} 2.0

# HELP vllm:num_requests_waiting Number of requests waiting to be processed.
# TYPE vllm:num_requests_waiting gauge
vllm:num_requests_waiting{model_name="mistral-7b"} 0.0

# HELP vllm:gpu_cache_usage_perc GPU KV-cache usage. 1 means 100 percent usage.
# TYPE vllm:gpu_cache_usage_perc gauge
vllm:gpu_cache_usage_perc{model_name="mistral-7b"} 0.42
```

---

## Implementation

**File:** `cmd/management.go`

The handler:
1. Parses `{name}` from the URL path
2. Queries `docker.GetRunningModels()` to find the container's exposed port
3. Issues an HTTP GET to `http://localhost:{port}/metrics` with a 5-second timeout
4. Streams the response body back to the caller, preserving status code and
   `Content-Type` header

```go
// Shared HTTP client with short timeout — vLLM /metrics is local
var vllmMetricsClient = &http.Client{Timeout: 5 * time.Second}

func handleModelMetrics(w http.ResponseWriter, r *http.Request, modelName string) {
    // Look up running model by name → resolve to port
    // Proxy GET to http://localhost:{port}/metrics
}
```

The dispatcher `handleModelEndpoints` is registered against `/api/v1/models/`
(trailing slash) so it does not collide with the existing
`/api/v1/models/running` exact-match route.

---

## Integration with Platform Backend

Platform backend's `internal/collector/inference_collector.go` can now scrape
per-model metrics via management service rather than connecting directly to
each container:

```go
// Before: required knowing each model's port
http.Get("http://localhost:8000/metrics") // mistral-7b
http.Get("http://localhost:8002/metrics") // phi-3

// After: single management endpoint, model name only
http.Get("http://localhost:8888/api/v1/models/mistral-7b/metrics")
http.Get("http://localhost:8888/api/v1/models/phi-3/metrics")
```

This decouples the collector from container port discovery and centralizes
the model-name → port mapping in the management service.

---

## Tests

**File:** `cmd/management_test.go`

- `TestHandleModelEndpoints_InvalidPath` — path too short → 400
- `TestHandleModelEndpoints_UnknownEndpoint` — `/api/v1/models/x/foo` → 404
- `TestHandleModelMetrics_MethodNotAllowed` — POST/PUT/DELETE → 405
- `TestHandleModelMetrics_ModelNotRunning` — non-existent model → 404 (or 500
  if Docker is unavailable)

Run with:
```bash
go test ./cmd -run TestHandleModel -v
```

---

## Files Changed

| File | Action | Lines |
|------|--------|-------|
| `cmd/management.go` | MODIFY — add handler, route, imports | +85 |
| `cmd/management_test.go` | CREATE — endpoint tests | +68 |
| `docs/PHASE_9_VLLM_METRICS_ENDPOINT.md` | CREATE — this doc | — |
