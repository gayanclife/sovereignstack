package model

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gayanclife/sovereignstack/core"
)

// Manager handles model lifecycle (download, cache, validation)
type Manager struct {
	cacheDir string
	models   map[string]*core.ModelCache // In-memory cache registry
}

// NewManager creates a new model manager
func NewManager(cacheDir string) (*Manager, error) {
	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &Manager{
		cacheDir: cacheDir,
		models:   make(map[string]*core.ModelCache),
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
	return getCommonModels()[modelName]
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

// Common model registry - these are well-known models
func getCommonModels() map[string]*core.ModelMetadata {
	return map[string]*core.ModelMetadata{
		"meta-llama/Llama-2-7b-hf": {
			Name:                "meta-llama/Llama-2-7b-hf",
			Repo:                "meta-llama/Llama-2-7b-hf",
			ParameterCount:      7e9,
			DefaultQuantization: core.QuantizationAWQ,
			Size: map[string]int64{
				"none": 13 * 1024 * 1024 * 1024, // ~13GB FP16
				"awq":  3 * 1024 * 1024 * 1024,  // ~3GB INT4
				"gptq": 3 * 1024 * 1024 * 1024,  // ~3GB INT4
				"int8": 7 * 1024 * 1024 * 1024,  // ~7GB INT8
			},
			Context:     4096,
			Description: "Meta's Llama 2 7B parameter model",
			RequiredVRAM: map[string]int64{
				"none": 14 * 1024 * 1024 * 1024, // ~14GB with overhead
				"awq":  4 * 1024 * 1024 * 1024,  // ~4GB with overhead
				"gptq": 4 * 1024 * 1024 * 1024,  // ~4GB with overhead
				"int8": 8 * 1024 * 1024 * 1024,  // ~8GB with overhead
			},
		},
		"meta-llama/Llama-2-13b-hf": {
			Name:                "meta-llama/Llama-2-13b-hf",
			Repo:                "meta-llama/Llama-2-13b-hf",
			ParameterCount:      13e9,
			DefaultQuantization: core.QuantizationAWQ,
			Size: map[string]int64{
				"none": 26 * 1024 * 1024 * 1024, // ~26GB FP16
				"awq":  6 * 1024 * 1024 * 1024,  // ~6GB INT4
				"gptq": 6 * 1024 * 1024 * 1024,  // ~6GB INT4
				"int8": 13 * 1024 * 1024 * 1024, // ~13GB INT8
			},
			Context:     4096,
			Description: "Meta's Llama 2 13B parameter model",
			RequiredVRAM: map[string]int64{
				"none": 28 * 1024 * 1024 * 1024, // ~28GB with overhead
				"awq":  7 * 1024 * 1024 * 1024,  // ~7GB with overhead
				"gptq": 7 * 1024 * 1024 * 1024,  // ~7GB with overhead
				"int8": 15 * 1024 * 1024 * 1024, // ~15GB with overhead
			},
		},
		"mistralai/Mistral-7B-v0.1": {
			Name:                "mistralai/Mistral-7B-v0.1",
			Repo:                "mistralai/Mistral-7B-v0.1",
			ParameterCount:      7e9,
			DefaultQuantization: core.QuantizationAWQ,
			Size: map[string]int64{
				"none": 13 * 1024 * 1024 * 1024, // ~13GB FP16
				"awq":  3 * 1024 * 1024 * 1024,  // ~3GB INT4
				"gptq": 3 * 1024 * 1024 * 1024,  // ~3GB INT4
				"int8": 7 * 1024 * 1024 * 1024,  // ~7GB INT8
			},
			Context:     32768,
			Description: "Mistral's 7B parameter model with extended context",
			RequiredVRAM: map[string]int64{
				"none": 14 * 1024 * 1024 * 1024, // ~14GB with overhead
				"awq":  4 * 1024 * 1024 * 1024,  // ~4GB with overhead
				"gptq": 4 * 1024 * 1024 * 1024,  // ~4GB with overhead
				"int8": 8 * 1024 * 1024 * 1024,  // ~8GB with overhead
			},
		},
	}
}
