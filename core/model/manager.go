package model

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gayanclife/sovereignstack/core"
)

// Manager handles model lifecycle (download, cache, validation)
type Manager struct {
	cacheDir    string
	models      map[string]*core.ModelCache // In-memory cache registry
	modelsMeta  map[string]*core.ModelMetadata
	modelsMutex sync.RWMutex
}

var (
	// Global models registry, loaded once
	globalModels map[string]*core.ModelMetadata
	modelsMutex  sync.Once
)

// NewManager creates a new model manager
func NewManager(cacheDir string) (*Manager, error) {
	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Load models from configuration (only once)
	modelsMutex.Do(func() {
		var err error
		globalModels, err = LoadAllModels()
		if err != nil {
			fmt.Printf("Warning: failed to load models from config: %v\n", err)
			globalModels = make(map[string]*core.ModelMetadata)
		}
	})

	return &Manager{
		cacheDir:   cacheDir,
		models:     make(map[string]*core.ModelCache),
		modelsMeta: globalModels,
	}, nil
}

// GetCacheDir returns the model cache directory
func (m *Manager) GetCacheDir() string {
	return m.cacheDir
}

// ListCachedModels returns all cached models
func (m *Manager) ListCachedModels() ([]core.ModelCache, error) {
	var cached []core.ModelCache

	entries, err := os.ReadDir(m.cacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			info, _ := entry.Info()
			cached = append(cached, core.ModelCache{
				Name:         entry.Name(),
				LocalPath:    filepath.Join(m.cacheDir, entry.Name()),
				Downloaded:   info.ModTime(),
				LastAccessed: info.ModTime(),
			})
		}
	}

	return cached, nil
}

// GetModel retrieves model metadata
func (m *Manager) GetModel(modelName string) *core.ModelMetadata {
	m.modelsMutex.RLock()
	defer m.modelsMutex.RUnlock()
	return m.modelsMeta[modelName]
}

// ValidateModel checks if a model is available and valid
func (m *Manager) ValidateModel(modelName string) error {
	metadata := m.GetModel(modelName)
	if metadata == nil {
		return fmt.Errorf("unknown model: %s", modelName)
	}

	// Check if model is cached locally
	localPath := filepath.Join(m.cacheDir, modelName)
	if info, err := os.Stat(localPath); err != nil || !info.IsDir() {
		return fmt.Errorf("model not cached locally: %s. Run 'sovstack pull %s' first", modelName, modelName)
	}

	return nil
}

// PullModel downloads a model to the cache (stub - actual implementation uses huggingface_hub)
func (m *Manager) PullModel(modelName string) error {
	metadata := m.GetModel(modelName)
	if metadata == nil {
		return fmt.Errorf("unknown model: %s", modelName)
	}

	localPath := filepath.Join(m.cacheDir, modelName)

	// Create model directory
	if err := os.MkdirAll(localPath, 0755); err != nil {
		return fmt.Errorf("failed to create model directory: %w", err)
	}

	// TODO: Implement actual model download from Hugging Face
	// For now, create a placeholder
	infoFile := filepath.Join(localPath, "model_info.json")
	if err := os.WriteFile(infoFile, []byte("{}"), 0644); err != nil {
		return fmt.Errorf("failed to save model info: %w", err)
	}

	// Update memory cache
	m.models[modelName] = &core.ModelCache{
		Name:         modelName,
		LocalPath:    localPath,
		Size:         metadata.Size["none"],
		Downloaded:   time.Now(),
		LastAccessed: time.Now(),
	}

	return nil
}

// RemoveModel removes a cached model
func (m *Manager) RemoveModel(modelName string) error {
	localPath := filepath.Join(m.cacheDir, modelName)
	if err := os.RemoveAll(localPath); err != nil {
		return fmt.Errorf("failed to remove model: %w", err)
	}

	delete(m.models, modelName)
	return nil
}

// GetModelPath returns the local path to a model
func (m *Manager) GetModelPath(modelName string) (string, error) {
	if err := m.ValidateModel(modelName); err != nil {
		return "", err
	}
	return filepath.Join(m.cacheDir, modelName), nil
}

// GetSuitableModels returns models that can run on the detected hardware
// If GPU is detected, returns GPU-optimized models
// If CPU-only, returns CPU-optimized models that fit in system RAM
func (m *Manager) GetSuitableModels(hasGPU bool, systemRAM int64) (suitable []*core.ModelMetadata, unavailable []*core.ModelMetadata) {
	m.modelsMutex.RLock()
	allModels := m.modelsMeta
	m.modelsMutex.RUnlock()

	for _, model := range allModels {
		switch {
		// GPU available - recommend GPU models
		case hasGPU:
			if model.HardwareTarget == core.HardwareGPUOnly || model.HardwareTarget == core.HardwareBoth {
				suitable = append(suitable, model)
			} else {
				unavailable = append(unavailable, model)
			}

		// CPU only - recommend CPU-optimized models that fit in RAM
		case !hasGPU:
			if model.HardwareTarget == core.HardwareCPUOptimized || model.HardwareTarget == core.HardwareBoth {
				// Check if model fits in available system RAM
				if model.MinimumSystemRAM <= systemRAM {
					suitable = append(suitable, model)
				} else {
					unavailable = append(unavailable, model)
				}
			} else {
				unavailable = append(unavailable, model)
			}
		}
	}

	return suitable, unavailable
}
