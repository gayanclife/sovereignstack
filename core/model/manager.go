package model

import (
	"encoding/binary"
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

	// Check if model directory exists
	localPath := filepath.Join(m.cacheDir, modelName)
	if info, err := os.Stat(localPath); err != nil || !info.IsDir() {
		return fmt.Errorf("model not cached locally: %s. Run 'sovstack pull %s' first", modelName, modelName)
	}

	// Check for actual model files (not just placeholder directory)
	hasModelFiles := false
	err := filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && (filepath.Ext(path) == ".safetensors" || filepath.Ext(path) == ".bin") {
			hasModelFiles = true
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to validate model directory: %w", err)
	}

	if !hasModelFiles {
		return fmt.Errorf("model directory exists but contains no model files: run 'sovstack pull %s'", modelName)
	}

	// Check safetensors files for corruption
	err = filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && filepath.Ext(path) == ".safetensors" {
			if validErr := validateSafetensorsFile(path); validErr != nil {
				return fmt.Errorf("model file is corrupt (%s): %v. Run 'sovstack pull -f %s' to re-download", filepath.Base(path), validErr, modelName)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

// validateSafetensorsFile checks if a safetensors file is valid by reading its header
func validateSafetensorsFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer file.Close()

	// Check file size
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("cannot stat file: %w", err)
	}
	fileSize := info.Size()
	if fileSize < 8 {
		return fmt.Errorf("file too small: %d bytes (minimum 8 bytes for header)", fileSize)
	}

	// safetensors format: 8-byte little-endian header size + JSON header + tensors
	headerSizeBytes := make([]byte, 8)
	n, err := file.Read(headerSizeBytes)
	if err != nil || n < 8 {
		return fmt.Errorf("cannot read header size: incomplete safetensors file")
	}

	headerSize := binary.LittleEndian.Uint64(headerSizeBytes)
	if headerSize == 0 || headerSize > 1024*1024*100 { // header should be < 100MB
		return fmt.Errorf("invalid safetensors header size: %d bytes", headerSize)
	}

	// Check if file has enough bytes for header
	minFileSize := int64(8) + int64(headerSize)
	if fileSize < minFileSize {
		return fmt.Errorf("incomplete file: expected at least %d bytes (8 + %d header), got %d", minFileSize, headerSize, fileSize)
	}

	// Try to read the header to verify it's readable
	headerBytes := make([]byte, headerSize)
	n, err = file.Read(headerBytes)
	if err != nil || int64(n) < int64(headerSize) {
		return fmt.Errorf("incomplete safetensors file: expected %d header bytes, got %d", headerSize, n)
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
