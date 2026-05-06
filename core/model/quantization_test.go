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
package model

import (
	"testing"

	"github.com/gayanclife/sovereignstack/core"
)

func TestQuantizationCalculator(t *testing.T) {
	tests := []struct {
		name          string
		availableVRAM int64
		paramCount    int64
		shouldSucceed bool
	}{
		{
			name:          "Large VRAM - 7B model fits",
			availableVRAM: 16 * 1024 * 1024 * 1024, // 16GB
			paramCount:    7e9,                     // 7B model
			shouldSucceed: true,
		},
		{
			name:          "Small VRAM - 1B model fits",
			availableVRAM: 2 * 1024 * 1024 * 1024, // 2GB
			paramCount:    1e9,                    // 1B model
			shouldSucceed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qc := NewQuantizationCalculator(tt.availableVRAM)

			metadata := &core.ModelMetadata{
				Name:           "test-model",
				ParameterCount: tt.paramCount,
			}

			quant, err := qc.SuggestQuantization(metadata)

			if tt.shouldSucceed && err != nil {
				t.Errorf("expected success, got error: %v", err)
			}

			if !tt.shouldSucceed && err == nil {
				t.Errorf("expected failure, but succeeded with quantization: %s", quant)
			}

			if tt.shouldSucceed && string(quant) == "" {
				t.Error("expected non-empty quantization type")
			}
		})
	}
}

func TestGetQuantizationProfiles(t *testing.T) {
	qc := NewQuantizationCalculator(16 * 1024 * 1024 * 1024) // 16GB VRAM

	profiles := qc.GetQuantizationProfiles(7e9) // 7B params

	if len(profiles) == 0 {
		t.Error("expected non-empty profiles")
	}

	// Verify all profiles are present
	expectedTypes := []core.QuantizationType{
		core.QuantizationNone,
		core.QuantizationAWQ,
		core.QuantizationGPTQ,
		core.QuantizationINT8,
	}

	for _, expectedType := range expectedTypes {
		found := false
		for _, profile := range profiles {
			if profile.Type == expectedType {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing profile for quantization type: %s", expectedType)
		}
	}

	// Verify INT8 requires less VRAM than FP16
	int8Vram := int64(0)
	fp16Vram := int64(0)
	for _, profile := range profiles {
		if profile.Type == core.QuantizationINT8 {
			int8Vram = profile.VRAMRequired
		}
		if profile.Type == core.QuantizationNone {
			fp16Vram = profile.VRAMRequired
		}
	}

	if int8Vram >= fp16Vram {
		t.Errorf("INT8 (%d bytes) should require less VRAM than FP16 (%d bytes)", int8Vram, fp16Vram)
	}
}

func TestQuantizationProfileQuality(t *testing.T) {
	qc := NewQuantizationCalculator(16 * 1024 * 1024 * 1024)
	profiles := qc.GetQuantizationProfiles(7e9)

	// Check quality levels
	qualityMap := make(map[core.QuantizationType]string)
	for _, profile := range profiles {
		qualityMap[profile.Type] = profile.Quality
	}

	// AWQ should have high quality
	if qualityMap[core.QuantizationAWQ] != "high" {
		t.Errorf("expected AWQ to have 'high' quality, got %s", qualityMap[core.QuantizationAWQ])
	}

	// INT8 should have medium quality
	if qualityMap[core.QuantizationINT8] != "medium" {
		t.Errorf("expected INT8 to have 'medium' quality, got %s", qualityMap[core.QuantizationINT8])
	}
}

func TestQuantizationVRAMScaling(t *testing.T) {
	// Test that smaller models require less VRAM
	qc := NewQuantizationCalculator(16 * 1024 * 1024 * 1024)

	profiles1B := qc.GetQuantizationProfiles(1e9)
	profiles7B := qc.GetQuantizationProfiles(7e9)

	if len(profiles1B) == 0 || len(profiles7B) == 0 {
		t.Fatal("expected profiles for both models")
	}

	// For the same quantization, 1B should use less VRAM than 7B
	vram1B := profiles1B[0].VRAMRequired
	vram7B := profiles7B[0].VRAMRequired

	if vram1B >= vram7B {
		t.Errorf("1B model (%d bytes) should use less VRAM than 7B model (%d bytes)", vram1B, vram7B)
	}
}

func TestSmallModelFitsInSmallVRAM(t *testing.T) {
	// 1B model with INT8 needs: 1e9 * 1 byte * 1.15 = 1.15GB
	qc := NewQuantizationCalculator(2 * 1024 * 1024 * 1024) // 2GB

	metadata := &core.ModelMetadata{
		Name:           "small-model",
		ParameterCount: 1e9, // 1B model
	}

	quant, err := qc.SuggestQuantization(metadata)
	if err != nil {
		t.Errorf("expected success: 1B model should fit in 2GB, got error: %v", err)
	}

	if string(quant) == "" {
		t.Error("expected valid quantization type")
	}
}
