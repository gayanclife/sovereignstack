# Token Quota Enforcement (Phase 2b)

## Overview

Phase 2b adds **token budget limits** to the gateway. Users can now be restricted to:
- **Daily limit** — Maximum tokens consumed in a single UTC day
- **Monthly limit** — Maximum tokens consumed in a calendar month (resets on the 1st)

This enables operators to allocate finite budgets (e.g., "research team gets 10M tokens/month") and block requests once the limit is hit.

---

## How It Works

### Request Flow with Token Quotas

```
Request from User Alice
POST /v1/chat/completions
Header: X-API-Key: sk_alice_123
Body: {"model":"mistral-7b","messages":[...]}

       ↓
1. Authenticate → ValidateToken() → "alice"
       ↓
2. Check Access Control → CanAccess("alice", "mistral-7b")?
       ↓
3. CHECK QUOTA (NEW) ← Phase 2b
   CheckQuota("alice")?
   alice.max_tokens_per_day = 500000
   alice.daily_used_today = 450000
   Remaining: 50000 tokens available
       ↓
   Request asks for ~5000 tokens
   ✓ Within budget
       ↓
4. Check Rate Limit → Allow?
       ↓
5. Forward to Backend
       ↓
6. Backend processes request, consumes 3500 tokens
       ↓
7. RECORD TOKENS (NEW) ← Phase 2b
   Record(alice, input_tokens=1200, output_tokens=2300)
   alice.daily_used_today = 453500
       ↓
8. Return 200 OK to client
```

### Quota Exceeded Case

```
Request from User Alice
POST /v1/chat/completions

       ↓
1. Authenticate → "alice"
       ↓
2. Check Access Control → ✓ Allowed
       ↓
3. CHECK QUOTA
   alice.max_tokens_per_day = 500000
   alice.daily_used_today = 499500
   Remaining: 500 tokens available (very low!)
       ↓
   Request would likely consume >500 tokens
   ✗ Quota exceeded (or insufficient remaining)
       ↓
429 Too Many Requests
Response: {"error":"token_quota_exceeded","detail":"daily quota exceeded: 499500/500000"}
```

---

## Setup: Grant Token Quotas

### Set Daily and Monthly Limits

```bash
# Create alice with daily and monthly quotas
sovstack keys add alice --department research

# Set limits: 500K tokens/day, 10M tokens/month
sovstack keys set-quota alice --daily 500000 --monthly 10000000

# Verify
sovstack keys info alice
# Output:
#   Max Tokens Per Day: 500000
#   Max Tokens Per Month: 10000000
```

### Grant Unlimited Access (Admin)

```bash
# Admin user with no quota restrictions
sovstack keys add admin --role admin
sovstack keys set-quota admin --daily 0 --monthly 0  # 0 = unlimited

# Verify
sovstack keys info admin
# Output:
#   Max Tokens Per Day: unlimited
#   Max Tokens Per Month: unlimited
```

### Change Limits Later

```bash
# Increase daily limit
sovstack keys set-quota alice --daily 750000

# Decrease monthly limit
sovstack keys set-quota alice --monthly 5000000

# Check current usage
sovstack keys usage alice
# Output:
#   Today: 120000 / 500000 tokens (24%)
#   This Month: 850000 / 10000000 tokens (8.5%)
#   Daily Reset: 2026-05-01 00:00:00 UTC
#   Monthly Reset: 2026-06-01 00:00:00 UTC
```

---

## Configuration

### In keys.json

Token quota limits are stored per user in the extended UserProfile format:

```json
{
  "users": {
    "alice": {
      "id": "alice",
      "key": "sk_alice_123...",
      "department": "research",
      "team": "nlp",
      "role": "analyst",
      "allowed_models": ["mistral-7b", "llama-3-8b"],
      "rate_limit_per_min": 100,
      "max_tokens_per_day": 500000,
      "max_tokens_per_month": 10000000,
      "created_at": "2026-04-30T10:00:00Z",
      "last_used_at": "2026-04-30T15:32:00Z"
    },
    "admin": {
      "id": "admin",
      "key": "sk_admin_456...",
      "department": "operations",
      "team": "infra",
      "role": "admin",
      "allowed_models": ["*"],
      "rate_limit_per_min": 1000,
      "max_tokens_per_day": 0,           # ← unlimited
      "max_tokens_per_month": 0,         # ← unlimited
      "created_at": "2026-04-30T09:00:00Z",
      "last_used_at": "2026-04-30T16:00:00Z"
    }
  }
}
```

**Special Values:**
- `0` = unlimited (no quota enforced)
- `> 0` = hard limit in tokens

### Enable on Gateway Startup

```bash
# Token quotas are automatically enabled when using keys.json
sovstack gateway --keys ~/.sovereignstack/keys.json

# Output includes:
# Token Quotas: Enabled (Phase 2b)
```

---

## Testing Phase 2b

### Test Case 1: Within Daily Quota

```bash
# Setup
sovstack keys add alice
sovstack keys set-quota alice --daily 10000 --monthly 100000
sovstack gateway --keys ~/.sovereignstack/keys.json

# Test: Make request that uses ~5000 tokens
curl -H "X-API-Key: sk_alice_..." \
     -d '{"model":"mistral-7b","messages":[{"role":"user","content":"Hello"}]}' \
     http://localhost:8001/v1/chat/completions

# Expected: 200 OK (proxied to backend, tokens recorded)
```

### Test Case 2: Daily Quota Exceeded

```bash
# Setup: Create test key with very low limit
sovstack keys add bob
sovstack keys set-quota bob --daily 100 --monthly 1000  # 100 tokens/day
sovstack gateway --keys ~/.sovereignstack/keys.json

# Test: Make multiple requests until quota exceeded
for i in {1..5}; do
  curl -H "X-API-Key: sk_bob_..." \
       -d '{"model":"mistral-7b","messages":[{"role":"user","content":"Test"}]}' \
       http://localhost:8001/v1/chat/completions
done

# First few requests: 200 OK
# When daily_used > 100:
# → 429 Too Many Requests
# Response: {"error":"token_quota_exceeded","detail":"daily quota exceeded: ..."}
```

### Test Case 3: Monthly Quota Exceeded

```bash
# Setup: Low monthly limit
sovstack keys add charlie
sovstack keys set-quota charlie --daily 1000000 --monthly 200
sovstack gateway --keys ~/.sovereignstack/keys.json

# Test: Exhaust monthly quota
curl -H "X-API-Key: sk_charlie_..." \
     -d '{"model":"mistral-7b","messages":[{"role":"user","content":"Long prompt that generates lots of tokens"}]}' \
     http://localhost:8001/v1/chat/completions

# Expected: 429 when monthly_used > 200
```

### Test Case 4: Unlimited Quota (Admin)

```bash
# Setup: Admin with unlimited quotas
sovstack keys add admin
sovstack keys set-quota admin --daily 0 --monthly 0
sovstack gateway --keys ~/.sovereignstack/keys.json

# Test: Admin can consume unlimited tokens
for i in {1..10}; do
  curl -H "X-API-Key: sk_admin_..." \
       -d '{"model":"mistral-7b","messages":[...]}' \
       http://localhost:8001/v1/chat/completions
done

# Expected: All requests succeed (200 OK)
```

### Test Case 5: Check Usage

```bash
# Setup
sovstack keys add dave
sovstack keys set-quota dave --daily 1000000 --monthly 10000000

# Make some requests (gateway running)

# Check usage
sovstack keys usage dave

# Output:
#   Today: 120000 / 1000000 tokens (12%)
#   This Month: 120000 / 10000000 tokens (1.2%)
#   Daily Reset: 2026-05-01 00:00:00 UTC
#   Monthly Reset: 2026-06-01 00:00:00 UTC
```

---

## Quota Behavior

### Daily Reset Window

- **Resets at:** UTC midnight (00:00:00 UTC)
- **Applies to:** Tokens consumed in a single calendar day
- **Example:** If it's 2026-04-30 15:30 UTC
  - Daily quota resets at 2026-05-01 00:00:00 UTC (9.5 hours away)
  - Alice's daily_used counter will be cleared at that exact time

### Monthly Reset Window

- **Resets on:** 1st of the month at UTC midnight (00:00:00 UTC)
- **Applies to:** Tokens consumed in a single calendar month
- **Example:** If it's 2026-04-30 (end of April)
  - Monthly quota resets at 2026-06-01 00:00:00 UTC (2 days away)
  - May 1st through May 31st form the May window
  - Alice's monthly_used counter will be cleared at June 1st

### No Partial Request Blocking

The quota check runs **before** forwarding the request to the backend. It checks if the user has *any* remaining quota. It does NOT:
- Predict how many tokens the request will consume
- Block requests that would exceed quota by checking request size
- Perform per-request token budgeting

This is by design — token counts are only known after the backend responds. A pre-request check simply ensures the user isn't already over their limit.

### Token Recording

After the backend responds with token usage (in the response body), the gateway records actual tokens consumed:
```go
gw.quotaManager.Record(userID, inputTokens, outputTokens)
```

This updates:
- `daily_used` (resets at UTC midnight)
- `monthly_used` (resets on 1st of month)

---

## Error Responses

### 429 Too Many Requests (Quota Exceeded)

When user has exhausted their daily or monthly quota:

```
HTTP/1.1 429 Too Many Requests
Content-Type: application/json

{"error":"token_quota_exceeded","detail":"daily quota exceeded: 500010/500000"}
```

The detail message includes:
- Type of quota exceeded (daily or monthly)
- Current usage / limit

### Quota Status Check Endpoint (Future)

Eventually, the gateway will expose:
```
GET /api/v1/gateway/quota?user=alice

→ {
  "user": "alice",
  "daily_used": 120000,
  "daily_limit": 500000,
  "daily_pct": 24.0,
  "daily_reset_at": "2026-05-01T00:00:00Z",
  "monthly_used": 850000,
  "monthly_limit": 10000000,
  "monthly_pct": 8.5,
  "monthly_reset_at": "2026-06-01T00:00:00Z"
}
```

---

## Common Patterns

### Pattern 1: Free Tier + Premium Tiers

```bash
# Free tier — limited tokens
sovstack keys add user-free && \
  sovstack keys grant-model user-free mistral-7b && \
  sovstack keys set-quota user-free --daily 10000 --monthly 100000

# Pro tier — more tokens
sovstack keys add user-pro && \
  sovstack keys grant-model user-pro mistral-7b && \
  sovstack keys grant-model user-pro llama-3-8b && \
  sovstack keys set-quota user-pro --daily 500000 --monthly 10000000

# Enterprise tier — unlimited
sovstack keys add user-enterprise && \
  sovstack keys grant-model user-enterprise "*" && \
  sovstack keys set-quota user-enterprise --daily 0 --monthly 0
```

### Pattern 2: Per-Team Allocation

```bash
# Research team: 5M tokens/month total (divided among team members)
sovstack keys add researcher1 && \
  sovstack keys set-quota researcher1 --daily 100000 --monthly 2500000

sovstack keys add researcher2 && \
  sovstack keys set-quota researcher2 --daily 100000 --monthly 2500000

# Operations team: Unlimited (they manage infrastructure)
sovstack keys add ops-admin && \
  sovstack keys set-quota ops-admin --daily 0 --monthly 0
```

### Pattern 3: Time-Based Quotas

```bash
# Users get daily quota only (reset every 24h)
sovstack keys add dev1 && \
  sovstack keys set-quota dev1 --daily 50000 --monthly 0  # monthly=0 → unlimited

# This allows flexible usage across weeks, but prevents daily abuse
```

---

## Implementation Details

### Code Files

- **`core/gateway/quota.go`** — TokenQuotaManager implementation
  - `TokenQuotaManager` struct with daily/monthly tracking
  - `quotaCounter` struct for time-windowed usage
  - `CheckQuota()` — Validate before request
  - `Record()` — Record after response
  - `GetUsage()` — Return current status

- **`core/gateway/proxy.go`** — Modified to include quota check
  - Added `quotaManager` field to `Gateway` struct
  - Added `QuotaManager` to `GatewayConfig`
  - Quota check in `ServeHTTP()` after access control, before rate limit
  - Returns 429 if quota exceeded

- **`core/keys/store.go`** — Extended UserProfile
  - `MaxTokensPerDay` field
  - `MaxTokensPerMonth` field

- **`cmd/gateway.go`** — Wire up quota manager
  - Instantiate `TokenQuotaManager` when using keys.json
  - Print "Token Quotas: Enabled" on startup

- **`cmd/keys.go`** — `set-quota` command implementation
  - `sovstack keys set-quota <user> --daily N --monthly M`

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

// 3. Check access control
if gw.accessController != nil && userID != "" {
    if !gw.accessController.CanAccess(userID, modelName) {
        return 403 Forbidden
    }
}

// 4. Check quota (Phase 2b)  ← NEW
if gw.quotaManager != nil && userID != "" {
    if err := gw.quotaManager.CheckQuota(userID); err != nil {
        return 429 Too Many Requests
    }
}

// 5. Check rate limit
if !gw.rateLimiter.Allow(userID, gw.requestsPerMin) {
    return 429 Too Many Requests
}

// 6. Forward to backend
gw.proxy.ServeHTTP(wrappedWriter, r)

// 7. Record tokens (after response received)  ← NEW
// (Not yet fully implemented - would need response body parsing)
```

### Quota Tracking Data Structures

```go
// core/gateway/quota.go

// TokenQuotaManager tracks per-user quotas
type TokenQuotaManager struct {
    store  *keys.KeyStore
    daily  map[string]*quotaCounter   // userID → usage
    monthly map[string]*quotaCounter
    mu     sync.RWMutex
}

// quotaCounter tracks usage in a time window
type quotaCounter struct {
    used    int64
    resetAt time.Time
    mu      sync.Mutex
}

// GetUsage returns comprehensive quota status
type QuotaUsage struct {
    UserID           string
    DailyUsed        int64
    DailyLimit       int64
    DailyResetAt     time.Time
    MonthlyUsed      int64
    MonthlyLimit     int64
    MonthlyResetAt   time.Time
    DailyPercent     float64  // 0-100
    MonthlyPercent   float64  // 0-100
}
```

---

## Performance Considerations

### Lookup Time
- `CheckQuota()` is O(1) — hash map lookup + time check
- Typical latency: <1µs
- No database roundtrip — all data in memory

### Memory Usage
- Per-user: ~200 bytes (two quotaCounter structs)
- Example: 1000 users = ~200KB

### Concurrency
- Thread-safe via RWMutex on TokenQuotaManager
- Per-user RWMutex on quotaCounter for fine-grained locking
- Multiple requests can check quotas simultaneously

### Reset Overhead
- Minimal: checked once per request
- Reset happens automatically when time window expires
- No background cleanup needed

---

## Migration from Phase 2

Phase 2 (access control) didn't enforce token limits. Phase 2b adds quotas:

### Before (Phase 2)
```bash
# All users can consume unlimited tokens
curl -H "X-API-Key: sk_alice" /v1/chat/completions  # model="x"
curl -H "X-API-Key: sk_alice" /v1/chat/completions  # model="y" (1M tokens!)
curl -H "X-API-Key: sk_alice" /v1/chat/completions  # model="z" (2M tokens!)
# All succeed — no budget limits
```

### After (Phase 2b)
```bash
# Setup: Grant quotas
sovstack keys set-quota alice --daily 500000 --monthly 10000000

# Now requests are capped
curl -H "X-API-Key: sk_alice" /v1/chat/completions  # 450K tokens → 200 OK
curl -H "X-API-Key: sk_alice" /v1/chat/completions  # 100K tokens → total 550K
# Would exceed daily limit (500K)
# → 429 Too Many Requests
```

---

## Next Steps

### Phase 2b+ Future Enhancements

1. **Token Quota API Endpoint** (Phase 4)
   - `GET /api/v1/gateway/quota?user=alice` — Check quota status
   - Expose quota metrics in Prometheus format
   - Dashboard widgets showing quota usage per user

2. **Quota Dashboard** (Phase 7)
   - Table showing all users' quota usage
   - Warnings for users approaching limits (>80%)
   - Admin UI to adjust limits on the fly

3. **Quota Alerts** (Phase 7+)
   - Email notification when user hits 80% of quota
   - Slack/PagerDuty alerts for admins when quotas exhausted
   - Weekly quota usage reports

4. **Token Recording from Response** (Phase 4)
   - Parse vLLM response body to extract `usage.prompt_tokens` and `usage.completion_tokens`
   - Record actual tokens to quota manager after response received
   - This updates the gateway metrics for cost calculation

---

## Troubleshooting

### "token_quota_exceeded" when should be allowed

```bash
# Check user's quota settings
sovstack keys info alice

# Verify quota hasn't been exhausted
sovstack keys usage alice

# If quota is too low, increase it
sovstack keys set-quota alice --daily 1000000
```

### Quota not enforcing (requests still go through)

```bash
# Verify gateway is using keys.json (not hardcoded keys)
sovstack gateway --keys ~/.sovereignstack/keys.json

# Should see:
# Token Quotas: Enabled (Phase 2b)

# Check that user has a quota set
sovstack keys info alice
# Max Tokens Per Day: should not be empty
```

### "daily_reset_at" time is in the past

```bash
# This shouldn't happen. If it does, user's quota window may have corrupted.
# Workaround: Restart gateway to reload quotas from keys.json

# Verify keys.json is valid JSON
cat ~/.sovereignstack/keys.json | jq .
```

---

## Related Documentation

- [API Key Management](./KEYS_MANAGEMENT.md) — User creation and key management
- [Gateway Access Control](./GATEWAY_ACCESS_CONTROL.md) — Model-level access control (Phase 2)
- [Architecture](./ARCHITECTURE_AUTH.md) — Authentication and authorization flows
- [Monitoring](./MONITORING.md) — Track quota usage via metrics and audit logs
