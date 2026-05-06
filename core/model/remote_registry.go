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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gayanclife/sovereignstack/core"
)

// RemoteRegistry manages fetching and caching models from a remote API
type RemoteRegistry struct {
	URL       string
	CacheDir  string
	CacheTTL  time.Duration
	Client    *http.Client
	cacheFile string
}

// NewRemoteRegistry creates a new remote registry client
func NewRemoteRegistry(url string) *RemoteRegistry {
	homeDir, _ := os.UserHomeDir()
	cacheDir := filepath.Join(homeDir, ".sovereignstack")

	return &RemoteRegistry{
		URL:       url,
		CacheDir:  cacheDir,
		CacheTTL:  24 * time.Hour, // Cache for 24 hours
		Client:    &http.Client{Timeout: 10 * time.Second},
		cacheFile: filepath.Join(cacheDir, "models-remote.json"),
	}
}

// FetchAndCache fetches models from remote API and caches locally
// Returns models from cache if valid, or from API if fresh
// Falls back to cache if API unavailable
func (r *RemoteRegistry) FetchAndCache() (map[string]*core.ModelMetadata, error) {
	// Check if cache exists and is fresh
	if cached, err := r.loadCache(); err == nil && r.isCacheValid() {
		return cached, nil
	}

	// Attempt to fetch from remote API
	models, err := r.fetchFromAPI()
	if err != nil {
		// If fetch fails, return cached models if available
		if cached, cacheErr := r.loadCache(); cacheErr == nil {
			fmt.Printf("⚠️  Failed to fetch remote models, using cached version: %v\n", err)
			return cached, nil
		}
		// Both failed, return the API error
		return nil, err
	}

	// Successfully fetched, save to cache
	_ = r.saveCache(models)

	return models, nil
}

// FetchOnly fetches from API without caching fallback (used for --refresh)
func (r *RemoteRegistry) FetchOnly() (map[string]*core.ModelMetadata, error) {
	models, err := r.fetchFromAPI()
	if err != nil {
		return nil, err
	}

	// Save to cache on success
	_ = r.saveCache(models)

	return models, nil
}

// fetchFromAPI makes HTTP request to remote registry API
func (r *RemoteRegistry) fetchFromAPI() (map[string]*core.ModelMetadata, error) {
	resp, err := r.Client.Get(r.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from remote registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("remote registry returned %d: %s", resp.StatusCode, string(body))
	}

	// Parse response as YAML-like registry
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read remote response: %w", err)
	}

	models, err := LoadModelsFromYAML(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse remote models: %w", err)
	}

	return models, nil
}

// loadCache loads models from local cache file
func (r *RemoteRegistry) loadCache() (map[string]*core.ModelMetadata, error) {
	data, err := os.ReadFile(r.cacheFile)
	if err != nil {
		return nil, err
	}

	var cached struct {
		Timestamp time.Time                      `json:"timestamp"`
		Models    map[string]*core.ModelMetadata `json:"models"`
	}

	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, err
	}

	return cached.Models, nil
}

// saveCache saves models to local cache file
func (r *RemoteRegistry) saveCache(models map[string]*core.ModelMetadata) error {
	// Create cache directory if needed
	os.MkdirAll(r.CacheDir, 0755)

	cached := struct {
		Timestamp time.Time                      `json:"timestamp"`
		Models    map[string]*core.ModelMetadata `json:"models"`
	}{
		Timestamp: time.Now(),
		Models:    models,
	}

	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(r.cacheFile, data, 0644)
}

// isCacheValid checks if cache file is still valid (not expired)
func (r *RemoteRegistry) isCacheValid() bool {
	info, err := os.Stat(r.cacheFile)
	if err != nil {
		return false
	}

	return time.Since(info.ModTime()) < r.CacheTTL
}

// ClearCache removes the local cache
func (r *RemoteRegistry) ClearCache() error {
	return os.Remove(r.cacheFile)
}

// GetCacheAge returns how old the cache is, or error if not cached
func (r *RemoteRegistry) GetCacheAge() (time.Duration, error) {
	info, err := os.Stat(r.cacheFile)
	if err != nil {
		return 0, err
	}

	return time.Since(info.ModTime()), nil
}

// MergeRegistries merges local and remote models, with remote taking precedence
func MergeRegistries(local, remote map[string]*core.ModelMetadata) map[string]*core.ModelMetadata {
	merged := make(map[string]*core.ModelMetadata)

	// Add all local models
	for name, model := range local {
		merged[name] = model
	}

	// Add/override with remote models
	for name, model := range remote {
		merged[name] = model
	}

	return merged
}

// FilterByHardware returns models compatible with given hardware
func FilterByHardware(models map[string]*core.ModelMetadata, hasGPU bool, availableVRAM int64, systemRAM int64) []*core.ModelMetadata {
	var compatible []*core.ModelMetadata

	for _, model := range models {
		// Filter by hardware target
		if hasGPU {
			// GPU available - accept GPU and both targets
			if model.HardwareTarget != core.HardwareGPUOnly && model.HardwareTarget != core.HardwareBoth {
				continue
			}

			// Check VRAM requirements for most efficient quantization
			if minVRAM := getMinVRAM(model.RequiredVRAM); minVRAM > availableVRAM {
				continue
			}
		} else {
			// No GPU - only accept CPU and both targets
			if model.HardwareTarget == core.HardwareGPUOnly {
				continue
			}

			// Check system RAM requirement
			if model.MinimumSystemRAM > systemRAM {
				continue
			}
		}

		compatible = append(compatible, model)
	}

	return compatible
}

// getMinVRAM returns the minimum VRAM requirement across all quantizations
func getMinVRAM(requiredVRAM map[string]int64) int64 {
	if len(requiredVRAM) == 0 {
		return 0
	}

	var minVRAM int64 = 9223372036854775807 // Max int64
	for _, vram := range requiredVRAM {
		if vram < minVRAM {
			minVRAM = vram
		}
	}

	return minVRAM
}
