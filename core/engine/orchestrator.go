package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/gayanclife/sovereignstack/core"
	"github.com/gayanclife/sovereignstack/core/model"
	"github.com/gayanclife/sovereignstack/internal/docker"
	"github.com/gayanclife/sovereignstack/internal/hardware"
)

// EngineRoom is the main inference engine orchestrator
type EngineRoom struct {
	hardware     *hardware.SystemHardware
	modelManager *model.Manager
	vllmOrch     *docker.VLLMOrchestrator
	runningModel *core.ModelInstance
	quantCalc    *model.QuantizationCalculator
	config       EngineConfig
}

// EngineConfig contains engine configuration
type EngineConfig struct {
	ModelCacheDir string
	Port          int
	VRAMLimit     int64 // Optional: limit VRAM usage (0 = use all)
	GPUIndices    []int // GPU indices to use (nil = all)
}

// DeploymentPlan describes how a model will be deployed
type DeploymentPlan struct {
	ModelName             string
	Quantization          core.QuantizationType
	RequiredVRAM          int64
	AvailableVRAM         int64
	ContextLength         int
	EstimatedTokensPerSec int
	GPUAssignment         []int
	Notes                 string
}

// NewEngineRoom creates a new inference engine
func NewEngineRoom(config EngineConfig) (*EngineRoom, error) {
	// Detect hardware
	hw, err := hardware.GetSystemHardware()
	if err != nil {
		return nil, fmt.Errorf("failed to detect hardware: %w", err)
	}

	if len(hw.GPUs) == 0 {
		return nil, fmt.Errorf("no NVIDIA GPUs detected")
	}

	// Create model manager
	modelMgr, err := model.NewManager(config.ModelCacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create model manager: %w", err)
	}

	// Calculate available VRAM
	availableVRAM := hw.TotalAvailable
	if config.VRAMLimit > 0 && config.VRAMLimit < availableVRAM {
		availableVRAM = config.VRAMLimit
	}

	return &EngineRoom{
		hardware:     hw,
		modelManager: modelMgr,
		vllmOrch:     docker.NewVLLMOrchestrator(),
		quantCalc:    model.NewQuantizationCalculator(availableVRAM),
		config:       config,
	}, nil
}

// GetSystemInfo returns information about the system
func (er *EngineRoom) GetSystemInfo() *hardware.SystemHardware {
	return er.hardware
}

// PlanDeployment analyzes a model and creates a deployment plan
func (er *EngineRoom) PlanDeployment(ctx context.Context, modelName string) (*DeploymentPlan, error) {
	metadata := er.modelManager.GetModel(modelName)
	if metadata == nil {
		return nil, fmt.Errorf("unknown model: %s", modelName)
	}

	// Analyze model fit
	analysis := er.quantCalc.AnalyzeModelFit(metadata)
	if len(analysis.FittingQuantizations) == 0 {
		return nil, fmt.Errorf("model does not fit in available VRAM (%d MB)", er.hardware.TotalAvailable/(1024*1024))
	}

	plan := &DeploymentPlan{
		ModelName:     modelName,
		Quantization:  analysis.RecommendedQuant,
		RequiredVRAM:  analysis.RequiredVRAM[analysis.RecommendedQuant],
		AvailableVRAM: er.hardware.TotalAvailable,
		ContextLength: metadata.Context,
		GPUAssignment: er.config.GPUIndices,
		Notes:         analysis.Notes,
	}

	// Estimate tokens per second based on GPU
	if len(er.hardware.GPUs) > 0 {
		// Rough estimate: ~100 tok/sec per 24GB GPU with optimal quantization
		vramPer24GB := float32(er.hardware.GPUs[0].VRAM) / (24 * 1024 * 1024 * 1024)
		plan.EstimatedTokensPerSec = int(100.0 * vramPer24GB)
	}

	return plan, nil
}

// Deploy deploys a model with automatic quantization
func (er *EngineRoom) Deploy(ctx context.Context, modelName string, optionalQuantization *core.QuantizationType) error {
	// Check model is cached
	if err := er.modelManager.ValidateModel(modelName); err != nil {
		return err
	}

	metadata := er.modelManager.GetModel(modelName)
	if metadata == nil {
		return fmt.Errorf("unknown model: %s", modelName)
	}

	// Determine quantization
	quant := optionalQuantization
	if quant == nil {
		suggested, err := er.quantCalc.SuggestQuantization(metadata)
		if err != nil {
			return fmt.Errorf("cannot deploy model: %w", err)
		}
		quant = &suggested
	}

	// Get model path
	modelPath, err := er.modelManager.GetModelPath(modelName)
	if err != nil {
		return err
	}

	// Create vLLM config
	vllmCfg := docker.VLLMConfig{
		ModelPath:            modelPath,
		ModelName:            modelName,
		Quantization:         *quant,
		GPUIndices:           er.config.GPUIndices,
		ContextLength:        metadata.Context,
		GPUMemoryUtilization: 0.9,
		TensorParallelSize:   len(er.hardware.GPUs),
		Port:                 er.config.Port,
	}

	// Start vLLM container
	containerID, err := er.vllmOrch.Start(ctx, vllmCfg)
	if err != nil {
		return fmt.Errorf("failed to start vLLM: %w", err)
	}

	// Wait for health check
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		if err := er.vllmOrch.HealthCheck(ctx, er.config.Port); err == nil {
			break
		}
		if i == maxRetries-1 {
			return fmt.Errorf("vLLM container failed health check after 30s")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}

	// Update running model
	er.runningModel = &core.ModelInstance{
		ID:              containerID,
		ModelName:       modelName,
		Quantization:    *quant,
		ContainerID:     containerID,
		StartedAt:       time.Now(),
		IsHealthy:       true,
		LastHealthCheck: time.Now(),
	}

	return nil
}

// Stop stops the running model
func (er *EngineRoom) Stop(ctx context.Context) error {
	if er.runningModel == nil {
		return fmt.Errorf("no model running")
	}

	if err := er.vllmOrch.Stop(ctx); err != nil {
		return err
	}

	_ = er.vllmOrch.Remove(ctx) // Best effort cleanup
	er.runningModel = nil

	return nil
}

// Status returns the status of the engine
func (er *EngineRoom) Status(ctx context.Context) *EngineStatus {
	status := &EngineStatus{
		Timestamp:        time.Now(),
		Hardware:         er.hardware,
		RunningModel:     er.runningModel,
		ModelCacheDir:    er.config.ModelCacheDir,
		ContainerRunning: false,
	}

	if er.runningModel != nil {
		running, _ := er.vllmOrch.IsRunning(ctx)
		status.ContainerRunning = running
	}

	return status
}

// ListModels returns available models for deployment
func (er *EngineRoom) ListModels() map[string]*core.ModelMetadata {
	models := make(map[string]*core.ModelMetadata)

	// Get common models
	commonModels := map[string]*core.ModelMetadata{
		"meta-llama/Llama-2-7b-hf": {
			Name:           "meta-llama/Llama-2-7b-hf",
			ParameterCount: 7e9,
		},
		"meta-llama/Llama-2-13b-hf": {
			Name:           "meta-llama/Llama-2-13b-hf",
			ParameterCount: 13e9,
		},
		"mistralai/Mistral-7B-v0.1": {
			Name:           "mistralai/Mistral-7B-v0.1",
			ParameterCount: 7e9,
		},
	}

	for name, metadata := range commonModels {
		models[name] = metadata
	}

	return models
}

// EngineStatus represents the current state of the engine
type EngineStatus struct {
	Timestamp        time.Time
	Hardware         *hardware.SystemHardware
	RunningModel     *core.ModelInstance
	ModelCacheDir    string
	ContainerRunning bool
}
