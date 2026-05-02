# API Key Management Guide

## Overview

The SovereignStack gateway uses API keys to authenticate requests, enforce per-user rate limits, control model access, and track token usage. Keys are managed via the `sovstack keys` CLI and stored in `~/.sovereignstack/keys.json`.

---

## Quick Start

### Create Your First User

```bash
# Add a user named 'alice'
sovstack keys add alice --department research --team nlp --role analyst

# Output:
# ✓ Created user "alice"
#   API Key: sk_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6
#   Rate Limit: 100 requests/min
```

### Use the API Key

```bash
curl -H "X-API-Key: sk_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6" \
     -d '{"model":"mistral-7b","messages":[]}' \
     http://localhost:8001/v1/chat/completions
```

### List All Users

```bash
sovstack keys list

# Output:
# USER ID    DEPARTMENT   ROLE     MODELS   DAILY QUOTA   MONTHLY QUOTA   LAST USED
# alice      research     analyst  0        unlimited     unlimited       never
# bob        operations   admin    0        unlimited     unlimited       never
```

---

## Commands

### `sovstack keys add <user-id>`

Create a new user with a randomly generated API key.

**Options:**
- `--department, -d` — Department name (optional)
- `--team, -t` — Team name (optional)
- `--role, -r` — User role (default: "user")
- `--rate-limit, -l` — Requests per minute (default: 100)

**Example:**
```bash
sovstack keys add alice \
  --department research \
  --team nlp \
  --role analyst \
  --rate-limit 200
```

**Output shows the generated key once** — save it securely. If lost, remove the user and create a new one.

---

### `sovstack keys list`

List all registered users without showing API keys.

**Output columns:**
- USER ID
- DEPARTMENT
- ROLE
- MODELS — Number of allowed models
- DAILY QUOTA — Token limit per day (or "unlimited")
- MONTHLY QUOTA — Token limit per month (or "unlimited")
- LAST USED — When the user's key was last used

---

### `sovstack keys info <user-id>`

Show detailed profile for a user.

**Example:**
```bash
sovstack keys info alice

# Output:
# User: alice
#   API Key: sk_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6
#   Department: research
#   Team: nlp
#   Role: analyst
#   Rate Limit: 200.0 requests/min
#   Daily Token Limit: unlimited
#   Monthly Token Limit: unlimited
#   Allowed Models: (none)
#   Created: 2026-04-30 10:15:32
#   Last Used: 2026-04-30 10:16:00
```

---

### `sovstack keys grant-model <user-id> <model>`

Allow a user to access a specific model.

**Example:**
```bash
sovstack keys grant-model alice mistral-7b
sovstack keys grant-model alice llama-3-8b

# User alice can now access both models
```

**Special value:**
- `sovstack keys grant-model bob "*"` — Allow user to access ALL models (useful for admins)

---

### `sovstack keys revoke-model <user-id> <model>`

Deny a user access to a model.

**Example:**
```bash
sovstack keys revoke-model alice llama-3-8b

# User alice can no longer access llama-3-8b
```

---

### `sovstack keys set-quota <user-id>`

Set daily and/or monthly token limits for a user.

**Options:**
- `--daily, -d` — Maximum tokens per day (0 = unlimited)
- `--monthly, -m` — Maximum tokens per month (0 = unlimited)

**Examples:**

```bash
# Limit alice to 500K tokens per day, 10M per month
sovstack keys set-quota alice --daily 500000 --monthly 10000000

# Limit bob to 100K per day, unlimited monthly
sovstack keys set-quota bob --daily 100000

# Remove daily limit but keep monthly (set to 0)
sovstack keys set-quota bob --daily 0
```

**When limit is exceeded:**
- Requests return `429 Too Many Requests` with error message
- User can try again after the reset (daily limit resets at UTC midnight, monthly on the 1st)

---

### `sovstack keys remove <user-id>`

Revoke a user's API key and delete their profile.

**Example:**
```bash
sovstack keys remove alice

# Output:
# ✓ Removed user "alice"
```

⚠️ **This is permanent.** The user cannot access the gateway anymore. To re-enable, use `sovstack keys add`.

---

### `sovstack keys usage <user-id>`

Show token quota status for a user. Requires gateway running with `--keys` flag.

**Example:**
```bash
sovstack keys usage alice

# Output:
# User: alice
#   Daily Limit: 500000 tokens
#   Monthly Limit: 10000000 tokens
#   
#   Note: Usage tracking requires gateway with metrics endpoint running.
#         Run: sovstack gateway --keys ~/.sovereignstack/keys.json
```

---

## Keys File Format

Keys are stored in `~/.sovereignstack/keys.json`. **Do not edit manually.** Always use the CLI.

Example `keys.json`:
```json
{
  "users": {
    "alice": {
      "id": "alice",
      "key": "sk_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6",
      "department": "research",
      "team": "nlp",
      "role": "analyst",
      "allowed_models": ["mistral-7b", "llama-3-8b"],
      "rate_limit_per_min": 200,
      "max_tokens_per_day": 500000,
      "max_tokens_per_month": 10000000,
      "created_at": "2026-04-30T10:15:32Z",
      "last_used_at": "2026-04-30T10:16:00Z"
    },
    "bob": {
      "id": "bob",
      "key": "sk_b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7",
      "department": "operations",
      "team": "infra",
      "role": "admin",
      "allowed_models": ["*"],
      "rate_limit_per_min": 1000,
      "max_tokens_per_day": 0,
      "max_tokens_per_month": 0,
      "created_at": "2026-04-30T09:00:00Z",
      "last_used_at": "2026-04-30T15:30:00Z"
    }
  }
}
```

---

## Starting the Gateway with Keys

### Option 1: Use Custom Keys File

```bash
sovstack gateway \
  --keys ~/.sovereignstack/keys.json \
  --backend http://localhost:8000 \
  --port 8001
```

### Option 2: Use Hardcoded Test Keys (Development Only)

```bash
sovstack gateway --backend http://localhost:8000 --port 8001
```

When using hardcoded keys, the gateway shows:
```
Example test keys (hardcoded for development):
  - sk_test_123 (test-user)
  - sk_demo_456 (demo-user)

To use keys.json, run: sovstack gateway --keys ~/.sovereignstack/keys.json
```

---

## Workflow Examples

### Scenario 1: Research Team Setup

```bash
# Create three research users
sovstack keys add alice --department research --team nlp --role analyst
sovstack keys add bob --department research --team nlp --role analyst
sovstack keys add charlie --department research --team nlp --role lead

# Allow alice and bob to use open models, charlie gets all
sovstack keys grant-model alice mistral-7b
sovstack keys grant-model alice llama-3-8b
sovstack keys grant-model bob mistral-7b
sovstack keys grant-model bob llama-3-8b
sovstack keys grant-model charlie "*"

# Set quotas: analysts get 500K tokens/day, lead gets unlimited
sovstack keys set-quota alice --daily 500000
sovstack keys set-quota bob --daily 500000

# Start gateway
sovstack gateway --keys ~/.sovereignstack/keys.json
```

### Scenario 2: Production Deployment

```bash
# Admin user with no limits
sovstack keys add admin --role admin
sovstack keys grant-model admin "*"

# Team users with budget limits (500K tokens/day each)
sovstack keys add team-a-user1 --department sales --rate-limit 200
sovstack keys set-quota team-a-user1 --daily 500000
sovstack keys grant-model team-a-user1 mistral-7b

sovstack keys add team-b-user1 --department marketing --rate-limit 100
sovstack keys set-quota team-b-user1 --daily 300000
sovstack keys grant-model team-b-user1 mistral-7b

# Start with auth enabled
sovstack gateway --keys ~/.sovereignstack/keys.json --port 8001 --audit-db ./audit.db
```

---

## Security Best Practices

1. **Protect `~/.sovereignstack/keys.json`**
   - File permissions are set to `0600` (readable only by owner)
   - Don't commit to version control
   - Back up securely

2. **Rotate Keys Periodically**
   - Remove old users: `sovstack keys remove <user-id>`
   - Create new users with new keys
   - Communicate changes to users

3. **Monitor Key Usage**
   - Check `Last Used` column in `sovstack keys list`
   - Revoke unused keys to reduce attack surface

4. **Use Rate Limits and Token Quotas**
   - Start with conservative defaults
   - Increase as needed based on usage patterns
   - Monitor quota approaches

5. **Audit Access**
   - Enable audit logging: `--audit-db ./audit.db`
   - Review `/api/v1/audit/logs` endpoint

---

## Troubleshooting

### "user already exists"
```bash
# User is already registered. Remove and re-add:
sovstack keys remove alice
sovstack keys add alice --department research
```

### "API key not working"
```bash
# Verify the exact key:
sovstack keys info alice

# Verify gateway started with correct keys file:
sovstack gateway --keys ~/.sovereignstack/keys.json

# Test with curl:
curl -H "X-API-Key: sk_..." http://localhost:8001/v1/models
```

### "Access denied to model"
```bash
# User needs explicit grant:
sovstack keys grant-model alice mistral-7b

# Or grant all models:
sovstack keys grant-model alice "*"
```

### "Rate limit exceeded (429)"
```bash
# User hit their per-minute request limit, wait a moment and retry.
# Check their limit:
sovstack keys info alice  # Shows "Rate Limit: X requests/min"

# Increase if needed:
sovstack keys add alice --rate-limit 500  # This updates the user
```

### Keys file corrupted
```bash
# If keys.json is damaged, restore from backup.
# If no backup, all users must be re-added:
rm ~/.sovereignstack/keys.json
sovstack keys add alice --department research
# ... re-add all users
```

---

## API Reference (for programmatic use)

The gateway has endpoints for audit logging:

### Get Audit Logs
```
GET /api/v1/audit/logs?n=100
```

Response:
```json
{
  "logs": [
    {
      "timestamp": "2026-04-30T10:15:32Z",
      "event_type": "request",
      "user": "alice",
      "model": "mistral-7b",
      "endpoint": "/v1/chat/completions",
      "status_code": 200,
      "tokens_used": 450,
      "duration_ms": 1250
    }
  ]
}
```

### Get Audit Stats
```
GET /api/v1/audit/stats
```

Response:
```json
{
  "total_logs": 150,
  "total_requests": 145,
  "total_errors": 5,
  "total_tokens_used": 67500,
  "unique_users": 3,
  "unique_models": 2
}
```

---

## Migration from Hardcoded Keys

If you're currently using the hardcoded test keys (`sk_test_123`, `sk_demo_456`):

### Step 1: Create New Users
```bash
sovstack keys add alice --department research
sovstack keys add bob --department operations
```

### Step 2: Grant Models (if using multi-model routing)
```bash
sovstack keys grant-model alice "*"
sovstack keys grant-model bob "*"
```

### Step 3: Update Scripts/Configs
Replace:
```bash
curl -H "X-API-Key: sk_test_123" ...
```

With:
```bash
curl -H "X-API-Key: sk_a1b2c3d4..." ...
```

### Step 4: Start Gateway with Keys File
```bash
# Old:
sovstack gateway --backend http://localhost:8000

# New:
sovstack gateway --keys ~/.sovereignstack/keys.json --backend http://localhost:8000
```

---

## Related Topics

- [Multi-Model Routing](./GATEWAY_ROUTING.md) — How models are routed in Phase 3
- [Access Control](./GATEWAY_ACCESS_CONTROL.md) — Model-level access policies (Phase 2)
- [Token Quotas](./TOKEN_QUOTAS.md) — Per-user token budget enforcement (Phase 2b)
- [Prometheus Metrics](./GATEWAY_METRICS.md) — Gateway metrics and monitoring (Phase 4)
