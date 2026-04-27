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

	"github.com/gayanclife/sovereignstack/core"
)

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
