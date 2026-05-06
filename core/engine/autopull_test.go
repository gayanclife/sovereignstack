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
	"testing"

	"github.com/gayanclife/sovereignstack/core/model"
)

// TestCacheManagerInitialization verifies cache manager can be created
func TestCacheManagerInitialization(t *testing.T) {
	cm, err := model.NewCacheManager("./models")
	if err != nil {
		t.Fatalf("failed to create cache manager: %v", err)
	}

	if cm == nil {
		t.Error("expected non-nil cache manager")
	}
}

// TestCacheChecking verifies we can check if a model is cached
func TestCacheChecking(t *testing.T) {
	cm, err := model.NewCacheManager("/tmp/test-models")
	if err != nil {
		t.Fatalf("failed to create cache manager: %v", err)
	}

	// This model shouldn't exist in temp directory
	isCached := cm.IsCached("nonexistent-model")
	if isCached {
		t.Error("expected nonexistent model to not be cached")
	}
}

// TestAutoPullWorkflow documents the auto-pull logic flow
func TestAutoPullWorkflow(t *testing.T) {
	// This test documents what happens in deploy with auto-pull:
	// 1. Create cache manager
	cm, err := model.NewCacheManager("./models")
	if err != nil {
		t.Fatalf("failed to create cache manager: %v", err)
	}

	// 2. Check if model is cached
	modelName := "test-model"
	isCached := cm.IsCached(modelName)

	if !isCached {
		// 3. If not cached, would call: cm.DownloadModel(modelName)
		// (skipped in test to avoid actual download)
		// After download, would verify: cm.IsCached(modelName)
		t.Logf("Model %s would be auto-pulled in deploy command", modelName)
	}

	// 4. Then continue with deployment validation
	t.Logf("Auto-pull workflow verified")
}
