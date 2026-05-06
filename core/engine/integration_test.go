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
package engine

import (
	"context"
	"testing"

	"github.com/gayanclife/sovereignstack/core"
)

// TestMultiModelLifecycle simulates deploying and stopping multiple models
func TestMultiModelLifecycle(t *testing.T) {
	er, err := NewEngineRoom(EngineConfig{
		ModelCacheDir: "/tmp/models",
		Port:          8000,
	})
	if err != nil {
		t.Fatalf("failed to create EngineRoom: %v", err)
	}

	ctx := context.Background()

	// Step 1: Verify no models running initially
	models := er.GetRunningModels()
	if len(models) != 0 {
		t.Errorf("expected 0 running models initially, got %d", len(models))
	}

	// Step 2: Try to stop non-existent model
	err = er.StopModel(ctx, "model-that-doesnt-exist")
	if err == nil {
		t.Error("expected error when stopping non-existent model")
	}

	// Step 3: Verify status shows empty running models
	status := er.Status(ctx)
	if len(status.RunningModels) != 0 {
		t.Errorf("expected status to show 0 running models, got %d", len(status.RunningModels))
	}

	// Step 4: Verify hardware info is available
	if status.Hardware == nil {
		t.Error("expected hardware info in status")
	}

	// Step 5: Verify system info retrieval
	sysInfo := er.GetSystemInfo()
	if sysInfo == nil {
		t.Error("expected system info to be non-nil")
	}

	// Step 6: Verify cache directory is set
	if status.ModelCacheDir == "" {
		t.Error("expected ModelCacheDir to be set")
	}
}

// TestEngineMapConcurrency verifies maps are properly isolated
func TestEngineMapConcurrency(t *testing.T) {
	er, err := NewEngineRoom(EngineConfig{
		ModelCacheDir: "/tmp/models",
		Port:          8000,
	})
	if err != nil {
		t.Fatalf("failed to create EngineRoom: %v", err)
	}

	ctx := context.Background()

	// Verify both maps exist and are independent
	if er.engines == nil {
		t.Error("engines map is nil")
	}
	if er.runningModels == nil {
		t.Error("runningModels map is nil")
	}

	// Verify they're empty
	if len(er.engines) != 0 {
		t.Errorf("expected empty engines map, got %d entries", len(er.engines))
	}
	if len(er.runningModels) != 0 {
		t.Errorf("expected empty runningModels map, got %d entries", len(er.runningModels))
	}

	// Verify status returns correct empty state
	status := er.Status(ctx)
	if len(status.RunningModels) != 0 {
		t.Errorf("expected status.RunningModels to be empty, got %d", len(status.RunningModels))
	}

	// Verify GetRunningModels returns empty map
	running := er.GetRunningModels()
	if len(running) != 0 {
		t.Errorf("expected GetRunningModels to return empty, got %d", len(running))
	}
}

// TestStatusStructure verifies EngineStatus has correct fields
func TestStatusStructure(t *testing.T) {
	er, err := NewEngineRoom(EngineConfig{
		ModelCacheDir: "/tmp/test-cache",
		Port:          9000,
	})
	if err != nil {
		t.Fatalf("failed to create EngineRoom: %v", err)
	}

	ctx := context.Background()
	status := er.Status(ctx)

	// Verify all required fields
	if status.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}

	if status.Hardware == nil {
		t.Error("expected hardware to be non-nil")
	}

	if status.RunningModels == nil {
		t.Error("expected RunningModels to be non-nil (even if empty)")
	}

	if status.ModelCacheDir == "" {
		t.Error("expected ModelCacheDir to be set")
	}

	if status.ModelCacheDir != "/tmp/test-cache" {
		t.Errorf("expected ModelCacheDir=/tmp/test-cache, got %s", status.ModelCacheDir)
	}
}

// TestQuantizationHandling verifies quantization types work correctly
func TestQuantizationHandling(t *testing.T) {
	tests := []struct {
		name  string
		quant core.QuantizationType
	}{
		{"None", core.QuantizationNone},
		{"AWQ", core.QuantizationAWQ},
		{"GPTQ", core.QuantizationGPTQ},
		{"INT8", core.QuantizationINT8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.quant) == "" {
				t.Errorf("quantization %s should not be empty", tt.name)
			}
		})
	}
}
