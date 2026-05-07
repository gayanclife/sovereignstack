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
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/gayanclife/sovereignstack/core"
)

// RunningModelRecord persists model state to disk
type RunningModelRecord struct {
	ModelName       string                `json:"model_name"`
	ContainerID     string                `json:"container_id"`
	Quantization    core.QuantizationType `json:"quantization"`
	StartedAt       time.Time             `json:"started_at"`
	IsHealthy       bool                  `json:"is_healthy"`
	LastHealthCheck time.Time             `json:"last_health_check"`
}

// getRunningModelsFile returns path to running models file
func getRunningModelsFile() string {
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".sovereignstack")
	os.MkdirAll(configDir, 0755)
	return filepath.Join(configDir, "running_models.json")
}

// saveRunningModels persists running models to disk
func saveRunningModels(models map[string]*core.ModelInstance) error {
	records := make([]RunningModelRecord, 0)
	for _, model := range models {
		records = append(records, RunningModelRecord{
			ModelName:       model.ModelName,
			ContainerID:     model.ContainerID,
			Quantization:    model.Quantization,
			StartedAt:       model.StartedAt,
			IsHealthy:       model.IsHealthy,
			LastHealthCheck: model.LastHealthCheck,
		})
	}

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(getRunningModelsFile(), data, 0644)
}

// loadRunningModels loads running models from disk
func loadRunningModels() map[string]*core.ModelInstance {
	file := getRunningModelsFile()
	data, err := os.ReadFile(file)
	if err != nil {
		return make(map[string]*core.ModelInstance) // Empty map if file doesn't exist
	}

	var records []RunningModelRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return make(map[string]*core.ModelInstance)
	}

	models := make(map[string]*core.ModelInstance)
	for _, record := range records {
		models[record.ModelName] = &core.ModelInstance{
			ID:              record.ContainerID,
			ModelName:       record.ModelName,
			Quantization:    record.Quantization,
			ContainerID:     record.ContainerID,
			StartedAt:       record.StartedAt,
			IsHealthy:       record.IsHealthy,
			LastHealthCheck: record.LastHealthCheck,
		}
	}

	return models
}

// (clearRunningModels was unused; the on-disk state file is invalidated
// by Stop() rewriting it with a smaller running set, so an explicit
// clear is unnecessary.)
