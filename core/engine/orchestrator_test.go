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

func TestEngineRoomMultiModel(t *testing.T) {
	tests := []struct {
		name   string
		testFn func(t *testing.T, er *EngineRoom)
	}{
		{
			name: "GetRunningModels returns empty initially",
			testFn: func(t *testing.T, er *EngineRoom) {
				models := er.GetRunningModels()
				if len(models) != 0 {
					t.Errorf("expected 0 running models, got %d", len(models))
				}
			},
		},
		{
			name: "runningModels map is initialized",
			testFn: func(t *testing.T, er *EngineRoom) {
				if er.runningModels == nil {
					t.Error("runningModels map is nil")
				}
			},
		},
		{
			name: "engines map is initialized",
			testFn: func(t *testing.T, er *EngineRoom) {
				if er.engines == nil {
					t.Error("engines map is nil")
				}
			},
		},
		{
			name: "Status reflects empty running models",
			testFn: func(t *testing.T, er *EngineRoom) {
				ctx := context.Background()
				status := er.Status(ctx)
				if len(status.RunningModels) != 0 {
					t.Errorf("expected 0 models in status, got %d", len(status.RunningModels))
				}
				if status.Hardware == nil {
					t.Error("expected hardware info in status")
				}
				if status.ModelCacheDir == "" {
					t.Error("expected ModelCacheDir in status")
				}
			},
		},
	}

	er, err := NewEngineRoom(EngineConfig{
		ModelCacheDir: "/tmp/models",
		Port:          8000,
	})
	if err != nil {
		t.Fatalf("failed to create EngineRoom: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFn(t, er)
		})
	}
}

func TestStopModelNotRunning(t *testing.T) {
	er, err := NewEngineRoom(EngineConfig{
		ModelCacheDir: "/tmp/models",
		Port:          8000,
	})
	if err != nil {
		t.Fatalf("failed to create EngineRoom: %v", err)
	}

	ctx := context.Background()
	err = er.StopModel(ctx, "nonexistent-model")
	if err == nil {
		t.Error("expected error when stopping non-existent model, got nil")
	}

	expected := "model nonexistent-model is not running"
	if err.Error() != expected {
		t.Errorf("expected error '%s', got '%s'", expected, err.Error())
	}
}

func TestQuantizationTypeDetection(t *testing.T) {
	// Verify that core.QuantizationType is used correctly
	quant := core.QuantizationNone
	if quant != "none" {
		t.Errorf("expected QuantizationNone to be 'none', got '%s'", quant)
	}
}

func TestEngineRoomSystemInfo(t *testing.T) {
	er, err := NewEngineRoom(EngineConfig{
		ModelCacheDir: "/tmp/models",
		Port:          8000,
	})
	if err != nil {
		t.Fatalf("failed to create EngineRoom: %v", err)
	}

	info := er.GetSystemInfo()
	if info == nil {
		t.Error("expected non-nil system info")
	}
}
