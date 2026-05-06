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
	"testing"

	"github.com/gayanclife/sovereignstack/core"
)

func TestInferenceConfigStructure(t *testing.T) {
	config := InferenceConfig{
		ModelPath:            "/path/to/model",
		ModelName:            "test-model",
		Quantization:         core.QuantizationNone,
		GPUIndices:           []int{0, 1},
		ContextLength:        4096,
		GPUMemoryUtilization: 0.9,
		TensorParallelSize:   2,
		Port:                 8000,
		RebuildImage:         false,
	}

	// Verify all fields are set
	if config.ModelPath == "" {
		t.Error("ModelPath should not be empty")
	}
	if config.ModelName == "" {
		t.Error("ModelName should not be empty")
	}
	if config.Port != 8000 {
		t.Errorf("expected port 8000, got %d", config.Port)
	}
	if len(config.GPUIndices) != 2 {
		t.Errorf("expected 2 GPU indices, got %d", len(config.GPUIndices))
	}
}

func TestNewInferenceEngineWithGPU(t *testing.T) {
	engine := NewInferenceEngine(true)

	if engine == nil {
		t.Error("expected non-nil engine")
	}

	// Should return VLLM orchestrator for GPU
	_, isVLLM := engine.(*VLLMOrchestrator)
	if !isVLLM {
		t.Errorf("expected VLLMOrchestrator for GPU, got %T", engine)
	}
}

func TestNewInferenceEngineWithoutGPU(t *testing.T) {
	engine := NewInferenceEngine(false)

	if engine == nil {
		t.Error("expected non-nil engine")
	}

	// Should return CPU orchestrator for CPU-only
	_, isCPU := engine.(*CPUInferenceOrchestrator)
	if !isCPU {
		t.Errorf("expected CPUInferenceOrchestrator for CPU, got %T", engine)
	}
}

func TestFindFreePort(t *testing.T) {
	startPort := 8000
	port := findFreePort(startPort)

	if port < startPort {
		t.Errorf("expected port >= %d, got %d", startPort, port)
	}

	if port >= startPort+100 {
		t.Errorf("port search exceeded range: %d", port)
	}
}

func TestGPUSpecBuilder(t *testing.T) {
	tests := []struct {
		name       string
		gpuIndices []int
		expected   string
	}{
		{
			name:       "Empty indices",
			gpuIndices: []int{},
			expected:   "all",
		},
		{
			name:       "Single GPU",
			gpuIndices: []int{0},
			expected:   "device=0",
		},
		{
			name:       "Multiple GPUs",
			gpuIndices: []int{0, 1, 2},
			expected:   "device=0,1,2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildGPUSpec(tt.gpuIndices)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestCUDADevicesBuilder(t *testing.T) {
	tests := []struct {
		name       string
		gpuIndices []int
		expected   string
	}{
		{
			name:       "Empty indices",
			gpuIndices: []int{},
			expected:   "0",
		},
		{
			name:       "Single GPU",
			gpuIndices: []int{0},
			expected:   "0",
		},
		{
			name:       "Multiple GPUs",
			gpuIndices: []int{0, 2, 3},
			expected:   "0,2,3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildCUDADevices(tt.gpuIndices)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestVLLMOrchestratorCreation(t *testing.T) {
	vo := NewVLLMOrchestrator()

	if vo == nil {
		t.Error("expected non-nil VLLMOrchestrator")
	}

	if vo.containerID != "" {
		t.Error("expected empty containerID initially")
	}
}

func TestCPUInferenceOrchestratorCreation(t *testing.T) {
	co := NewCPUInferenceOrchestrator()

	if co == nil {
		t.Error("expected non-nil CPUInferenceOrchestrator")
	}

	if co.containerID != "" {
		t.Error("expected empty containerID initially")
	}
}

func TestQuantizationStringConversion(t *testing.T) {
	tests := []struct {
		quant core.QuantizationType
		name  string
	}{
		{core.QuantizationNone, "none"},
		{core.QuantizationAWQ, "awq"},
		{core.QuantizationGPTQ, "gptq"},
		{core.QuantizationINT8, "int8"},
	}

	for _, tt := range tests {
		if string(tt.quant) != tt.name {
			t.Errorf("expected %s, got %s", tt.name, string(tt.quant))
		}
	}
}
