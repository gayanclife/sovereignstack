package model

import (
	"fmt"

	"github.com/gayanclife/sovereignstack/core"
)

// QuantizationCalculator handles model size calculations for different quantization types
type QuantizationCalculator struct {
	availableVRAM int64
}

// NewQuantizationCalculator creates a calculator for a specific VRAM amount
func NewQuantizationCalculator(availableVRAMBytes int64) *QuantizationCalculator {
	return &QuantizationCalculator{
		availableVRAM: availableVRAMBytes,
	}
}

// GetQuantizationProfiles returns all quantization options for a model
func (qc *QuantizationCalculator) GetQuantizationProfiles(paramCount int64) []core.QuantizationProfile {
	return []core.QuantizationProfile{
		{
			Type:         core.QuantizationNone,
			VRAMRequired: qc.calculateVRAM(paramCount, 16), // FP16
			DataType:     "fp16",
			Speed:        "fast",
			Quality:      "high",
		},
		{
			Type:         core.QuantizationAWQ,
			VRAMRequired: qc.calculateVRAM(paramCount, 4),
			DataType:     "int4",
			Speed:        "medium",
			Quality:      "high",
		},
		{
			Type:         core.QuantizationGPTQ,
			VRAMRequired: qc.calculateVRAM(paramCount, 4),
			DataType:     "int4",
			Speed:        "medium",
			Quality:      "medium",
		},
		{
			Type:         core.QuantizationINT8,
			VRAMRequired: qc.calculateVRAM(paramCount, 8),
			DataType:     "int8",
			Speed:        "slow",
			Quality:      "medium",
		},
	}
}

// SuggestQuantization returns the best quantization type for the given model
// It prioritizes quality first, then speed, then fitting in VRAM
func (qc *QuantizationCalculator) SuggestQuantization(metadata *core.ModelMetadata) (core.QuantizationType, error) {
	profiles := qc.GetQuantizationProfiles(metadata.ParameterCount)

	// Find all quantizations that fit in available VRAM
	fittingQuantizations := []core.QuantizationProfile{}
	for _, profile := range profiles {
		if profile.VRAMRequired <= qc.availableVRAM {
			fittingQuantizations = append(fittingQuantizations, profile)
		}
	}

	if len(fittingQuantizations) == 0 {
		return "", fmt.Errorf(
			"model %s (%d params) requires %d MB, but only %d MB VRAM available",
			metadata.Name,
			metadata.ParameterCount,
			profiles[0].VRAMRequired/(1024*1024),
			qc.availableVRAM/(1024*1024),
		)
	}

	// Priority: AWQ (best quality/speed trade-off) > FP16 (if it fits) > GPTQ > INT8
	priorityOrder := []core.QuantizationType{
		core.QuantizationAWQ,
		core.QuantizationNone,
		core.QuantizationGPTQ,
		core.QuantizationINT8,
	}

	for _, priority := range priorityOrder {
		for _, profile := range fittingQuantizations {
			if profile.Type == priority {
				return priority, nil
			}
		}
	}

	return fittingQuantizations[0].Type, nil
}

// CanFit checks if a model can fit in the available VRAM with a specific quantization
func (qc *QuantizationCalculator) CanFit(paramCount int64, quantization core.QuantizationType) bool {
	required := qc.vramForQuantization(paramCount, quantization)
	return required <= qc.availableVRAM
}

// GetModelSize returns the approximate size of a model with specific quantization (in bytes)
func (qc *QuantizationCalculator) GetModelSize(paramCount int64, quantization core.QuantizationType) int64 {
	return qc.vramForQuantization(paramCount, quantization)
}

// calculateVRAM calculates VRAM needed for a model based on bits per parameter
// Formula: (params * bits_per_param) / 8 + overhead
// Typical overhead is 10-20% for activations, KV cache, etc.
func (qc *QuantizationCalculator) calculateVRAM(paramCount int64, bitsPerParam int) int64 {
	bytesPerParam := int64(bitsPerParam) / 8
	modelSize := paramCount * bytesPerParam
	overhead := int64(float64(modelSize) * 0.15) // 15% overhead for activations, etc.
	return modelSize + overhead
}

// vramForQuantization returns VRAM requirement for a specific quantization type
func (qc *QuantizationCalculator) vramForQuantization(paramCount int64, quantization core.QuantizationType) int64 {
	switch quantization {
	case core.QuantizationNone:
		return qc.calculateVRAM(paramCount, 16) // FP16
	case core.QuantizationAWQ:
		return qc.calculateVRAM(paramCount, 4)
	case core.QuantizationGPTQ:
		return qc.calculateVRAM(paramCount, 4)
	case core.QuantizationINT8:
		return qc.calculateVRAM(paramCount, 8)
	default:
		return qc.calculateVRAM(paramCount, 16)
	}
}

// ModelFitAnalysis provides detailed information about model fitting
type ModelFitAnalysis struct {
	ModelName            string
	AvailableVRAM        int64
	FittingQuantizations []core.QuantizationType
	RecommendedQuant     core.QuantizationType
	RequiredVRAM         map[core.QuantizationType]int64
	Notes                string
}

// AnalyzeModelFit provides a detailed analysis of model fit
func (qc *QuantizationCalculator) AnalyzeModelFit(metadata *core.ModelMetadata) *ModelFitAnalysis {
	analysis := &ModelFitAnalysis{
		ModelName:            metadata.Name,
		AvailableVRAM:        qc.availableVRAM,
		FittingQuantizations: []core.QuantizationType{},
		RequiredVRAM:         make(map[core.QuantizationType]int64),
	}

	profiles := qc.GetQuantizationProfiles(metadata.ParameterCount)

	for _, profile := range profiles {
		analysis.RequiredVRAM[profile.Type] = profile.VRAMRequired

		if profile.VRAMRequired <= qc.availableVRAM {
			analysis.FittingQuantizations = append(analysis.FittingQuantizations, profile.Type)
		}
	}

	if len(analysis.FittingQuantizations) == 0 {
		analysis.Notes = fmt.Sprintf(
			"Model does not fit. Min required: %d MB, available: %d MB",
			profiles[0].VRAMRequired/(1024*1024),
			qc.availableVRAM/(1024*1024),
		)
		return analysis
	}

	recommended, _ := qc.SuggestQuantization(metadata)
	analysis.RecommendedQuant = recommended
	analysis.Notes = fmt.Sprintf(
		"✓ Model fits with %s quantization (%d MB / %d MB)",
		recommended,
		analysis.RequiredVRAM[recommended]/(1024*1024),
		qc.availableVRAM/(1024*1024),
	)

	return analysis
}
