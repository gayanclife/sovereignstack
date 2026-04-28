/*
Copyright 2026 SovereignStack Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package docker

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/gayanclife/sovereignstack/core"
)

const (
	containerPrefix = "ss-" // SovereignStack prefix for all managed containers
)

// RunningModel represents a model running in Docker
type RunningModel struct {
	ModelName   string
	ContainerID string
	Type        string // "gpu" or "cpu"
	Status      string // "running", "exited", "paused"
	Port        int
}

// GetContainerName returns the standard container name for a model
func GetContainerName(modelName string, isGPU bool) string {
	engineType := "cpu"
	if isGPU {
		engineType = "vllm"
	}
	// Sanitize model name: Docker only allows [a-zA-Z0-9][a-zA-Z0-9_.-]
	// Replace forward slashes and other invalid chars with dashes
	sanitized := strings.ReplaceAll(modelName, "/", "-")
	sanitized = strings.ReplaceAll(sanitized, " ", "-")
	return containerPrefix + engineType + "-" + sanitized
}

// GetRunningModels queries Docker for all SovereignStack managed containers
func GetRunningModels(ctx context.Context) ([]RunningModel, error) {
	// Query Docker for containers with our prefix using custom delimiter
	// Using ||| as delimiter since it won't appear in container names
	cmd := exec.CommandContext(ctx, "docker", "ps", "-a", "--format", "{{.Names}}|||{{.ID}}|||{{.Status}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := string(output)
		if strings.Contains(errMsg, "permission denied") {
			return nil, fmt.Errorf("Docker permission denied. Try: sudo usermod -aG docker $USER\nOr run with: sudo sovstack")
		}
		return nil, fmt.Errorf("failed to query Docker: %v (output: %s)", err, errMsg)
	}

	var models []RunningModel
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Split by custom delimiter
		parts := strings.Split(line, "|||")
		if len(parts) < 3 {
			continue
		}

		containerName := strings.TrimSpace(parts[0])
		containerID := strings.TrimSpace(parts[1])
		statusStr := strings.TrimSpace(parts[2])

		// Check if container matches our naming pattern
		if !strings.HasPrefix(containerName, containerPrefix) {
			continue
		}

		// Parse container type and model name from: ss-vllm-modelname or ss-cpu-modelname
		// Remove ss- prefix first
		nameWithoutPrefix := strings.TrimPrefix(containerName, containerPrefix)
		nameParts := strings.Split(nameWithoutPrefix, "-")
		if len(nameParts) < 2 {
			continue
		}

		engineType := nameParts[0]
		modelName := strings.Join(nameParts[1:], "-")

		// Determine status
		status := "unknown"
		if strings.Contains(statusStr, "Up") {
			status = "running"
		} else if strings.Contains(statusStr, "Exited") {
			status = "exited"
		}

		// Get port mapping for this container
		port := getContainerPort(ctx, containerID[:12])

		models = append(models, RunningModel{
			ModelName:   modelName,
			ContainerID: containerID,
			Type:        engineType,
			Status:      status,
			Port:        port,
		})
	}

	return models, nil
}

// getContainerPort extracts the mapped port from a container
func getContainerPort(ctx context.Context, containerID string) int {
	// Use docker inspect to get port bindings
	// Format: {{range .NetworkSettings.Ports}}{{.HostIp}}:{{index .}} {{end}}
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{range $p, $conf := .NetworkSettings.Ports}}{{(index $conf 0).HostPort}}{{end}}", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0
	}

	portStr := strings.TrimSpace(string(output))
	if portStr == "" {
		return 0
	}

	// Try to parse the port number
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return port
}

// InferenceConfig represents inference engine configuration
type InferenceConfig struct {
	ModelPath            string
	ModelName            string
	Quantization         core.QuantizationType
	GPUIndices           []int
	MaxModelLen          int
	ContextLength        int
	GPUMemoryUtilization float32
	TensorParallelSize   int
	Port                 int
	RebuildImage         bool
}

// InferenceEngine abstracts inference server implementations (vLLM, CPU, etc)
type InferenceEngine interface {
	Start(ctx context.Context, config InferenceConfig) (containerID string, err error)
	Stop(ctx context.Context) error
	Remove(ctx context.Context) error
	HealthCheck(ctx context.Context, port int) error
	IsRunning(ctx context.Context) (bool, error)
	GetLogs(ctx context.Context, tailLines int) (string, error)
}

// NewInferenceEngine creates the appropriate inference engine based on hardware
func NewInferenceEngine(hasGPU bool) InferenceEngine {
	if hasGPU {
		return NewVLLMOrchestrator()
	}
	return NewCPUInferenceOrchestrator()
}

// findFreePort returns an available port starting from the requested port
func findFreePort(startPort int) int {
	for port := startPort; port < startPort+100; port++ {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			listener.Close()
			return port
		}
	}
	return startPort // Fallback
}
