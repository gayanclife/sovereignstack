package docker

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gayanclife/sovereignstack/core"
)

// VLLMOrchestrator manages vLLM container lifecycle (implements InferenceEngine)
type VLLMOrchestrator struct {
	containerID string
}

// NewVLLMOrchestrator creates a new orchestrator
func NewVLLMOrchestrator() *VLLMOrchestrator {
	return &VLLMOrchestrator{}
}

// imageExists checks if a Docker image exists locally
func imageExists(ctx context.Context, image string) (bool, error) {
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	err := cmd.Run()
	return err == nil, nil
}

// pullImage downloads a Docker image from registry
func pullImage(ctx context.Context, image string) error {
	fmt.Printf("Pulling Docker image: %s\n", image)
	cmd := exec.CommandContext(ctx, "docker", "pull", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %v\nOutput: %s", image, err, string(output))
	}
	fmt.Println("✓ Image ready")
	return nil
}

// Start launches a vLLM container with the specified configuration
func (vo *VLLMOrchestrator) Start(ctx context.Context, config InferenceConfig) (containerID string, err error) {
	const vllmImage = "vllm/vllm-openai:latest"
	containerName := "vllm-" + config.ModelName

	// Ensure vLLM image is available
	exists, err := imageExists(ctx, vllmImage)
	if err != nil {
		return "", fmt.Errorf("failed to check for vLLM image: %w", err)
	}

	if !exists {
		if err := pullImage(ctx, vllmImage); err != nil {
			return "", err
		}
	}

	// Remove stale container with same name if it exists
	checkCmd := exec.CommandContext(ctx, "docker", "ps", "-a", "-q", "-f", "name="+containerName)
	existingID, _ := checkCmd.Output()
	if len(existingID) > 0 {
		// Container exists, remove it
		removeCmd := exec.CommandContext(ctx, "docker", "rm", "-f", containerName)
		removeCmd.Output()
		fmt.Printf("Cleaned up stale container: %s\n", containerName)
	}

	// Find a free port if the requested one is in use
	actualPort := findFreePort(config.Port)
	if actualPort != config.Port {
		fmt.Printf("Port %d is in use, using port %d instead\n", config.Port, actualPort)
	}

	// Build vLLM command arguments
	args := []string{
		"run",
		"--shm-size", "2g",
		"-p", fmt.Sprintf("%d:8000", actualPort),
		"-v", fmt.Sprintf("%s:/models", filepath.Dir(config.ModelPath)),
		"--name", containerName,
		"--restart", "unless-stopped",
	}

	// Add GPU support only if GPUs are available
	if len(config.GPUIndices) > 0 {
		args = append(args,
			"--gpus", buildGPUSpec(config.GPUIndices),
			"-e", "CUDA_VISIBLE_DEVICES="+buildCUDADevices(config.GPUIndices),
			"-e", fmt.Sprintf("VLLM_GPU_MEMORY_UTILIZATION=%.2f", config.GPUMemoryUtilization),
			"-e", fmt.Sprintf("VLLM_TENSOR_PARALLEL_SIZE=%d", config.TensorParallelSize),
		)
	}

	// Add vLLM image and model arguments
	args = append(args,
		vllmImage,
		"--model", filepath.Base(config.ModelPath),
		"--host", "0.0.0.0",
		"--port", "8000",
	)

	// Force CPU mode if no GPUs available
	if len(config.GPUIndices) == 0 {
		args = append(args, "--device", "cpu")
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

	// Health check with the actual port
	for i := 0; i < 30; i++ {
		if err := vo.HealthCheck(ctx, actualPort); err == nil {
			fmt.Printf("✓ Inference engine ready on port %d\n", actualPort)
			break
		}
		if i == 29 {
			return "", fmt.Errorf("inference engine failed health check after 30s")
		}
		time.Sleep(time.Second)
	}

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
	if vo.containerID == "" {
		return fmt.Errorf("no container ID")
	}

	inspectCmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", vo.containerID)
	running, err := inspectCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to inspect container: %w", err)
	}

	if strings.TrimSpace(string(running)) == "true" {
		return nil
	}

	return fmt.Errorf("container not running")
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
