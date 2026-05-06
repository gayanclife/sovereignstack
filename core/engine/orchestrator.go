package engine

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/gayanclife/sovereignstack/core"
	"github.com/gayanclife/sovereignstack/core/model"
	"github.com/gayanclife/sovereignstack/internal/docker"
	"github.com/gayanclife/sovereignstack/internal/hardware"
)

// EngineRoom is the main inference engine orchestrator
type EngineRoom struct {
	hardware      *hardware.SystemHardware
	modelManager  *model.Manager
	engines       map[string]docker.InferenceEngine // one engine per running model
	runningModels map[string]*core.ModelInstance    // models keyed by model name
	modelsMutex   sync.RWMutex                      // protects runningModels and engines
	quantCalc     *model.QuantizationCalculator
	config        EngineConfig
}

// EngineConfig contains engine configuration
type EngineConfig struct {
	ModelCacheDir string
	Port          int
	VRAMLimit     int64 // Optional: limit VRAM usage (0 = use all)
	GPUIndices    []int // GPU indices to use (nil = all)
	RebuildImage  bool  // Force rebuild of Docker image
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

	// Create model manager
	modelMgr, err := model.NewManager(config.ModelCacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create model manager: %w", err)
	}

	// Calculate available VRAM (0 if no GPUs)
	availableVRAM := hw.TotalAvailable
	if config.VRAMLimit > 0 && config.VRAMLimit < availableVRAM {
		availableVRAM = config.VRAMLimit
	}

	er := &EngineRoom{
		hardware:      hw,
		modelManager:  modelMgr,
		engines:       make(map[string]docker.InferenceEngine),
		runningModels: loadRunningModels(), // Load from disk
		quantCalc:     model.NewQuantizationCalculator(availableVRAM),
		config:        config,
	}

	return er, nil
}

// GetSystemInfo returns information about the system
func (er *EngineRoom) GetSystemInfo() *hardware.SystemHardware {
	return er.hardware
}

// GetSuitableModels returns models that can run on the detected hardware
// Returns two slices: suitable models and unavailable models with reasons
func (er *EngineRoom) GetSuitableModels() (suitable []*core.ModelMetadata, unavailable []*core.ModelMetadata) {
	hasGPU := len(er.hardware.GPUs) > 0
	return er.modelManager.GetSuitableModels(hasGPU, er.hardware.SystemRAM)
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

	// Check if model is actually running in Docker (not just in memory/persistence)
	runningModels, err := docker.GetRunningModels(ctx)
	if err == nil {
		for _, model := range runningModels {
			if model.ModelName == modelName && model.Status == "running" {
				return fmt.Errorf("model %s is already running (container: %s)", modelName, model.ContainerID[:12])
			}
		}
	}

	// Create a new engine instance for this model
	engine := docker.NewInferenceEngine(len(er.hardware.GPUs) > 0)

	// Create inference engine config with auto-assigned port
	inferenceCfg := docker.InferenceConfig{
		ModelPath:            modelPath,
		ModelName:            modelName,
		Quantization:         *quant,
		GPUIndices:           er.config.GPUIndices,
		ContextLength:        metadata.Context,
		GPUMemoryUtilization: 0.9,
		TensorParallelSize:   len(er.hardware.GPUs),
		Port:                 er.config.Port,
		RebuildImage:         er.config.RebuildImage,
	}

	// Start inference engine container
	containerID, err := engine.Start(ctx, inferenceCfg)
	if err != nil {
		return fmt.Errorf("failed to start inference engine: %w", err)
	}

	// Add to running models and engines maps immediately (no blocking)
	er.modelsMutex.Lock()
	er.engines[modelName] = engine
	er.runningModels[modelName] = &core.ModelInstance{
		ID:              containerID,
		ModelName:       modelName,
		Quantization:    *quant,
		ContainerID:     containerID,
		StartedAt:       time.Now(),
		IsHealthy:       false,
		LastHealthCheck: time.Now(),
	}
	er.modelsMutex.Unlock()

	// Persist running models to disk
	er.modelsMutex.RLock()
	_ = saveRunningModels(er.runningModels)
	er.modelsMutex.RUnlock()

	// Start background health monitoring
	go er.backgroundHealthCheck(modelName, engine, inferenceCfg.Port)

	return nil
}

// backgroundHealthCheck monitors model health asynchronously (for models that didn't pass initial check)
func (er *EngineRoom) backgroundHealthCheck(modelName string, engine docker.InferenceEngine, port int) {
	ctx := context.Background()
	maxRetries := 45 // Continue for up to 45 more seconds (total 60)
	for i := 0; i < maxRetries; i++ {
		if err := engine.HealthCheck(ctx, port); err == nil {
			// Update health status
			er.modelsMutex.Lock()
			if model, exists := er.runningModels[modelName]; exists {
				model.IsHealthy = true
				model.LastHealthCheck = time.Now()
				// Save updated state
				saveRunningModels(er.runningModels)
			}
			er.modelsMutex.Unlock()
			return
		}
		time.Sleep(1 * time.Second)
	}
}

// StopModel stops a specific running model by name (queries Docker directly)
func (er *EngineRoom) StopModel(ctx context.Context, modelName string) error {
	// Query Docker for the model container (works for GPU or CPU inference)
	runningModels, err := docker.GetRunningModels(ctx)
	if err != nil {
		return fmt.Errorf("failed to query running models: %w", err)
	}

	var containerID string
	for _, model := range runningModels {
		if model.ModelName == modelName {
			containerID = model.ContainerID
			break
		}
	}

	if containerID == "" {
		return fmt.Errorf("model %s is not running", modelName)
	}

	// Stop and remove the container directly via Docker
	stopCmd := exec.CommandContext(ctx, "docker", "stop", containerID)
	if err := stopCmd.Run(); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	removeCmd := exec.CommandContext(ctx, "docker", "rm", containerID)
	if err := removeCmd.Run(); err != nil {
		// Don't fail if removal fails, container is already stopped
	}

	// Clean up in-memory state if it exists
	er.modelsMutex.Lock()
	delete(er.engines, modelName)
	delete(er.runningModels, modelName)
	er.modelsMutex.Unlock()

	return nil
}

// GetRunningModels returns all currently running models by querying Docker
func (er *EngineRoom) GetRunningModels() map[string]*core.ModelInstance {
	ctx := context.Background()
	runningDockerModels, err := docker.GetRunningModels(ctx)
	if err != nil {
		// Log Docker query error but don't fail - fallback to in-memory state
		// Common causes: Docker not running, permission denied, or Docker not installed
		_ = err // Ignore for now, use fallback

		er.modelsMutex.RLock()
		defer er.modelsMutex.RUnlock()
		result := make(map[string]*core.ModelInstance)
		for name, instance := range er.runningModels {
			result[name] = instance
		}
		return result
	}

	// Convert Docker state to ModelInstance objects
	result := make(map[string]*core.ModelInstance)
	for _, dockerModel := range runningDockerModels {
		// Only include running containers
		if dockerModel.Status == "running" {
			isHealthy := dockerModel.Status == "running"
			result[dockerModel.ModelName] = &core.ModelInstance{
				ID:              dockerModel.ContainerID,
				ModelName:       dockerModel.ModelName,
				ContainerID:     dockerModel.ContainerID,
				StartedAt:       time.Now(), // Docker doesn't track start time easily, use now
				IsHealthy:       isHealthy,
				LastHealthCheck: time.Now(),
			}
		}
	}
	return result
}

// Status returns the status of the engine (queries Docker for actual state)
func (er *EngineRoom) Status(ctx context.Context) *EngineStatus {
	runningModels := er.GetRunningModels()

	status := &EngineStatus{
		Timestamp:     time.Now(),
		Hardware:      er.hardware,
		RunningModels: runningModels,
		ModelCacheDir: er.config.ModelCacheDir,
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
	Timestamp     time.Time
	Hardware      *hardware.SystemHardware
	RunningModels map[string]*core.ModelInstance
	ModelCacheDir string
}
