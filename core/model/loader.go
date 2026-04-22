package model

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gayanclife/sovereignstack/core"
	"gopkg.in/yaml.v3"
)

// ModelRegistry represents the YAML structure for models
type ModelRegistry struct {
	GPUModels    []YAMLModel `yaml:"gpu_models"`
	CPUModels    []YAMLModel `yaml:"cpu_models"`
	HybridModels []YAMLModel `yaml:"hybrid_models"`
}

// YAMLModel represents a single model in the YAML file
type YAMLModel struct {
	Name                string             `yaml:"name"`
	Repo                string             `yaml:"repo"`
	Description         string             `yaml:"description"`
	Parameters          string             `yaml:"parameters"`
	ContextLength       int                `yaml:"context_length"`
	HardwareTarget      string             `yaml:"hardware_target"`
	MinimumSystemRAMGB  float64            `yaml:"minimum_system_ram_gb"`
	Sizes               map[string]int64   `yaml:"sizes"`
	RequiredVRAMGB      map[string]float64 `yaml:"required_vram_gb"`
	DefaultQuantization string             `yaml:"default_quantization"`
	Tags                []string           `yaml:"tags"`
}

// LoadModelsFromYAML loads models from YAML bytes and converts to ModelMetadata
func LoadModelsFromYAML(data []byte) (map[string]*core.ModelMetadata, error) {
	var registry ModelRegistry

	if err := yaml.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return convertRegistryToModels(&registry)
}

// convertRegistryToModels converts ModelRegistry to ModelMetadata map
func convertRegistryToModels(registry *ModelRegistry) (map[string]*core.ModelMetadata, error) {
	models := make(map[string]*core.ModelMetadata)

	allYAMLModels := append(
		append(registry.GPUModels, registry.CPUModels...),
		registry.HybridModels...,
	)

	for _, yml := range allYAMLModels {
		// Convert hardware target string to enum
		var hwTarget core.HardwareTarget
		switch yml.HardwareTarget {
		case "gpu":
			hwTarget = core.HardwareGPUOnly
		case "cpu":
			hwTarget = core.HardwareCPUOptimized
		case "both":
			hwTarget = core.HardwareBoth
		default:
			return nil, fmt.Errorf("unknown hardware target: %s for model %s", yml.HardwareTarget, yml.Name)
		}

		// Convert VRAM requirements from GB to bytes
		requiredVRAM := make(map[string]int64)
		for quant, gb := range yml.RequiredVRAMGB {
			requiredVRAM[quant] = int64(gb * 1024 * 1024 * 1024)
		}

		// Convert default quantization string to type
		defaultQuant := core.QuantizationType(yml.DefaultQuantization)

		model := &core.ModelMetadata{
			Name:                yml.Name,
			Repo:                yml.Repo,
			ParameterCount:      parseParameterCount(yml.Parameters),
			DefaultQuantization: defaultQuant,
			Size:                yml.Sizes,
			Description:         yml.Description,
			Context:             yml.ContextLength,
			RequiredVRAM:        requiredVRAM,
			HardwareTarget:      hwTarget,
			MinimumSystemRAM:    int64(yml.MinimumSystemRAMGB * 1024 * 1024 * 1024),
		}

		models[yml.Name] = model
	}

	return models, nil
}

// getDefaultModels returns a hardcoded default model registry as fallback
func getDefaultModels() *ModelRegistry {
	return &ModelRegistry{
		GPUModels: []YAMLModel{
			{
				Name:                "meta-llama/Llama-2-7b-hf",
				Repo:                "meta-llama/Llama-2-7b-hf",
				Description:         "Meta's Llama 2 7B - highly capable general-purpose model",
				Parameters:          "7B",
				ContextLength:       4096,
				HardwareTarget:      "gpu",
				MinimumSystemRAMGB:  0,
				Sizes:               map[string]int64{"none": 13858000000, "int8": 7456000000, "awq": 3200000000, "gptq": 3200000000},
				RequiredVRAMGB:      map[string]float64{"none": 14, "int8": 8, "awq": 4, "gptq": 4},
				DefaultQuantization: "awq",
			},
			{
				Name:                "meta-llama/Llama-2-13b-hf",
				Repo:                "meta-llama/Llama-2-13b-hf",
				Description:         "Meta's Llama 2 13B - larger model for more complex tasks",
				Parameters:          "13B",
				ContextLength:       4096,
				HardwareTarget:      "gpu",
				MinimumSystemRAMGB:  0,
				Sizes:               map[string]int64{"none": 27000000000, "int8": 13600000000, "awq": 6400000000, "gptq": 6400000000},
				RequiredVRAMGB:      map[string]float64{"none": 28, "int8": 15, "awq": 7, "gptq": 7},
				DefaultQuantization: "awq",
			},
			{
				Name:                "mistralai/Mistral-7B-v0.1",
				Repo:                "mistralai/Mistral-7B-v0.1",
				Description:         "Mistral 7B - efficient model with extended context (32k tokens)",
				Parameters:          "7B",
				ContextLength:       32768,
				HardwareTarget:      "gpu",
				MinimumSystemRAMGB:  0,
				Sizes:               map[string]int64{"none": 13858000000, "int8": 7456000000, "awq": 3200000000, "gptq": 3200000000},
				RequiredVRAMGB:      map[string]float64{"none": 14, "int8": 8, "awq": 4, "gptq": 4},
				DefaultQuantization: "awq",
			},
		},
		CPUModels: []YAMLModel{
			{
				Name:                "distilbert-base-uncased",
				Repo:                "distilbert-base-uncased",
				Description:         "DistilBERT - lightweight BERT variant, excellent for embeddings and classification",
				Parameters:          "66M",
				ContextLength:       512,
				HardwareTarget:      "cpu",
				MinimumSystemRAMGB:  0.5,
				Sizes:               map[string]int64{"none": 250000000, "int8": 100000000},
				RequiredVRAMGB:      map[string]float64{"none": 0, "int8": 0},
				DefaultQuantization: "int8",
			},
			{
				Name:                "TinyLlama/TinyLlama-1.1B",
				Repo:                "TinyLlama/TinyLlama-1.1B",
				Description:         "TinyLlama - small but capable LLM, great for on-premise CPU deployment",
				Parameters:          "1.1B",
				ContextLength:       2048,
				HardwareTarget:      "cpu",
				MinimumSystemRAMGB:  3,
				Sizes:               map[string]int64{"none": 2200000000, "int8": 1100000000},
				RequiredVRAMGB:      map[string]float64{"none": 0, "int8": 0},
				DefaultQuantization: "int8",
			},
			{
				Name:                "microsoft/phi-2",
				Repo:                "microsoft/phi-2",
				Description:         "Microsoft Phi-2 - state-of-the-art small language model",
				Parameters:          "2.7B",
				ContextLength:       2048,
				HardwareTarget:      "cpu",
				MinimumSystemRAMGB:  6,
				Sizes:               map[string]int64{"none": 5400000000, "int8": 2700000000},
				RequiredVRAMGB:      map[string]float64{"none": 0, "int8": 0},
				DefaultQuantization: "int8",
			},
		},
	}
}

// LoadAllModels loads models from multiple sources with proper precedence:
// 1. ./models.yaml (project-specific)
// 2. /etc/sovereignstack/models.yaml (system-wide)
// 3. ~/.sovereignstack/models.yaml (user-specific)
// Later sources override earlier ones with the same model name.
func LoadAllModels() (map[string]*core.ModelMetadata, error) {
	models := make(map[string]*core.ModelMetadata)

	// Try to find and load bundled models.yaml from common locations
	bundledPaths := []string{
		"models.yaml",
		filepath.Join(filepath.Dir(os.Args[0]), "models.yaml"),
		"/usr/local/share/sovereignstack/models.yaml",
	}

	foundBundled := false
	for _, path := range bundledPaths {
		if bundleModels, err := loadModelsFromFile(path); err == nil {
			for name, model := range bundleModels {
				models[name] = model
			}
			foundBundled = true
			break
		}
	}

	// If no bundled models found, use default hardcoded registry
	if !foundBundled {
		defaultRegistry := getDefaultModels()
		defaultModels, err := convertRegistryToModels(defaultRegistry)
		if err != nil {
			return nil, fmt.Errorf("failed to load default models: %w", err)
		}
		for name, model := range defaultModels {
			models[name] = model
		}
	}

	// Load from system-wide config (overrides bundled)
	if systemModels, err := loadModelsFromFile("/etc/sovereignstack/models.yaml"); err == nil {
		for name, model := range systemModels {
			models[name] = model
		}
	}

	// Load from user config (overrides system)
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		userConfigPath := filepath.Join(homeDir, ".sovereignstack", "models.yaml")
		if userModels, err := loadModelsFromFile(userConfigPath); err == nil {
			for name, model := range userModels {
				models[name] = model
			}
		}
	}

	// Load from project-local config (highest priority)
	if localModels, err := loadModelsFromFile("./models.local.yaml"); err == nil {
		for name, model := range localModels {
			models[name] = model
		}
	}

	return models, nil
}

// loadModelsFromFile loads models from a specific file
func loadModelsFromFile(path string) (map[string]*core.ModelMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	models, err := LoadModelsFromYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse models from %s: %w", path, err)
	}

	return models, nil
}

// parseParameterCount converts strings like "7B", "13B", "1.1B" to int64
func parseParameterCount(s string) int64 {
	var multiplier int64 = 1

	if len(s) > 0 {
		lastChar := s[len(s)-1]
		switch lastChar {
		case 'B':
			multiplier = 1_000_000_000
			s = s[:len(s)-1]
		case 'M':
			multiplier = 1_000_000
			s = s[:len(s)-1]
		case 'K':
			multiplier = 1_000
			s = s[:len(s)-1]
		}
	}

	var value float64
	fmt.Sscanf(s, "%f", &value)
	return int64(value * float64(multiplier))
}
