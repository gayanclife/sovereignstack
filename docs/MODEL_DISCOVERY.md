# Dynamic Model Discovery

SovereignStack uses a hybrid model discovery system that combines local and remote registries to give you the latest models while maintaining offline capability.

## How It Works

### Local Registry (Always Available)
Every SovereignStack installation includes a bundled `models.yaml` with community-tested models. This ensures the CLI works offline and you always have a set of known-good models.

### Remote Registry (Optional, Cached)
When internet is available, SovereignStack can fetch the latest models from a remote registry and cache them locally for 24 hours. This keeps your model list fresh without requiring online access every time.

### Fallback Strategy
1. **Try remote** → if available and newer, use it
2. **Fall back to cache** → if remote fails, use cached remote models
3. **Use local** → if no cache, use bundled local models

Result: **Always works, always useful.**

---

## Discovery Commands

### List Compatible Models

Show models that work with your hardware:

```bash
./sovstack models list
```

Output shows:
- Your hardware specs (GPU/CPU, VRAM, RAM)
- Compatible models with parameter count and context length
- Minimum VRAM/RAM requirements
- Download commands

### Show All Models

See all available models (including incompatible ones):

```bash
./sovstack models list --all
```

Useful for checking if a larger GPU model exists that you might want to work toward.

### Filter by Hardware Type

```bash
# Show only GPU models
./sovstack models list --gpu

# Show only CPU models
./sovstack models list --cpu
```

### Refresh From Remote Registry

Manually fetch latest models from remote (requires internet):

```bash
./sovstack models refresh
```

This updates the local cache and shows results:
```
✓ Successfully fetched 42 models from remote registry
  Cached at: ~/.sovereignstack/models-remote.json
  Cache expires in 24 hours
```

### Clear Cache

Remove cached remote models (will re-fetch on next refresh):

```bash
./sovstack models clear-cache
```

---

## Model Sources

### Local Registry (`models.yaml`)
- Bundled with SovereignStack binary
- Community-maintained
- Tested on common hardware
- Updated with new SovereignStack releases

### Remote Registry
- URL: `https://models.sovereignstack.io/registry.yaml` (default)
- Override with: `./sovstack models list --registry https://custom-registry.io/models.yaml`
- Contains latest models from Hugging Face
- Automatically cached for 24 hours

### User Config
- Path: `~/.sovereignstack/models.yaml`
- Highest priority (overrides all other sources)
- Useful for adding custom/private models

### Project Config
- Path: `./models.local.yaml` (in your SovereignStack directory)
- Git-ignored, for local customization

---

## Example Workflows

### Discover CPU-Only Models

You want to test without a GPU:

```bash
./sovstack models list
# Shows TinyLlama, Phi-3 Mini, etc.

./sovstack pull TinyLlama/TinyLlama-1.1B-Chat-v1.0
./sovstack up TinyLlama/TinyLlama-1.1B-Chat-v1.0
```

### Find What Your GPU Can Run

```bash
./sovstack models list
# Automatically filtered by your GPU's VRAM
# Shows all models that fit

./sovstack pull mistralai/Mistral-7B-Instruct-v0.3
./sovstack up mistralai/Mistral-7B-Instruct-v0.3
```

### Use Custom Model Registry

Point to your organization's internal registry:

```bash
./sovstack models list --registry https://internal-registry.company.com/models.yaml
./sovstack models refresh --registry https://internal-registry.company.com/models.yaml
```

### Offline First Setup

1. On a machine with internet, refresh models:
   ```bash
   ./sovstack models refresh
   ```

2. Transfer to offline environment (cache is in `~/.sovereignstack/models-remote.json`)

3. Use models list offline:
   ```bash
   ./sovstack models list
   # Uses cached remote + local models
   ```

---

## Filtering Logic

Models are automatically filtered based on your hardware:

### GPU Systems
- Shows: GPU-only models + GPU/CPU hybrid models
- Hides: CPU-only models (not optimized for GPU)
- Filter: Models with minimum VRAM ≤ available VRAM

### CPU-Only Systems
- Shows: CPU-optimized models + GPU/CPU hybrid models
- Hides: GPU-only models (won't work without NVIDIA drivers)
- Filter: Models with minimum RAM ≤ available RAM

### Override Filtering
Use `--all` to see everything (useful for planning upgrades):

```bash
./sovstack models list --all
# Shows GPU models even on CPU system
# Lets you see what becomes available with GPU upgrade
```

---

## Architecture

```
┌─────────────────────────────────────┐
│  sovstack models list               │
├─────────────────────────────────────┤
│                                     │
│  ┌──────────────────────────────┐   │
│  │ Detect Hardware              │   │
│  │ (GPUs, VRAM, RAM)            │   │
│  └───────────┬──────────────────┘   │
│              │                       │
│  ┌───────────▼──────────────────┐   │
│  │ Load Local Models            │   │
│  │ (bundled models.yaml)        │   │
│  └───────────┬──────────────────┘   │
│              │                       │
│  ┌───────────▼──────────────────┐   │
│  │ Try Remote (if internet)     │   │
│  │ Cache results locally        │   │
│  └───────────┬──────────────────┘   │
│              │                       │
│  ┌───────────▼──────────────────┐   │
│  │ Merge Local + Remote         │   │
│  │ (remote takes precedence)    │   │
│  └───────────┬──────────────────┘   │
│              │                       │
│  ┌───────────▼──────────────────┐   │
│  │ Filter by Hardware           │   │
│  │ Sort by Parameter Count      │   │
│  └───────────┬──────────────────┘   │
│              │                       │
│  ┌───────────▼──────────────────┐   │
│  │ Display Results              │   │
│  │ with Download Commands       │   │
│  └──────────────────────────────┘   │
│                                     │
└─────────────────────────────────────┘
```

---

## FAQ

**Q: Do I need internet to use SovereignStack?**  
A: No. The local bundled models work offline. Internet is optional to refresh the remote registry.

**Q: How often are remote models updated?**  
A: That depends on the remote registry operator. Our default registry auto-updates when new models are tested.

**Q: Can I point to my own registry?**  
A: Yes. Use `--registry https://your-registry.io/models.yaml` for any command.

**Q: What if the remote registry goes down?**  
A: SovereignStack falls back to cached models, then local models. You always have something to work with.

**Q: Why is my model not in the list?**  
A: Either it's incompatible with your hardware, or not yet tested by the community. Use `--all` to see everything, or add it to `~/.sovereignstack/models.yaml`.

---

**See Also:**
- [Quick Start Guide](./QUICKSTART.md)
- [Command Reference](./COMMANDS.md)
- [Model Management Guide](./MODEL_MANAGEMENT.md)
