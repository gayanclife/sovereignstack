# Gateway Authentication & Authorization Architecture

## Overview

The SovereignStack gateway implements a pluggable authentication system with multi-layered access control. This document explains how requests flow through authentication, access control, rate limiting, and finally to the backend.

---

## Component Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         HTTP Request                                │
│  POST /v1/chat/completions                                          │
│  Header: "X-API-Key: sk_xxx" or "Authorization: Bearer sk_xxx"     │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
                               ↓
┌──────────────────────────────────────────────────────────────────────┐
│ core/gateway/proxy.go :: Gateway.ServeHTTP()                         │
│                                                                      │
│  1️⃣  Extract API Key from header                                    │
│      (X-API-Key or Authorization: Bearer)                            │
└──────────────────────────┬───────────────────────────────────────────┘
                           │
                           ↓
┌──────────────────────────────────────────────────────────────────────┐
│ core/gateway/auth.go :: AuthProvider.ValidateToken(apiKey)           │
│                                                                      │
│  Can be implemented by:                                              │
│  - APIKeyAuthProvider (in-memory, for testing)                       │
│  - gatewayAuthAdapter (wraps KeyStore from keys.json)               │
│                                                                      │
│  Returns: (userID string, error)                                     │
│  ✓ Valid key → userID extracted                                     │
│  ✗ Invalid key → error, gateway returns 401 Unauthorized            │
└──────────────────────────┬───────────────────────────────────────────┘
                           │
                           ├─ 401 Unauthorized (invalid key)
                           │
                           ↓
          ┌─────────────────────────────────────────┐
          │ Access Control (Phase 2)                │
          │ NOT YET IMPLEMENTED                     │
          │                                         │
          │ Future: Check if user can access model  │
          │ → 403 Forbidden if not allowed         │
          └─────────────────────────────────────────┘
                           │
                           ↓
┌──────────────────────────────────────────────────────────────────────┐
│ core/gateway/proxy.go :: RateLimiter.Allow(userID, rps)             │
│                                                                      │
│  Per-user token bucket limiter                                       │
│  Configured: --rate-limit (requests per minute, default 100)         │
│                                                                      │
│  ✓ Within limit → allow request                                      │
│  ✗ Exceeded → return 429 Too Many Requests                           │
└──────────────────────────┬───────────────────────────────────────────┘
                           │
                           ├─ 429 Too Many Requests (rate limited)
                           │
                           ↓
┌──────────────────────────────────────────────────────────────────────┐
│ core/gateway/proxy.go :: Gateway.director()                          │
│                                                                      │
│  Rewrite request URL to backend                                      │
│  - Extract model name from path/body                                 │
│  - Look up backend service (currently hardcoded to --backend URL)    │
│  - Set proper scheme, host, port                                     │
│  - Forward to: http://localhost:8000 or similar                      │
└──────────────────────────┬───────────────────────────────────────────┘
                           │
                           ↓
        ┌──────────────────────────────────────┐
        │  vLLM Backend (or other service)     │
        │  http://localhost:8000               │
        │                                      │
        │  - Models indexed                    │
        │  - Inference executed                │
        │  - Response returned                 │
        └──────────────────────────────────────┘
                           │
                           ↓
┌──────────────────────────────────────────────────────────────────────┐
│ core/gateway/proxy.go :: ServeHTTP() (return to client)              │
│                                                                      │
│ - Capture response status code and body size                         │
│ - Extract tokens used (if present in response)                       │
│ - Write response back to client                                      │
└──────────────────────────┬───────────────────────────────────────────┘
                           │
                           ↓
┌──────────────────────────────────────────────────────────────────────┐
│ core/audit/sqlite.go :: AuditLogger.LogRequest()                     │
│                                                                      │
│  Record event for compliance:                                        │
│  - Timestamp                                                         │
│  - User ID                                                           │
│  - Model (extracted from request)                                    │
│  - Endpoint (path)                                                   │
│  - Method (POST, GET, etc.)                                          │
│  - Status code                                                       │
│  - Tokens used/generated                                             │
│  - Duration                                                          │
│  - Client IP                                                         │
│  - Correlation ID (for tracing)                                      │
│                                                                      │
│  Storage: SQLite (encrypted) or in-memory depending on --audit-db   │
└──────────────────────────────────────────────────────────────────────┘
                           │
                           ↓
          ┌─────────────────────────────────────┐
          │  Response to Client ✓               │
          │  200 OK + response body             │
          └─────────────────────────────────────┘
```

---

## Authentication Providers

### Interface Definition

All auth providers implement the `AuthProvider` interface:

```go
// core/gateway/auth.go
type AuthProvider interface {
    ValidateToken(token string) (userID string, err error)
    AddKey(apiKey, userID string)           // For manual key registration
    RemoveKey(apiKey string)                 // For manual key removal
}
```

### Implementation 1: APIKeyAuthProvider (In-Memory)

**Location:** `core/gateway/auth.go`

Used for development and testing. Keys live only in memory.

```go
type APIKeyAuthProvider struct {
    mu   sync.RWMutex
    keys map[string]string  // API key → user ID
}

authProvider := gateway.NewAPIKeyAuthProvider()
authProvider.AddKey("sk_test_123", "test-user")
authProvider.ValidateToken("sk_test_123")  // Returns "test-user"
```

**Pros:** Simple, no file I/O, fast
**Cons:** Keys lost on restart, hardcoded in source

### Implementation 2: gatewayAuthAdapter (With KeyStore)

**Location:** `cmd/gateway.go`

Wraps the `KeyStore` to provide auth via `keys.json` file.

```go
type gatewayAuthAdapter struct {
    store *keys.KeyStore
}

func (a *gatewayAuthAdapter) ValidateToken(token string) (string, error) {
    user, _ := a.store.GetByKey(token)
    if user == nil {
        return "", fmt.Errorf("invalid API key")
    }
    return user.ID, nil
}

// Usage
ks, _ := keys.LoadKeyStore("~/.sovereignstack/keys.json")
authProvider := &gatewayAuthAdapter{store: ks}
```

**Pros:** Persistent, user management CLI, metadata (department, team, role)
**Cons:** File I/O overhead (minimal with caching), requires restart to reload keys

---

## Request Flow: Step-by-Step

### Example 1: Valid Request

**Request:**
```bash
curl -H "X-API-Key: sk_alice_123" \
     -d '{"model":"mistral-7b","messages":[]}' \
     http://localhost:8001/v1/chat/completions
```

**Flow:**

1. **Extract Key** → `sk_alice_123` from header
2. **Validate** → `ValidateToken("sk_alice_123")` → Returns `"alice"`
3. **Access Control** (Phase 2) → Check if alice can access mistral-7b → ✓ Yes
4. **Rate Limit** → Check if alice is within 100 req/min → ✓ Yes
5. **Route** (Phase 3) → Find mistral-7b backend → `http://localhost:8000`
6. **Forward** → POST to `http://localhost:8000/v1/chat/completions`
7. **Receive** → Backend returns response with tokens used
8. **Audit** → Log: `{user:"alice", model:"mistral-7b", tokens:450, status:200}`
9. **Return** → 200 OK + response body to client

---

### Example 2: Invalid Key

**Request:**
```bash
curl -H "X-API-Key: sk_invalid" \
     http://localhost:8001/v1/chat/completions
```

**Flow:**

1. **Extract Key** → `sk_invalid` from header
2. **Validate** → `ValidateToken("sk_invalid")` → Returns error
3. **Return** → 401 Unauthorized
4. **Audit** → Log: `{error:"invalid_key", timestamp:...}`
5. **No forwarding** to backend

---

### Example 3: Access Denied (Phase 2)

**Request:**
```bash
# alice tries to access model she doesn't have permission for
curl -H "X-API-Key: sk_alice_123" \
     -d '{"model":"proprietary-model",...}'
     http://localhost:8001/v1/chat/completions
```

**Flow:**

1. **Extract Key** → `sk_alice_123`
2. **Validate** → Returns `"alice"`
3. **Access Control** → Check alice can access proprietary-model → ✗ No
4. **Return** → 403 Forbidden with message: `"access denied to model: proprietary-model"`
5. **Audit** → Log: `{user:"alice", model:"proprietary-model", status:403}`
6. **No forwarding** to backend

---

### Example 4: Rate Limited

**Request:**
```bash
# alice has 100 req/min limit and already made 100 requests in last min
curl -H "X-API-Key: sk_alice_123" \
     http://localhost:8001/v1/chat/completions
```

**Flow:**

1. **Extract Key** → `sk_alice_123`
2. **Validate** → Returns `"alice"`
3. **Access Control** → ✓ Allowed
4. **Rate Limit** → alice.tokens < 1 → ✗ Exceeded
5. **Return** → 429 Too Many Requests
6. **Audit** → Log: `{user:"alice", status:429, reason:"rate_limited"}`
7. **No forwarding** to backend

---

## KeyStore Data Flow

### Add User

```go
// CLI: sovstack keys add alice
user := &keys.UserProfile{
    ID:              "alice",
    Key:             "sk_a1b2c3d4...",  // Generated
    Department:      "research",
    AllowedModels:   []string{},        // Starts empty
    RateLimitPerMin: 100,               // Default
    MaxTokensPerDay: 0,                 // 0 = unlimited
    CreatedAt:       time.Now(),
}
ks.AddUser(user)  // Writes to ~/.sovereignstack/keys.json
```

### Load on Gateway Start

```go
// cmd/gateway.go :: runGateway()
ks, _ := keys.LoadKeyStore("~/.sovereignstack/keys.json")
authProvider := &gatewayAuthAdapter{store: ks}
// Now all requests validated against ks.Users
```

### Update Permissions (Phase 2)

```go
// CLI: sovstack keys grant-model alice mistral-7b
user, _ := ks.GetByID("alice")
user.AllowedModels = append(user.AllowedModels, "mistral-7b")
ks.AddUser(user)  // Writes updated profile to disk
// ⚠️ Gateway must restart or periodically reload to see changes
```

---

## Token Bucket Rate Limiting

**Location:** `core/gateway/proxy.go`

Each user has an independent token bucket.

### Algorithm

```
Configured: --rate-limit 100 (req/min)

When request arrives:
  1. Calculate elapsed time since last refill
  2. Add tokens: (elapsed_minutes * 100) to user's bucket
  3. Cap at 100 (don't overfill)
  4. If tokens >= 1:
       - Consume 1 token
       - Allow request
     Else:
       - Return 429 Too Many Requests
```

### Example

- **Configuration:** 100 req/min per user
- **Rate:** 100/60 ≈ 1.67 requests per second
- **Burst:** Up to 100 requests at once (then empty bucket)

```
Timeline:
T=0s:   alice makes 100 requests → bucket depleted
T=1s:   bucket has 1.67 tokens → 1 request allowed
T=30s:  bucket has ~50 tokens → 50 more requests allowed
T=60s:  bucket refills to 100 → ready for next burst
```

---

## Audit Logging Integration

Every request is logged regardless of outcome:

```go
// core/audit/sqlite.go
type AuditLog struct {
    Timestamp      time.Time
    EventType      string      // "request", "error", "auth_failure"
    User           string      // User ID or "unknown"
    Model          string      // Extracted from request
    Endpoint       string      // /v1/chat/completions
    Method         string      // POST, GET, etc.
    StatusCode     int         // 200, 401, 403, 429, 500, etc.
    TokensUsed     int64       // From response (if available)
    DurationMS     int64       // Request processing time
    ErrorMessage   string      // If error occurred
    ClientIP       string      // From X-Forwarded-For or RemoteAddr
    CorrelationID  string      // For tracing
}
```

**Storage options:**
- **SQLite** (`--audit-db ./audit.db`) — Encrypted, persistent, queryable
- **In-Memory** (default) — Fast, volatile, limited to N logs

**Access via API:**
- `GET /api/audit/logs?n=100` — Last 100 logs
- `GET /api/audit/stats` — Aggregate statistics

---

## Security Considerations

### API Key Management
- Keys are SHA-256 hashes of `userID + UnixNano()` — cryptographically random
- Keys stored in `keys.json` with file permissions `0600` (owner read/write only)
- Keys transmitted via HTTPS should be used in production

### Authentication
- Each user isolated — cannot impersonate others
- Invalid keys produce no information leak (generic 401)
- Failed auth attempts logged for monitoring

### Rate Limiting
- Per-user token bucket — prevents single user from monopolizing gateway
- Configurable — allows for tiered service plans
- Burst-friendly — allows batch processing while protecting baseline

### Audit Trail
- All requests logged with timestamp, user, model, status
- Encryption available (SQLite with `-audit-key`)
- Used for compliance, debugging, security investigation

---

## Future Extensions (Phases 2-4)

### Phase 2: Model-Level Access Control
```go
// core/gateway/access.go
type AccessController interface {
    CanAccess(userID, modelName string) bool
}

// In Gateway.ServeHTTP(), add check:
if !gw.accessController.CanAccess(userID, modelName) {
    http.Error(w, "403 Forbidden", http.StatusForbidden)
    return
}
```

### Phase 2b: Token Quotas
```go
// core/gateway/quota.go
type TokenQuotaManager struct {
    daily   map[string]*userQuota
    monthly map[string]*userQuota
}

// In Gateway.ServeHTTP(), add check before forwarding:
if !gw.quotaManager.CheckQuota(userID) {
    http.Error(w, "429 Quota Exceeded", http.StatusTooManyRequests)
    return
}

// After response received, record tokens:
gw.quotaManager.Record(userID, inputTokens, outputTokens)
```

### Phase 3: Multi-Model Routing
```go
// core/gateway/router.go
type ModelRouter struct {
    registry map[string]*ModelBackend
}

// In director(), add lookup:
if backend, ok := gw.router.GetBackend(modelName) {
    req.URL.Host = fmt.Sprintf("localhost:%d", backend.Port)
    req.URL.Path = stripModelPrefix(req.URL.Path, modelName)
}
```

### Phase 4: Prometheus Metrics
```go
// core/gateway/metrics.go
type GatewayMetrics struct {
    requestsTotal    map[string]int64   // {user,model,status}
    requestDuration  map[string][]float64 // {model}
    tokenQuotaExceeded map[string]int64 // {user,period}
}

// In ServeHTTP():
gw.metrics.RecordRequest(userID, model, status, duration)
gw.metrics.RecordTokens(userID, model, inputTokens, outputTokens)

// Expose via:
http.HandleFunc("/metrics", gw.metrics.WritePrometheusText)
```

---

## Testing

### Unit Tests
- `core/keys/store_test.go` — KeyStore add, get, remove, persistence
- `core/gateway/auth.go` — AuthProvider implementations (mock tests)

### Integration Tests
- Gateway with sample keys.json
- Full request lifecycle: auth → rate limit → audit
- Error cases: invalid key, rate limit exceeded

### Load Tests
- Concurrent users and requests
- Token bucket refill correctness
- Audit log throughput

---

## Configuration Reference

### Gateway Startup Flags

```bash
sovstack gateway \
  --backend http://localhost:8000           # vLLM backend URL
  --port 8001                               # Gateway listen port
  --rate-limit 100                          # Requests per minute per user
  --api-key-header X-API-Key                # HTTP header name
  --keys ~/.sovereignstack/keys.json        # KeyStore file path (empty = hardcoded)
  --audit-db ./sovstack-audit.db            # SQLite audit log (empty = in-memory)
  --audit-key $SOVSTACK_AUDIT_KEY           # Encryption key (auto-generated if empty)
  --audit-buffer 10000                      # In-memory log max size
```

### Environment Variables

```bash
export SOVSTACK_AUDIT_KEY="hex_string_of_32_bytes"
# Used if --audit-key not provided and --audit-db set
```

---

## Related Documentation

- [API Key Management Guide](./KEYS_MANAGEMENT.md) — CLI usage
- [Monitoring & Metrics](./MONITORING.md) — Prometheus metrics (Phase 4)
- [Access Control Policies](./GATEWAY_ACCESS_CONTROL.md) — Model access (Phase 2)
- [Token Quotas](./TOKEN_QUOTAS.md) — Budget enforcement (Phase 2b)
