package docker
package docker

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gayanclife/sovereignstack/core"
)

// VLLMConfig represents vLLM container configuration
type VLLMConfig struct {
	ModelPath           string
	ModelName           string
	Quantization        core.QuantizationType
	GPUIndices          []int
	MaxModelLen         int
	ContextLength       int
	GPUMemoryUtilization float32
	TensorParallelSize  int
	Port                int
}

// VLLMOrchestrator manages vLLM container lifecycle
type VLLMOrchestrator struct {
	containerID string
}

// NewVLLMOrchestrator creates a new orchestrator
func NewVLLMOrchestrator() *VLLMOrchestrator {
	return &VLLMOrchestrator{}
}

// Start launches a vLLM container with the specified configuration
func (vo *VLLMOrchestrator) Start(ctx context.Context, config VLLMConfig) (containerID string, err error) {
	// Build vLLM command arguments
	args := []string{
		"run",
		"--gpus", buildGPUSpec(config.GPUIndices),
		"--shm-size", "2g",
		"-p", fmt.Sprintf("%d:8000", config.Port),
		"-v", fmt.Sprintf("%s:/models", filepath.Dir(config.ModelPath)),
		"-e", "CUDA_VISIBLE_DEVICES=" + buildCUDADevices(config.GPUIndices),
		"-e", fmt.Sprintf("VLLM_GPU_MEMORY_UTILIZATION=%.2f", config.GPUMemoryUtilization),
		"-e", fmt.Sprintf("VLLM_TENSOR_PARALLEL_SIZE=%d", config.TensorParallelSize),
		"--name", "vllm-" + config.ModelName,
		"--restart", "unless-stopped",
		"vllm/vllm-openai:latest",
		"--model", filepath.Base(config.ModelPath),
		"--host", "0.0.0.0",
		"--port", "8000",
	}

	// Add quantization-specific parameters
	if config.Quantization != core.QuantizationNone {
		args = append(args, "--quantization", string(config.Quantization))
	}

	// Add context length if specified
	if config.ContextLength > 0 {
		args = append(args, "--max-model-len", strconv.Itoa(config.ContextLength))
	}

	// Execute docker run command
	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to start vLLM container: %v\nOutput: %s", err, string(output))
	}

	// Extract container ID from output
	containerID = strings.TrimSpace(string(output))
	vo.containerID = containerID

	return containerID, nil
}

// Stop halts the vLLM container
func (vo *VLLMOrchestrator) Stop(ctx context.Context) error {
	if vo.containerID == "" {
		return fmt.Errorf("no active container")
	}

	cmd := exec.CommandContext(ctx, "docker", "stop", vo.containerID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	return nil
}

// Remove deletes the container
func (vo *VLLMOrchestrator) Remove(ctx context.Context) error {
	if vo.containerID == "" {
		return fmt.Errorf("no active container")
	}

	cmd := exec.CommandContext(ctx, "docker", "rm", vo.containerID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	vo.containerID = ""
	return nil
}

// GetLogs retrieves container logs
func (vo *VLLMOrchestrator) GetLogs(ctx context.Context, tailLines int) (string, error) {
	if vo.containerID == "" {
		return "", fmt.Errorf("no active container")
	}

	cmd := exec.CommandContext(
		ctx,
		"docker", "logs",
		"--tail", strconv.Itoa(tailLines),
		vo.containerID,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}

	return string(output), nil
}

// IsRunning checks if the container is running
func (vo *VLLMOrchestrator) IsRunning(ctx context.Context) (bool, error) {
	if vo.containerID == "" {
		return false, nil
	}

	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", vo.containerID)
	output, err := cmd.Output()
	if err != nil {
		return false, nil // Container doesn't exist
	}

	return strings.TrimSpace(string(output)) == "true", nil
}

// HealthCheck performs a health check on the vLLM container
func (vo *VLLMOrchestrator) HealthCheck(ctx context.Context, port int) error {
	// Check if vLLM API is responding
	cmd := exec.CommandContext(
		ctx,
		"curl",
		"-s",
		fmt.Sprintf("http://localhost:%d/health", port),
	)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	if !strings.Contains(string(output), "ok") {
		return fmt.Errorf("unhealthy: %s", string(output))
	}

	return nil
}

// buildGPUSpec creates the Docker GPU specification
func buildGPUSpec(gpuIndices []int) string {
	if len(gpuIndices) == 0 {
		return "all"
	}

	parts := []string{}
	for _, idx := range gpuIndices {
		parts = append(parts, strconv.Itoa(idx))
	}
	return "device=" + strings.Join(parts, ",")
}

// buildCUDADevices creates the CUDA_VISIBLE_DEVICES string
func buildCUDADevices(gpuIndices []int) string {
	if len(gpuIndices) == 0 {
		return "0"
	}

	parts := []string{}
	for _, idx := range gpuIndices {
		parts = append(parts, strconv.Itoa(idx))
	}
	return strings.Join(parts, ",")
}
