# Gateway Model-Level Access Control (Phase 2)

## Overview

Phase 2 adds model-level access control to the gateway. Users can now be restricted to specific models, with per-user allow lists. Admin users can grant access to all models using the wildcard `"*"`.

---

## How It Works

### Request Flow with Access Control

```
Request from User Alice
POST /v1/chat/completions
Header: X-API-Key: sk_alice_123
Body: {"model":"proprietary-model","messages":[]}

          ↓
   1. Extract API Key: sk_alice_123
          ↓
   2. Authenticate → ValidateToken() → "alice"
          ↓
   3. Extract Model: "proprietary-model"
          ↓
   4. CHECK ACCESS CONTROL (NEW) ← Phase 2
      IsAllowed("alice", "proprietary-model")?
      ✗ NO: alice.AllowedModels = ["mistral-7b", "llama-3-8b"]
          ↓
      403 Forbidden
      Response: {"error":"access denied","model":"proprietary-model"}
```

### Positive Case (Model Allowed)

```
Request from User Alice
POST /v1/chat/completions
Header: X-API-Key: sk_alice_123
Body: {"model":"mistral-7b","messages":[]}

          ↓
   1. Extract API Key: sk_alice_123
          ↓
   2. Authenticate → ValidateToken() → "alice"
          ↓
   3. Extract Model: "mistral-7b"
          ↓
   4. CHECK ACCESS CONTROL
      IsAllowed("alice", "mistral-7b")?
      ✓ YES: alice.AllowedModels = ["mistral-7b", "llama-3-8b"]
          ↓
   5. Check Rate Limit → Allow
          ↓
   6. Forward to Backend → 200 OK
```

---

## Setup: Grant Model Access

### Add User and Grant Models

```bash
# Create alice
sovstack keys add alice --department research --role analyst

# Grant access to specific models
sovstack keys grant-model alice mistral-7b
sovstack keys grant-model alice llama-3-8b

# Verify
sovstack keys info alice
# Output:
#   Allowed Models: mistral-7b, llama-3-8b
```

### Grant Admin Wildcard Access

```bash
# Create admin with access to ALL models
sovstack keys add admin --role admin

# Grant wildcard
sovstack keys grant-model admin "*"

# Verify
sovstack keys info admin
# Output:
#   Allowed Models: *
```

---

## Configuration

### In keys.json

When using KeyStore-backed authentication, the gateway automatically enables access control.

```json
{
  "users": {
    "alice": {
      "id": "alice",
      "key": "sk_alice_123...",
      "allowed_models": ["mistral-7b", "llama-3-8b"],
      "...": "..."
    },
    "admin": {
      "id": "admin",
      "key": "sk_admin_456...",
      "allowed_models": ["*"],
      "...": "..."
    }
  }
}
```

### Enable on Gateway Startup

```bash
# Phase 2 access control is enabled automatically when using keys.json
sovstack gateway --keys ~/.sovereignstack/keys.json

# Output includes:
# Access Control: Enabled (Phase 2)
```

### Backward Compatibility

Access control is **optional**. If not using KeyStore:
- No AccessController registered
- All authenticated users can access all models
- Existing behavior unchanged (backward compatible)

---

## Testing Phase 2

### Test Case 1: Allowed Access

```bash
# Setup
sovstack keys add alice
sovstack keys grant-model alice mistral-7b
sovstack gateway --keys ~/.sovereignstack/keys.json

# Test
curl -H "X-API-Key: sk_alice_..." \
     -d '{"model":"mistral-7b","messages":[]}' \
     http://localhost:8001/v1/chat/completions

# Expected: 200 OK (or proxied to backend)
```

### Test Case 2: Denied Access

```bash
# Same setup as Test 1

# Test: Try accessing model alice doesn't have
curl -H "X-API-Key: sk_alice_..." \
     -d '{"model":"proprietary-model","messages":[]}' \
     http://localhost:8001/v1/chat/completions

# Expected: 403 Forbidden
# Response: {"error":"access denied","model":"proprietary-model"}
```

### Test Case 3: Wildcard Admin

```bash
# Setup
sovstack keys add admin
sovstack keys grant-model admin "*"
sovstack gateway --keys ~/.sovereignstack/keys.json

# Test: Admin can access any model
curl -H "X-API-Key: sk_admin_..." \
     -d '{"model":"mistral-7b","messages":[]}' \
     http://localhost:8001/v1/chat/completions
# Expected: 200 OK

curl -H "X-API-Key: sk_admin_..." \
     -d '{"model":"proprietary-model","messages":[]}' \
     http://localhost:8001/v1/chat/completions
# Expected: 200 OK
```

### Test Case 4: No Models Granted

```bash
# Setup
sovstack keys add bob
# Don't grant any models
sovstack gateway --keys ~/.sovereignstack/keys.json

# Test
curl -H "X-API-Key: sk_bob_..." \
     -d '{"model":"mistral-7b","messages":[]}' \
     http://localhost:8001/v1/chat/completions

# Expected: 403 Forbidden
```

---

## Audit Logging

Access denials are logged to the audit trail:

### CLI View

```bash
curl http://localhost:8001/api/audit/logs
```

### Example Log Entry

```json
{
  "timestamp": "2026-04-30T10:15:32Z",
  "event_type": "error",
  "user": "alice",
  "endpoint": "/v1/chat/completions",
  "status_code": 403,
  "error": "access denied to model: proprietary-model"
}
```

---

## Common Patterns

### Pattern 1: Team-Based Access

```bash
# Research team - can use research models only
sovstack keys add researcher1
sovstack keys grant-model researcher1 mistral-7b
sovstack keys grant-model researcher1 llama-3-8b

# Operations team - can use all production models
sovstack keys add operator1
sovstack keys grant-model operator1 production-model-1
sovstack keys grant-model operator1 production-model-2
```

### Pattern 2: Tiered Service

```bash
# Free tier - limited models
sovstack keys add free-user
sovstack keys grant-model free-user mistral-7b

# Pro tier - more models
sovstack keys add pro-user
sovstack keys grant-model pro-user mistral-7b
sovstack keys grant-model pro-user llama-3-8b
sovstack keys grant-model pro-user custom-model

# Enterprise tier - all models
sovstack keys add enterprise-user
sovstack keys grant-model enterprise-user "*"
```

### Pattern 3: Cost Control

```bash
# Expensive models restricted to admins
sovstack keys add admin
sovstack keys grant-model admin "*"

# Regular users get only economical models
sovstack keys add user1
sovstack keys grant-model user1 mistral-7b  # cheap, fast
```

---

## Error Responses

### 403 Forbidden (Access Denied)

When user tries to access a model they're not allowed to use:

```
HTTP/1.1 403 Forbidden
Content-Type: application/json

{"error":"access denied","model":"proprietary-model"}
```

### 401 Unauthorized (Invalid Key)

When API key doesn't exist:

```
HTTP/1.1 401 Unauthorized

Unauthorized
```

### 429 Too Many Requests (Rate Limited)

When user exceeds rate limit (tested before access control):

```
HTTP/1.1 429 Too Many Requests

Rate limit exceeded
```

---

## Implementation Details

### Code Files

- **`core/gateway/access.go`** — AccessController interface and implementations
  - `AccessController` interface
  - `KeyStoreAccessController` — Checks user's allowed_models
  - `DenyAllAccessController` — Always deny (testing)
  - `AllowAllAccessController` — Always allow (default)

- **`core/gateway/proxy.go`** — Modified to include access check
  - Added `AccessController` field to `Gateway` struct
  - Added `AccessController` to `GatewayConfig`
  - Access check in `ServeHTTP()` after auth, before rate limit

- **`cmd/gateway.go`** — Wire up access controller
  - Create `KeyStoreAccessController` when using keys.json
  - Pass to `GatewayConfig`
  - Print "Access Control: Enabled" on startup

- **`core/gateway/access_test.go`** — Unit tests (9 test cases)
  - `CanAccess_Allowed` — User with access
  - `CanAccess_Denied` — User without access
  - `Wildcard` — Admin with "*"
  - `NotFound` — Unknown user
  - `Empty` — User with no allowed models
  - `DenyAllAccessController` — Always deny
  - `AllowAllAccessController` — Always allow
  - `AccessControlIntegration` — Full flow test

### Request Flow in Code

```go
// core/gateway/proxy.go :: ServeHTTP()

// 1. Authenticate
userID, err := gw.authProvider.ValidateToken(apiKey)
if err != nil {
    return 401 Unauthorized
}

// 2. Extract model name
modelName := extractModelName(r)

// 3. Check access control (Phase 2)
if gw.accessController != nil && userID != "" {
    if !gw.accessController.CanAccess(userID, modelName) {
        return 403 Forbidden  // New in Phase 2
    }
}

// 4. Check rate limit
if !gw.rateLimiter.Allow(userID, gw.requestsPerMin) {
    return 429 Too Many Requests
}

// 5. Forward to backend
gw.proxy.ServeHTTP(wrappedWriter, r)
```

---

## Performance Considerations

### Lookup Time
- `KeyStoreAccessController.CanAccess()` is O(n) where n = number of allowed models
- Typical: 1-10 models per user → <1µs lookup time
- No database roundtrip — all data in memory

### Memory Usage
- Per-user: ~100 bytes (ID, key, metadata) + 50 bytes per allowed model
- Example: 1000 users with 5 models each = ~250KB

### Concurrency
- Thread-safe via KeyStore's RWMutex
- Multiple requests can check access simultaneously

---

## Migration from Phase 1

Phase 1 (hardcoded keys) had no access control. Phase 2 adds it:

### Before (Phase 1)
```bash
# All authenticated users access all models
curl -H "X-API-Key: sk_alice" /v1/chat/completions  # model="x"
curl -H "X-API-Key: sk_alice" /v1/chat/completions  # model="y"
curl -H "X-API-Key: sk_alice" /v1/chat/completions  # model="z"
# All succeed ✓
```

### After (Phase 2)
```bash
# Setup: grant models selectively
sovstack keys add alice
sovstack keys grant-model alice mistral-7b

# Now access is controlled
curl -H "X-API-Key: sk_alice" /v1/chat/completions  # model="mistral-7b"
# Succeeds ✓

curl -H "X-API-Key: sk_alice" /v1/chat/completions  # model="llama-3"
# Fails with 403 ✗
```

---

## Next Steps

### Phase 2b: Token Quotas
Coming soon — limit total tokens per user (daily/monthly).

```bash
# Future:
sovstack keys set-quota alice --daily 500000 --monthly 10000000
# alice gets 500K tokens/day, 10M tokens/month
```

### Phase 3: Multi-Model Routing
Coming soon — dynamic routing to different backend ports.

```bash
# Each model runs on a different port
:8000 → mistral-7b
:8001 → llama-3-8b
:8002 → proprietary-model

# Gateway automatically routes based on model name
```

---

## Troubleshooting

### "Access denied" when should be allowed

```bash
# Check user's allowed models
sovstack keys info alice

# If not granted, add it
sovstack keys grant-model alice mistral-7b
```

### Access control not working (gateway still allows all)

```bash
# Verify using keys.json (not hardcoded keys)
sovstack gateway --keys ~/.sovereignstack/keys.json

# Should see:
# Access Control: Enabled (Phase 2)
```

### Model name not recognized

```bash
# Extract model names from paths like:
# /v1/models/mistral-7b/chat/completions

# Or from request body:
# {"model":"mistral-7b",...}

# Use exact name in grants:
sovstack keys grant-model alice mistral-7b
```

---

## Related Documentation

- [API Key Management](./KEYS_MANAGEMENT.md) — User creation and key management
- [Architecture](./ARCHITECTURE_AUTH.md) — Authentication flow details
- [Token Quotas](./TOKEN_QUOTAS.md) — Budget enforcement (Phase 2b)
- [Monitoring](./MONITORING.md) — Track access patterns via audit logs
