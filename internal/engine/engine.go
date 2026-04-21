package engine

import (
	"fmt"
	"os/exec"

	"github.com/gayanclife/sovereignstack/internal/hardware"
)

// PullModel pulls a model from Hugging Face
func PullModel(modelName string) error {
	// For simplicity, assume models are pulled via Docker or direct download
	// In a real implementation, this would use huggingface_hub or similar
	fmt.Printf("Pulling model %s...\n", modelName)

	// Simulate pulling
	cmd := exec.Command("echo", "Model", modelName, "pulled successfully")
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to pull model: %v", err)
	}

	return nil
}

// DeployModel deploys a model using vLLM in Docker
func DeployModel(modelName string) error {
	// Detect GPUs to optimize parameters
	gpus, err := hardware.DetectGPUs()
	if err != nil {
		return fmt.Errorf("failed to detect GPUs: %v", err)
	}

	if len(gpus) == 0 {
		return fmt.Errorf("no GPUs detected")
	}

	// Calculate GPU memory utilization (use 90% of available VRAM)
	totalVRAM := 0
	for _, gpu := range gpus {
		totalVRAM += gpu.VRAM
	}
	memoryUtilization := 0.9

	// Docker run command for vLLM
	dockerCmd := fmt.Sprintf(
		"docker run --gpus all --shm-size 1g -p 8000:8000 -v ~/.cache/huggingface:/root/.cache/huggingface "+
			"-e CUDA_VISIBLE_DEVICES=0 -e HF_TOKEN=$HF_TOKEN "+
			"vllm/vllm-openai:latest "+
			"--model %s --gpu-memory-utilization %.2f --host 0.0.0.0 --port 8000",
		modelName, memoryUtilization,
	)

	fmt.Printf("Running: %s\n", dockerCmd)

	cmd := exec.Command("sh", "-c", dockerCmd)
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start vLLM container: %v", err)
	}

	// Note: In production, you'd want to handle the process lifecycle better
	fmt.Println("vLLM container started in background")

	return nil
}
