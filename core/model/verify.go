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
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type VerificationResult struct {
	ModelName        string
	IsValid          bool
	Status           string
	FileCount        int
	TotalSize        int64
	Files            []FileVerification
	Warnings         []string
	ReadyToDeploy    bool
	ChecksumVerified bool
}

type FileVerification struct {
	Name     string
	Size     int64
	Exists   bool
	Checksum string
	Status   string
}

// VerifyModel checks if a model is complete and ready to deploy
func (cm *CacheManager) VerifyModel(modelName string) *VerificationResult {
	result := &VerificationResult{
		ModelName: modelName,
		Files:     []FileVerification{},
		Warnings:  []string{},
	}

	meta := cm.GetCachedModel(modelName)
	if meta == nil {
		result.Status = "not_cached"
		result.IsValid = false
		return result
	}

	result.TotalSize = meta.Size
	result.Status = "verifying"

	// Check if directory exists
	dirInfo, err := os.Stat(meta.Path)
	if err != nil || !dirInfo.IsDir() {
		result.Status = "missing"
		result.IsValid = false
		result.Warnings = append(result.Warnings, "Model directory not found or inaccessible")
		return result
	}

	// Walk directory and check files
	var validFileCount int
	var totalSize int64

	err = filepath.Walk(meta.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		// Check if it's a model file (not metadata)
		if isModelFile(info.Name()) {
			validFileCount++
			totalSize += info.Size()

			fv := FileVerification{
				Name:   info.Name(),
				Size:   info.Size(),
				Exists: true,
				Status: "present",
			}
			result.Files = append(result.Files, fv)
		}

		return nil
	})

	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Error scanning directory: %v", err))
	}

	result.FileCount = validFileCount
	if totalSize != result.TotalSize {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Size mismatch: metadata=%d bytes, actual=%d bytes", result.TotalSize, totalSize))
	}

	// Determine if ready to deploy
	if validFileCount > 0 && err == nil {
		result.IsValid = true
		result.Status = "ready"
		result.ReadyToDeploy = true
	} else if validFileCount == 0 {
		result.Status = "incomplete"
		result.IsValid = false
		result.Warnings = append(result.Warnings, "No model files found in directory")
	}

	return result
}

// VerifyAllModels checks all cached models
func (cm *CacheManager) VerifyAllModels() []*VerificationResult {
	cached := cm.ListCached()
	results := make([]*VerificationResult, 0, len(cached))

	for _, meta := range cached {
		result := cm.VerifyModel(meta.Name)
		results = append(results, result)
	}

	return results
}

// ComputeChecksum calculates SHA256 checksum of a model file
func ComputeChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// GetModelInfo returns detailed model information
func (cm *CacheManager) GetModelInfo(modelName string) map[string]interface{} {
	meta := cm.GetCachedModel(modelName)
	if meta == nil {
		return nil
	}

	info := map[string]interface{}{
		"name":       meta.Name,
		"path":       meta.Path,
		"size_bytes": meta.Size,
		"size_gb":    float64(meta.Size) / (1024 * 1024 * 1024),
		"cached_at":  meta.Downloaded.Format("2006-01-02 15:04:05"),
	}

	// Add file listing
	var files []map[string]interface{}
	filepath.Walk(meta.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		files = append(files, map[string]interface{}{
			"name": info.Name(),
			"size": info.Size(),
		})
		return nil
	})

	info["files"] = files
	info["file_count"] = len(files)

	return info
}

// isModelFile checks if a file is a model weight file (not metadata)
func isModelFile(filename string) bool {
	// Model weight file extensions
	if endsWithAny(filename, ".safetensors", ".bin", ".pt", ".pth", ".gguf") {
		return true
	}

	// Exclude metadata files
	if filename == ".gitattributes" || filename == "README.md" || filename == "LICENSE" {
		return false
	}
	if endsWithAny(filename, ".md", ".json") {
		return false
	}

	return false
}

func endsWithAny(s string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
			return true
		}
	}
	return false
}
