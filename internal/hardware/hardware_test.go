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
package hardware

import (
	"runtime"
	"testing"
)

func TestGPUStructure(t *testing.T) {
	gpu := GPU{
		Index:          0,
		Name:           "RTX 4090",
		VRAM:           24 * 1024 * 1024 * 1024, // 24GB
		VRAMAvailable:  20 * 1024 * 1024 * 1024, // 20GB
		Driver:         "535.0",
		CUDACapability: "8.9",
		Temperature:    65.5,
	}

	if gpu.Index != 0 {
		t.Error("GPU index not set correctly")
	}
	if gpu.Name == "" {
		t.Error("GPU name should not be empty")
	}
	if gpu.VRAM == 0 {
		t.Error("GPU VRAM should not be zero")
	}
	if gpu.VRAMAvailable > gpu.VRAM {
		t.Error("available VRAM should not exceed total VRAM")
	}
}

func TestSystemHardwareStructure(t *testing.T) {
	hw := &SystemHardware{
		GPUs: []GPU{
			{
				Index: 0,
				Name:  "RTX 4090",
				VRAM:  24 * 1024 * 1024 * 1024,
			},
		},
		TotalVRAM:       24 * 1024 * 1024 * 1024,
		TotalAvailable:  20 * 1024 * 1024 * 1024,
		CPUCores:        16,
		SystemRAM:       64 * 1024 * 1024 * 1024,
		CUDAInstalled:   true,
		CUDAVersion:     "12.1",
		DockerInstalled: true,
	}

	if len(hw.GPUs) == 0 {
		t.Error("GPUs should not be empty")
	}
	if hw.CPUCores == 0 {
		t.Error("CPU cores should be detected")
	}
	if hw.SystemRAM == 0 {
		t.Error("System RAM should be detected")
	}
}

func TestGetSystemHardware(t *testing.T) {
	hw, _ := GetSystemHardware()

	// GetSystemHardware should not fail even if nvidia-smi is not available
	if hw == nil {
		t.Error("expected non-nil hardware info")
	}

	// Verify basic hardware detection
	if hw.CPUCores == 0 {
		t.Error("expected CPU cores to be detected")
	}

	if hw.SystemRAM == 0 {
		t.Error("expected system RAM to be detected")
	}

	// If no GPUs, TotalVRAM should be 0
	if len(hw.GPUs) == 0 && hw.TotalVRAM != 0 {
		t.Error("TotalVRAM should be 0 if no GPUs detected")
	}
}

func TestCPUDetection(t *testing.T) {
	cores := runtime.NumCPU()

	if cores <= 0 {
		t.Error("expected at least 1 CPU core")
	}

	hw, _ := GetSystemHardware()
	if hw.CPUCores != cores {
		t.Errorf("expected %d CPU cores, got %d", cores, hw.CPUCores)
	}
}

func TestSystemRAMDetection(t *testing.T) {
	ram := GetSystemRAM()

	if ram <= 0 {
		t.Error("expected system RAM to be detected")
	}

	// System should have at least 256MB
	minRAM := int64(256 * 1024 * 1024)
	if ram < minRAM {
		t.Errorf("expected at least %d bytes, got %d", minRAM, ram)
	}
}

func TestGPUTotalVRAMCalculation(t *testing.T) {
	hw := &SystemHardware{
		GPUs: []GPU{
			{Index: 0, VRAM: 24 * 1024 * 1024 * 1024},
			{Index: 1, VRAM: 24 * 1024 * 1024 * 1024},
			{Index: 2, VRAM: 48 * 1024 * 1024 * 1024},
		},
	}

	totalVRAM := int64(0)
	for _, gpu := range hw.GPUs {
		totalVRAM += gpu.VRAM
	}

	expected := int64(96 * 1024 * 1024 * 1024) // 96GB
	if totalVRAM != expected {
		t.Errorf("expected total VRAM %d, got %d", expected, totalVRAM)
	}
}

func TestEmptyGPUList(t *testing.T) {
	hw := &SystemHardware{
		GPUs:            []GPU{},
		CPUCores:        8,
		SystemRAM:       32 * 1024 * 1024 * 1024,
		CUDAInstalled:   false,
		DockerInstalled: true,
	}

	if len(hw.GPUs) != 0 {
		t.Error("expected empty GPU list")
	}

	if hw.TotalVRAM != 0 {
		t.Error("TotalVRAM should be 0 with no GPUs")
	}

	// CPU-only system should still be valid
	if hw.CPUCores == 0 || hw.SystemRAM == 0 {
		t.Error("CPU-only system should have CPU cores and RAM")
	}
}

func TestMultiGPUScenario(t *testing.T) {
	gpus := []GPU{
		{
			Index:         0,
			Name:          "GPU 0",
			VRAM:          24 * 1024 * 1024 * 1024,
			VRAMAvailable: 20 * 1024 * 1024 * 1024,
		},
		{
			Index:         1,
			Name:          "GPU 1",
			VRAM:          24 * 1024 * 1024 * 1024,
			VRAMAvailable: 22 * 1024 * 1024 * 1024,
		},
	}

	hw := &SystemHardware{
		GPUs:           gpus,
		TotalVRAM:      48 * 1024 * 1024 * 1024,
		TotalAvailable: 42 * 1024 * 1024 * 1024,
		CPUCores:       16,
	}

	if len(hw.GPUs) != 2 {
		t.Errorf("expected 2 GPUs, got %d", len(hw.GPUs))
	}

	if hw.TotalAvailable >= hw.TotalVRAM {
		t.Error("available VRAM should not exceed total VRAM")
	}
}
