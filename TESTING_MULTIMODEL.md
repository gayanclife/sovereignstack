# Multi-Model Testing Guide

## Quick Start

### Run All Tests
```bash
go test ./core/engine -v
```

**Expected Output:** 10 tests passing (~0.2s)

---

## Test Coverage

### Unit Tests (orchestrator_test.go)
- ✅ Map initialization
- ✅ Get running models when empty
- ✅ Stop non-existent model error handling
- ✅ Status structure verification
- ✅ System info retrieval

### Integration Tests (integration_test.go)
- ✅ Multi-model lifecycle simulation
- ✅ Engine map concurrency isolation
- ✅ Status structure with actual values
- ✅ Quantization type handling
- ✅ Error scenarios

---

## Manual Testing (With Docker)

### Prerequisites
- Docker installed and running
- A CPU-optimized model downloaded: `./sovstack pull distilbert-base-uncased`

### Test 1: Deploy Single Model

```bash
# Deploy one model
./sovstack deploy distilbert-base-uncased

# Expected output:
# ✅ Model deployed successfully!
#   Check status: sovstack status
#   Stop model:   sovstack stop distilbert-base-uncased
```

### Test 2: Check Running Model

```bash
./sovstack status

# Expected output shows:
# 🚀 Running Models (1)
#   • distilbert-base-uncased (started: HH:MM:SS)
#     Container ID: abc123def456
#     Quantization: none
#     Health: true
```

### Test 3: Deploy Second Model (If available)

```bash
# Pull another model first (if CPU allows)
./sovstack pull gpt2

# Deploy it alongside the first
./sovstack deploy gpt2

# Check status again
./sovstack status

# Expected output shows:
# 🚀 Running Models (2)
#   • distilbert-base-uncased (started: HH:MM:SS)
#   • gpt2 (started: HH:MM:SS)
```

### Test 4: Stop Specific Model

```bash
# Stop one model while other runs
./sovstack stop distilbert-base-uncased

# Expected output:
# ✓ Model distilbert-base-uncased stopped

# Check status
./sovstack status

# Expected: Only gpt2 remains in running models
```

### Test 5: Stop All Models

```bash
# Stop everything
./sovstack stop

# Expected output:
# Stopping all N running model(s)...
# ✓ Stopped gpt2
# ✓ Stopped <others>
```

### Test 6: Prevent Duplicate Deployments

```bash
# Try to deploy same model twice
./sovstack deploy distilbert-base-uncased
./sovstack deploy distilbert-base-uncased

# Expected: Second call fails with error
# ✗ Deployment failed: model distilbert-base-uncased is already running
```

---

## Automated Test Commands

```bash
# Run all tests with verbose output
go test ./core/engine -v

# Run specific test
go test ./core/engine -run TestMultiModelLifecycle -v

# Run tests with coverage
go test ./core/engine -cover

# Run tests and show memory usage
go test ./core/engine -v -race

# Run all tests in project
go test ./... -v
```

---

## Test Scenarios Matrix

| Scenario | Command | Expected Result |
|----------|---------|-----------------|
| Empty state | `go test -run TestEngineRoomMultiModel` | Maps initialized |
| Multiple models | Deploy model1, deploy model2 | Both in status |
| Stop specific | `sovstack stop model1` | Only model1 stops |
| Stop all | `sovstack stop` | All stopped |
| Duplicate deploy | Deploy same model twice | Error on 2nd |
| Status on empty | `sovstack status` | "Running Models: None" |

---

## Verification Checklist

After running tests, verify:

- [ ] Build compiles: `go build -o sovstack .`
- [ ] All unit tests pass: `go test ./core/engine`
- [ ] Help shows stop command: `./sovstack --help | grep stop`
- [ ] Status command works: `./sovstack status`
- [ ] Stop command works: `./sovstack stop --help`
- [ ] No panics on empty state
- [ ] Error handling for non-existent models
- [ ] Maps properly initialized in EngineRoom

---

## Debugging

### Enable Debug Output
```bash
# Run with verbose logging
GOVVV=1 go test ./core/engine -v

# Check map contents
go test ./core/engine -run TestEngineMapConcurrency -v
```

### Inspect EngineRoom State
Add this to a test to inspect state:
```go
t.Logf("runningModels: %v", len(er.runningModels))
t.Logf("engines: %v", len(er.engines))
t.Logf("status: %+v", er.Status(ctx))
```

---

## Performance Testing

```bash
# Benchmark map operations
go test ./core/engine -bench=. -benchmem

# Check for goroutine leaks
go test ./core/engine -race -v
```

---

## Files Tested

1. `core/engine/orchestrator.go` — Core multi-model logic
2. `core/engine/orchestrator_test.go` — Unit tests
3. `core/engine/integration_test.go` — Integration tests
4. `cmd/deploy.go` — Deploy command
5. `cmd/status.go` — Status command
6. `cmd/stop.go` — Stop command

---

## Continuous Integration

To run in CI/CD:

```bash
#!/bin/bash
set -e

echo "Building..."
go build -o sovstack .

echo "Running tests..."
go test ./core/engine -v -race -coverprofile=coverage.out

echo "Checking coverage..."
go tool cover -html=coverage.out

echo "✓ All tests passed"
```
