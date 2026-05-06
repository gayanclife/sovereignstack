# Audit Logging

SovereignStack provides comprehensive audit logging for compliance and security monitoring. The gateway logs all API requests, responses, and errors with optional encryption and persistence.

## Overview

The audit system tracks:
- **Requests**: API calls with method, endpoint, user, model, size
- **Responses**: Success responses with status, tokens used, latency
- **Errors**: Failed requests with error messages and status codes
- **Auth failures**: Invalid API key attempts with IP address

## Audit Logger Types

### In-Memory Logger (Default)

Keeps the last N logs in memory. Logs are lost on gateway restart.

**Use case**: Development, testing, or when persistence is not required.

```bash
./sovstack gateway --audit-buffer 10000
```

- Default buffer size: 10,000 logs
- Logs are lost on restart
- No encryption or storage overhead

### SQLite Logger (Encrypted)

Persists logs to an encrypted SQLite database. Sensitive fields (IP address, user agent, error messages) are encrypted with AES-256-GCM.

**Use case**: Production, compliance, and regulated industries requiring audit trails.

```bash
./sovstack gateway \
  --audit-db ./sovstack-audit.db \
  --audit-key $(echo $SOVSTACK_AUDIT_KEY)
```

## Encryption

The SQLite logger encrypts sensitive fields to protect personally identifiable information (PII):

- **Encrypted fields**: `ip_address`, `user_agent`, `error_message`
- **Plaintext fields**: `timestamp`, `user`, `model`, `endpoint`, `status_code`, `tokens_used`, `duration_ms` (preserved for queryability)
- **Encryption method**: AES-256-GCM
- **Key derivation**: PBKDF2-SHA256 (100,000 iterations)
- **Salt**: Auto-generated per database, stored in SQLite `config` table

### Key Management

#### Setting an Encryption Key

Provide an encryption key via environment variable or flag:

```bash
# Set environment variable
export SOVSTACK_AUDIT_KEY="your-32-byte-hex-key"
./sovstack gateway

# Or via flag
./sovstack gateway --audit-key "your-32-byte-hex-key"
```

#### Auto-Generated Keys

If no key is provided, the gateway generates a random 32-byte key and prints it:

```bash
./sovstack gateway --audit-db ./sovstack-audit.db

# Output:
# ⚠️  Generated audit encryption key (save this for future restarts):
#    SOVSTACK_AUDIT_KEY=a1b2c3d4e5f6...
```

**Important**: Save the printed key to a secure location. You'll need it to re-open the database later.

## Configuration

### Gateway Flags

```bash
./sovstack gateway \
  --audit-db ./sovstack-audit.db \        # SQLite database path (empty = in-memory)
  --audit-key $SOVSTACK_AUDIT_KEY \        # Encryption key (or via env var)
  --audit-buffer 10000 \                   # Buffer size for in-memory logger
  --backend http://localhost:8000 \        # Backend service URL
  --port 8001 \                            # Gateway port
  --rate-limit 100                         # Requests per minute per user
```

### Environment Variables

```bash
export SOVSTACK_AUDIT_KEY="a1b2c3d4e5f6..."  # Encryption key
```

## Querying Logs

### Get Recent Logs

```bash
curl http://localhost:8001/api/v1/audit/logs?n=100
```

Response:
```json
{
  "logs": [
    {
      "timestamp": "2026-04-28T15:04:05Z",
      "event_type": "request",
      "user": "alice",
      "model": "llama-7b",
      "endpoint": "/v1/chat/completions",
      "status_code": 200,
      "tokens_used": 150,
      "duration_ms": 425
    }
  ]
}
```

### Get Audit Statistics

```bash
curl http://localhost:8001/api/v1/audit/stats
```

Response:
```json
{
  "total_logs": 1253,
  "total_requests": 1200,
  "total_errors": 15,
  "total_tokens_used": 185000,
  "unique_users": 8,
  "unique_models": 3
}
```

## Database Access

To directly inspect the SQLite audit database:

```bash
# View table schema
sqlite3 sovstack-audit.db ".schema audit_logs"

# Count logs
sqlite3 sovstack-audit.db "SELECT COUNT(*) FROM audit_logs"

# View plaintext fields (encrypted fields appear as base64)
sqlite3 sovstack-audit.db \
  "SELECT timestamp, user, model, status_code FROM audit_logs LIMIT 10"
```

## Best Practices

1. **Use SQLite for production**: Always use encrypted SQLite audit logs in production environments.

2. **Secure your encryption key**: Store the `SOVSTACK_AUDIT_KEY` in a secrets manager (e.g., HashiCorp Vault, AWS Secrets Manager).

3. **Rotate keys periodically**: Implement a key rotation policy (e.g., every 90 days).

4. **Back up your database**: Regularly back up `sovstack-audit.db` to a secure location.

5. **Monitor audit log size**: The database grows ~500 bytes per logged request. Monitor disk space usage.

6. **Retention policy**: Implement a retention policy to archive or delete old logs:
   ```bash
   sqlite3 sovstack-audit.db \
     "DELETE FROM audit_logs WHERE timestamp < datetime('now', '-90 days')"
   ```

## Log Fields

Each audit log entry contains:

| Field | Type | Encrypted | Description |
|-------|------|-----------|-------------|
| `timestamp` | DateTime | No | When the event occurred |
| `event_type` | String | No | Type: request, response, error, auth_failure |
| `level` | String | No | Severity: info, warn, error |
| `user` | String | No | API key owner or user ID |
| `model` | String | No | Model name that was accessed |
| `method` | String | No | HTTP method (GET, POST, etc.) |
| `endpoint` | String | No | API endpoint path |
| `request_size` | Integer | No | Request body size in bytes |
| `response_size` | Integer | No | Response body size in bytes |
| `tokens_used` | Integer | No | Input tokens (for text generation) |
| `tokens_generated` | Integer | No | Output tokens (for text generation) |
| `duration_ms` | Integer | No | Request latency in milliseconds |
| `status_code` | Integer | No | HTTP status code |
| `error_message` | String | **Yes** | Error message or reason for failure |
| `ip_address` | String | **Yes** | Client IP address |
| `user_agent` | String | **Yes** | Client user agent string |
| `correlation_id` | String | No | Trace ID for linking related requests |

## Compliance

The audit system supports compliance requirements for:

- **SOC 2**: Comprehensive logging of API access and changes
- **HIPAA**: Encryption of sensitive data (IP addresses, user agents)
- **PCI DSS**: Tamper-evident logs with encryption
- **GDPR**: Audit trails of personal data processing

## Testing

Run the audit tests:

```bash
go test ./core/audit/...
```

Tests cover:
- Persistence (logs survive database reopening)
- Encryption (sensitive fields are encrypted, not plaintext)
- Query methods (GetLogsByUser, GetLogsByModel, GetLogsInTimeRange)
- Statistics accuracy
- Key derivation with PBKDF2
- Wrong key handling (decryption fails gracefully)
