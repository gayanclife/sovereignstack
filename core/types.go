package core

import "time"

// QuantizationType represents different model quantization methods
type QuantizationType string

const (
	QuantizationNone QuantizationType = "none" // Full precision (FP32/FP16)
	QuantizationAWQ  QuantizationType = "awq"  // Activation-aware Quantization
	QuantizationGPTQ QuantizationType = "gptq" // GPTQ post-training quantization
	QuantizationINT8 QuantizationType = "int8" // INT8 quantization
)

// HardwareTarget specifies what hardware a model requires/supports
type HardwareTarget string

const (
	HardwareGPUOnly      HardwareTarget = "gpu"  // Requires NVIDIA GPU
	HardwareCPUOptimized HardwareTarget = "cpu"  // CPU-optimized, can run on CPU
	HardwareBoth         HardwareTarget = "both" // Works well on both GPU and CPU
)

// ModelMetadata contains information about a model
type ModelMetadata struct {
	Name                string           `json:"name"`                 // e.g., "meta-llama/Llama-2-7b-hf"
	Repo                string           `json:"repo"`                 // Hugging Face repo ID
	ParameterCount      int64            `json:"parameter_count"`      // Number of parameters (e.g., 7B, 13B)
	DefaultQuantization QuantizationType `json:"default_quantization"` // Recommended quantization
	Size                map[string]int64 `json:"size"`                 // Size in bytes per quantization
	Description         string           `json:"description"`
	Context             int              `json:"context_length"`     // Max context length
	RequiredVRAM        map[string]int64 `json:"required_vram"`      // VRAM per quantization in bytes
	HardwareTarget      HardwareTarget   `json:"hardware_target"`    // GPU, CPU, or both
	MinimumSystemRAM    int64            `json:"minimum_system_ram"` // Min system RAM for CPU inference
}

// QuantizationProfile represents VRAM requirements for different quantizations
type QuantizationProfile struct {
	Type         QuantizationType `json:"type"`
	VRAMRequired int64            `json:"vram_required"` // Minimum VRAM in bytes
	DataType     string           `json:"data_type"`     // e.g., "int4", "int8", "fp16"
	Speed        string           `json:"speed"`         // Relative speed: "fast", "medium", "slow"
	Quality      string           `json:"quality"`       // Relative quality: "low", "medium", "high"
}

// ModelInstance represents a running model
type ModelInstance struct {
	ID              string
	ModelName       string
	Quantization    QuantizationType
	ContainerID     string
	StartedAt       time.Time
	GPUIndex        int
	VRAMUsed        int64
	IsHealthy       bool
	LastHealthCheck time.Time
}

// ModelCache represents a locally cached model
type ModelCache struct {
	Name         string
	LocalPath    string
	Quantization QuantizationType
	Size         int64
	Downloaded   time.Time
	LastAccessed time.Time
	Checksum     string
}

// InferenceRequest represents an API request to the model
type InferenceRequest struct {
	Model       string  `json:"model"`
	Prompt      string  `json:"prompt"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float32 `json:"temperature"`
	TopP        float32 `json:"top_p"`
}

// InferenceResponse represents the API response from the model
type InferenceResponse struct {
	Model        string `json:"model"`
	Output       string `json:"output"`
	TokensUsed   int    `json:"tokens_used"`
	Duration     int64  `json:"duration_ms"`
	FinishReason string `json:"finish_reason"`
}
