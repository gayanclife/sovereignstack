# Hybrid Model Registry Implementation

## Overview

SovereignStack now supports dynamic model discovery through a hybrid local + remote registry system. This allows users to:

✅ Work offline with bundled models  
✅ Automatically fetch latest models when internet available  
✅ Cache remote models locally for fast lookups  
✅ Fall back gracefully if remote unavailable  
✅ Use custom registries for organization-specific models  

---

## Architecture

### Components

#### 1. **Local Registry** (`models.yaml`)
- Bundled with SovereignStack binary
- Always available, offline-first
- Community-tested models
- Updated with releases

#### 2. **Remote Registry** (Optional)
- Default: `https://models.sovereignstack.io/registry.yaml`
- Automatically fetched when internet available
- Cached locally for 24 hours
- Can be customized per-command with `--registry` flag

#### 3. **Model Cache** (`~/.sovereignstack/models-remote.json`)
- Stores remote models locally
- 24-hour TTL (cache expiration)
- Enables offline operation after first fetch
- Can be cleared with `models clear-cache`

#### 4. **User Config** (`~/.sovereignstack/models.yaml`)
- Highest priority
- Allows custom/private models
- Overrides all other sources

---

## Code Implementation

### New Files

**`core/model/remote_registry.go`** (249 lines)
```go
type RemoteRegistry struct {
    URL       string        // Registry URL
    CacheDir  string        // Local cache directory
    CacheTTL  time.Duration // Cache validity (24h)
    Client    *http.Client  // HTTP client
    cacheFile string        // Cache file path
}
```

Key functions:
- `FetchAndCache()` — Fetch with fallback to cache
- `FetchOnly()` — Fetch without fallback (for `--refresh`)
- `MergeRegistries()` — Combine local + remote
- `FilterByHardware()` — Filter by GPU/CPU/RAM

**`cmd/models.go`** (180 lines)
```go
var modelsCmd          // Main models command
var modelsListCmd      // List compatible models
var modelsRefreshCmd   // Refresh remote cache
var modelsClearCacheCmd // Clear cache
```

Commands:
- `sovstack models list` — Show compatible models
- `sovstack models refresh` — Fetch fresh models
- `sovstack models clear-cache` — Remove cache

---

## Data Flow

### List Models (Default Behavior)

```
User runs: sovstack models list
    │
    ├─→ Detect hardware (GPU count, VRAM, RAM)
    │
    ├─→ Load local models from models.yaml
    │
    ├─→ Try to fetch remote models
    │   ├─→ Check internet connectivity
    │   ├─→ HTTP GET to registry URL
    │   ├─→ Parse response as YAML
    │   ├─→ Save to local cache if success
    │   └─→ Fall back to cache if failure
    │
    ├─→ Merge local + remote (remote overrides)
    │
    ├─→ Filter by hardware compatibility
    │   ├─→ GPU systems: show GPU + hybrid models
    │   ├─→ CPU systems: show CPU + hybrid models
    │   └─→ Apply VRAM/RAM filters
    │
    └─→ Display results with download commands
```

### Refresh Models (Manual Update)

```
User runs: sovstack models refresh
    │
    ├─→ Force fetch from remote (no cache fallback)
    │
    ├─→ Update local cache
    │
    └─→ Print success message
```

---

## Fallback Strategy

**Offline-First Design:**

```
Level 1: Remote (newest) ✓ if online, not expired
         ↓ fail
Level 2: Cache (cached remote)
         ↓ unavailable
Level 3: Local (bundled)
         ↓ not found
Level 4: Error message

Result: Always works except Level 4 (truly no models)
```

---

## Usage Examples

### Discover Compatible Models

```bash
./sovstack models list
```

Shows:
- Your hardware specs
- 3 compatible CPU models for CPU-only system
- 6+ models for GPU system (filtered by VRAM)

### See All Models (Planning Upgrades)

```bash
./sovstack models list --all
```

Useful for:
- Checking what's available for future GPU upgrades
- Finding larger models to aim for

### Use Custom Registry

```bash
# Organization-specific models
./sovstack models list --registry https://internal-registry.company.com/models.yaml

# Refresh from custom registry
./sovstack models refresh --registry https://internal-registry.company.com/models.yaml
```

### Offline Setup

1. Machine with internet:
   ```bash
   sovstack models refresh  # Cache latest models
   ```

2. Transfer cache to offline machine:
   ```bash
   cp ~/.sovereignstack/models-remote.json /target/
   ```

3. Offline machine:
   ```bash
   ./sovstack models list  # Uses cached models
   ```

---

## Registry File Format

Both local and remote registries use the same YAML format:

```yaml
gpu_models:
  - name: meta-llama/Llama-3.1-8B-Instruct
    repo: meta-llama/Llama-3.1-8B-Instruct
    description: "Meta Llama 3.1 8B..."
    parameters: "8B"
    context_length: 131072
    hardware_target: gpu
    sizes:
      none: 16000000000
      int8: 8000000000
      awq: 4500000000
    required_vram_gb:
      none: 16
      int8: 9
      awq: 5
    default_quantization: awq

cpu_models:
  - name: TinyLlama/TinyLlama-1.1B
    ...
```

---

## Caching Mechanism

### Cache Format

```json
{
  "timestamp": "2026-04-27T10:30:00Z",
  "models": {
    "meta-llama/Llama-3.1-8B-Instruct": {
      "name": "meta-llama/Llama-3.1-8B-Instruct",
      "repo": "meta-llama/Llama-3.1-8B-Instruct",
      ...
    }
  }
}
```

### Cache Location

```
~/.sovereignstack/models-remote.json
```

### Cache Validity

- **Default TTL:** 24 hours
- **Check on:** `models list` or `models refresh`
- **Clear with:** `models clear-cache`
- **Manual clear:** `rm ~/.sovereignstack/models-remote.json`

---

## Hardware Filtering

Models are automatically filtered based on hardware detection:

### GPU Systems

```
Has NVIDIA GPU?
  ├─→ YES: Show GPU-only + GPU/CPU hybrid models
  │        Filter by: model.required_vram ≤ available_vram
  │
  └─→ NO: Show CPU-optimized + CPU/GPU hybrid models
           Filter by: model.minimum_system_ram ≤ available_ram
```

### Quantization Filtering

When checking VRAM fit, uses the most aggressive (smallest) quantization:

```go
// Model fits if ANY quantization fits in available VRAM
minVRAM := min(model.required_vram["awq"], 
               model.required_vram["int8"],
               model.required_vram["fp16"])
fits := minVRAM ≤ available_vram
```

---

## Remote Registry Implementation Notes

### Default Registry URL

```
https://models.sovereignstack.io/registry.yaml
```

**Setup Required:** This URL does not exist yet. To enable remote model discovery:

1. **Create Registry API** (Node.js, Python, Go)
   - Serves YAML with all latest models
   - Validates models work on common hardware
   - Updates regularly

2. **Host at:** `models.sovereignstack.io` or custom URL

3. **Format:** Same YAML as local `models.yaml`

### Example: Simple Static Registry

```bash
# Host a static file
aws s3 cp models.yaml s3://models.sovereignstack.io/registry.yaml --public-read

# Or use GitHub Pages
# Push models.yaml to gh-pages branch
```

### Example: Dynamic Registry API

```python
# FastAPI endpoint
@app.get("/registry.yaml")
async def get_models():
    models = fetch_latest_from_huggingface()
    validated = validate_on_test_hardware()
    return validated_models.to_yaml()
```

---

## Configuration Options

### Global Registry Override

Set custom registry for all commands:

```bash
export SOVEREIGNSTACK_REGISTRY=https://my-registry.io/models.yaml
./sovstack models list
```

*Note: Currently not implemented, would require env var support*

### Per-Command Override

```bash
./sovstack models list --registry https://custom.io/models.yaml
```

### Disable Remote (Use Local Only)

```bash
./sovstack models list --registry disabled
```

*Note: Currently not implemented*

---

## Testing

### Test Local Models

```bash
./sovstack models list
# Shows CPU models (no GPU detected)

./sovstack models list --all
# Shows all including GPU models
```

### Test Remote Fallback

```bash
# Works offline (uses cache + local)
./sovstack models list

# Fails gracefully if remote unavailable
./sovstack models refresh
# ❌ Failed to refresh models: ...
```

---

## Future Enhancements

1. **Model Metadata:** Add tags, benchmarks, license info to filter by
2. **Search:** `sovstack models search --keyword "chat"`
3. **Ratings:** Show community ratings/reviews per model
4. **Hardware Benchmarks:** "This model gets 45 tokens/sec on RTX 4090"
5. **Dynamic Remote URL:** Config file for registry URL
6. **Model Packages:** Bundle models + config for specific use cases
7. **Registry Sync:** Keep local and remote in sync automatically

---

## Files Changed

### New Files
- `core/model/remote_registry.go` — Remote registry implementation
- `cmd/models.go` — CLI commands
- `docs/MODEL_DISCOVERY.md` — User documentation

### Modified Files
- `CLAUDE.md` — Added remote registry to architecture
- `docs/README.md` — Added link to model discovery guide
- `docs/COMMANDS.md` — Added `models` command reference

### No Changes Needed
- `models.yaml` — Compatible with both old and new code
- Other core files — Backward compatible

---

## Summary

The hybrid registry system brings:

✅ **Offline-first** — Always works without internet  
✅ **Modern models** — Remote fetch keeps list current  
✅ **Flexible** — Custom registries per organization  
✅ **Reliable** — Fallback strategy ensures availability  
✅ **Simple** — Single `models list` command does everything  

Users get the best of both worlds: reliability of bundled models + freshness of remote registries.
