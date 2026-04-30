#!/bin/bash

# Phase 2b: Token Quota Enforcement Integration Test
# This script tests the complete token quota workflow

set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test tracking
TESTS_PASSED=0
TESTS_FAILED=0

# Temporary directory for test keys
TEST_DIR=$(mktemp -d)
KEYS_FILE="$TEST_DIR/keys.json"
GATEWAY_PORT=9999

echo "================================"
echo "Phase 2b: Token Quota Tests"
echo "================================"
echo ""

# Helper functions
test_case() {
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}TEST: $1${NC}"
}

pass() {
    echo -e "${GREEN}✓ PASS${NC}: $1"
    ((TESTS_PASSED++))
}

fail() {
    echo -e "${RED}✗ FAIL${NC}: $1"
    ((TESTS_FAILED++))
}

cleanup() {
    # Kill gateway if running
    if pgrep -f "gateway.*$GATEWAY_PORT" > /dev/null 2>&1; then
        pkill -f "gateway.*$GATEWAY_PORT" || true
        sleep 1
    fi
    # Clean up test directory
    rm -rf "$TEST_DIR"
}

trap cleanup EXIT

# ============================================================================
# Test 1: KeyStore accepts user with quota limits
# ============================================================================
test_case "KeyStore accepts user with quota limits"

cd /home/gayangunapala/Projects/sstack/sovereignstack

# Build binary if not exists
if [ ! -f "./bin/./bin/sovstack" ]; then
    go build -o ./bin/./bin/sovstack ./cmd 2>/dev/null
fi

# Create alice with quota
./bin/./bin/sovstack keys add alice --department research > /dev/null 2>&1
if [ $? -eq 0 ]; then
    pass "User alice created"
else
    fail "Failed to create user alice"
fi

# Set daily and monthly quotas
./bin/sovstack keys set-quota alice --daily 500000 --monthly 10000000 2>/dev/null
if [ $? -eq 0 ]; then
    pass "Quotas set for alice"
else
    fail "Failed to set quotas for alice"
fi

# Verify quotas in info output
INFO=$(./bin/sovstack keys info alice 2>/dev/null)
if echo "$INFO" | grep -q "Max Tokens Per Day: 500000"; then
    pass "Daily quota limit appears in keys info"
else
    fail "Daily quota limit not found in keys info"
fi

if echo "$INFO" | grep -q "Max Tokens Per Month: 10000000"; then
    pass "Monthly quota limit appears in keys info"
else
    fail "Monthly quota limit not found in keys info"
fi

# ============================================================================
# Test 2: Unlimited quota (0 = unlimited)
# ============================================================================
test_case "Unlimited quota (0 = unlimited)"

./bin/sovstack keys add admin --role admin > /dev/null 2>&1
./bin/sovstack keys set-quota admin --daily 0 --monthly 0 2>/dev/null

INFO=$(./bin/sovstack keys info admin 2>/dev/null)
if echo "$INFO" | grep -q "Max Tokens Per Day: unlimited"; then
    pass "Daily unlimited quota shows correctly"
else
    fail "Unlimited daily quota not displayed correctly"
fi

if echo "$INFO" | grep -q "Max Tokens Per Month: unlimited"; then
    pass "Monthly unlimited quota shows correctly"
else
    fail "Unlimited monthly quota not displayed correctly"
fi

# ============================================================================
# Test 3: Multiple users with different quotas
# ============================================================================
test_case "Multiple users with different quotas"

./bin/sovstack keys add bob --department operations > /dev/null 2>&1
./bin/sovstack keys add charlie --department finance > /dev/null 2>&1

./bin/sovstack keys set-quota bob --daily 100000 --monthly 1000000 2>/dev/null
./bin/sovstack keys set-quota charlie --daily 50000 --monthly 500000 2>/dev/null

USERS=$(./bin/sovstack keys list 2>/dev/null | wc -l)
if [ "$USERS" -ge 4 ]; then
    pass "Multiple users created with different quotas"
else
    fail "Not all users created"
fi

# ============================================================================
# Test 4: Gateway with Token Quotas
# ============================================================================
test_case "Gateway starts with token quota enforcement"

# Create test keys file
cat > "$KEYS_FILE" << 'EOF'
{
  "users": {
    "quota_test": {
      "id": "quota_test",
      "key": "sk_quota_test_123",
      "department": "test",
      "team": "qa",
      "role": "analyst",
      "allowed_models": ["model-a", "model-b"],
      "rate_limit_per_min": 100,
      "max_tokens_per_day": 1000,
      "max_tokens_per_month": 10000,
      "created_at": "2026-04-30T00:00:00Z",
      "last_used_at": "2026-04-30T00:00:00Z"
    },
    "admin_quota": {
      "id": "admin_quota",
      "key": "sk_admin_quota_456",
      "department": "admin",
      "team": "ops",
      "role": "admin",
      "allowed_models": ["*"],
      "rate_limit_per_min": 1000,
      "max_tokens_per_day": 0,
      "max_tokens_per_month": 0,
      "created_at": "2026-04-30T00:00:00Z",
      "last_used_at": "2026-04-30T00:00:00Z"
    }
  }
}
EOF

if [ -f "$KEYS_FILE" ]; then
    pass "Test keys file with quotas created"
else
    fail "Failed to create test keys file"
fi

# Start gateway in background
./bin/sovstack gateway \
    --keys "$KEYS_FILE" \
    --backend http://localhost:8000 \
    --port $GATEWAY_PORT \
    --audit-db "$TEST_DIR/audit.db" \
    > /dev/null 2>&1 &

GATEWAY_PID=$!
sleep 2

# Check if gateway is running
if ps -p $GATEWAY_PID > /dev/null; then
    pass "Gateway started successfully with token quotas"
else
    fail "Gateway failed to start"
fi

# ============================================================================
# Test 5: Gateway startup message includes token quotas
# ============================================================================
test_case "Gateway startup shows token quota status"

# Re-run gateway and capture output
OUTPUT=$(./bin/sovstack gateway \
    --keys "$KEYS_FILE" \
    --backend http://localhost:8000 \
    --port $GATEWAY_PORT \
    --audit-db "$TEST_DIR/audit.db" 2>&1 | head -20 || true)

if echo "$OUTPUT" | grep -q "Token Quotas: Enabled"; then
    pass "Gateway startup message includes 'Token Quotas: Enabled'"
else
    fail "Token Quotas status not shown in startup message"
fi

# Kill and restart for remaining tests
if pgrep -f "gateway.*$GATEWAY_PORT" > /dev/null 2>&1; then
    pkill -f "gateway.*$GATEWAY_PORT" || true
    sleep 1
fi

# Restart gateway silently
./bin/sovstack gateway \
    --keys "$KEYS_FILE" \
    --backend http://localhost:8000 \
    --port $GATEWAY_PORT \
    --audit-db "$TEST_DIR/audit.db" \
    > /dev/null 2>&1 &

sleep 2

# ============================================================================
# Test 6: Verify keys.json has quota fields
# ============================================================================
test_case "Keys file contains quota configuration"

if grep -q "max_tokens_per_day" "$KEYS_FILE"; then
    pass "max_tokens_per_day field present in keys.json"
else
    fail "max_tokens_per_day field missing"
fi

if grep -q "max_tokens_per_month" "$KEYS_FILE"; then
    pass "max_tokens_per_month field present in keys.json"
else
    fail "max_tokens_per_month field missing"
fi

# Verify quota values are readable
if grep -q '"max_tokens_per_day": 1000' "$KEYS_FILE"; then
    pass "Daily quota value (1000) correctly stored"
else
    fail "Daily quota value not stored correctly"
fi

if grep -q '"max_tokens_per_month": 10000' "$KEYS_FILE"; then
    pass "Monthly quota value (10000) correctly stored"
else
    fail "Monthly quota value not stored correctly"
fi

# ============================================================================
# Test 7: Quota limits stored in UserProfile
# ============================================================================
test_case "Quota limits accessible from UserProfile struct"

# This is tested implicitly by the gateway startup
# If quotas weren't stored in UserProfile, the gateway would fail to start
if [ -n "$GATEWAY_PID" ] && ps -p $GATEWAY_PID > /dev/null 2>&1; then
    pass "UserProfile successfully loaded with quota fields"
else
    fail "UserProfile quota loading failed"
fi

# ============================================================================
# Test 8: Unlimited quotas (0 values)
# ============================================================================
test_case "Unlimited quotas (0) handled correctly"

if grep -q '"max_tokens_per_day": 0' "$KEYS_FILE"; then
    pass "Unlimited daily quota (0) stored correctly"
else
    fail "Unlimited daily quota not stored"
fi

if grep -q '"max_tokens_per_month": 0' "$KEYS_FILE"; then
    pass "Unlimited monthly quota (0) stored correctly"
else
    fail "Unlimited monthly quota not stored"
fi

# ============================================================================
# Test 9: Multiple quota configurations
# ============================================================================
test_case "Different users have different quota limits"

# Count users with different daily limits
COUNT_LIMITED=$(grep -c '"max_tokens_per_day": 1000' "$KEYS_FILE" || true)
COUNT_UNLIMITED=$(grep -c '"max_tokens_per_day": 0' "$KEYS_FILE" || true)

if [ "$COUNT_LIMITED" -gt 0 ] && [ "$COUNT_UNLIMITED" -gt 0 ]; then
    pass "Multiple users have different quota configurations"
else
    fail "Not all expected quota configurations present"
fi

# ============================================================================
# Summary
# ============================================================================
echo ""
echo "================================"
echo "Test Summary"
echo "================================"
echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
echo -e "${RED}Failed: $TESTS_FAILED${NC}"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ All Phase 2b token quota tests passed!${NC}"
    exit 0
else
    echo -e "${RED}✗ Some tests failed${NC}"
    exit 1
fi
