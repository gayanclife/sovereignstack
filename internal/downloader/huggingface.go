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
package downloader

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gayanclife/sovereignstack/internal/config"
)

// HFDownloader handles Hugging Face model downloads
type HFDownloader struct {
	token      string
	cacheDir   string
	httpClient *http.Client
	auditor    *config.AuditLogger
}


// NewHFDownloader creates a new Hugging Face downloader
func NewHFDownloader(cacheDir string, auditor *config.AuditLogger) *HFDownloader {
	token := os.Getenv("HF_TOKEN")
	if auditor != nil && token != "" {
		auditor.LogTokenAccess("environment_variable")
	}

	return &HFDownloader{
		token:      token,
		cacheDir:   cacheDir,
		auditor:    auditor,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// DownloadModel downloads a model from Hugging Face
func (d *HFDownloader) DownloadModel(modelID string, destDir string) error {
	// Create destination directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Try common model file names
	commonFiles := []string{
		"model.safetensors",
		"pytorch_model.bin",
		"model.bin",
		"adapter_model.bin",
		"model.gguf",
		"model.pt",
	}

	fmt.Printf("   Checking for model files in %s...\n", modelID)

	modelFileCount := 0
	for _, filename := range commonFiles {
		url := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", modelID, filename)
		destPath := filepath.Join(destDir, filename)

		// Check if file exists by making HEAD request
		req, err := http.NewRequest("HEAD", url, nil)
		if err != nil {
			continue
		}

		if d.token != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", d.token))
		}

		resp, err := d.httpClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}
		resp.Body.Close()

		// File exists, download it
		modelFileCount++
		fmt.Printf("   %d. %s\n", modelFileCount, filename)

		if err := d.downloadFile(url, destPath, resp.ContentLength, modelID); err != nil {
			fmt.Printf("   ⚠ Failed to download %s: %v\n", filename, err)
			if d.auditor != nil {
				d.auditor.LogModelDownload(modelID, "failed", filename)
			}
			continue
		}

		fmt.Printf("   ✓ Downloaded\n")
	}

	if modelFileCount == 0 {
		if d.auditor != nil {
			d.auditor.LogModelDownload(modelID, "failed", "no_files_found")
		}
		return fmt.Errorf("no model files found for %s", modelID)
	}

	fmt.Printf("   ✓ Download complete: %d files\n", modelFileCount)
	if d.auditor != nil {
		d.auditor.LogModelDownload(modelID, "success", fmt.Sprintf("%d files", modelFileCount))
	}
	return nil
}

// downloadFile downloads a single file with progress and resume support
func (d *HFDownloader) downloadFile(url, destPath string, totalSize int64, modelID string) error {
	// Check if file already exists
	existingFile, err := os.Stat(destPath)
	startByte := int64(0)

	if err == nil {
		// File exists, check if we can resume
		if existingFile.Size() == totalSize {
			fmt.Printf("   ✓ Already downloaded (verified size)\n")
			return nil
		}
		if existingFile.Size() < totalSize {
			// Resume download
			startByte = existingFile.Size()
			fmt.Printf("   ↻ Resuming from %.2f MB\n", float64(startByte)/(1024*1024))
		}
	}

	// Create request with range header for resume
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	if startByte > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startByte))
	}

	if d.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", d.token))
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		if resp.StatusCode == 401 {
			return fmt.Errorf("unauthorized - this model requires a Hugging Face token. Set HF_TOKEN=your_token")
		}
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Open file for writing (append if resuming)
	flags := os.O_WRONLY | os.O_CREATE
	if startByte > 0 {
		flags = os.O_APPEND | os.O_WRONLY
	} else {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}

	file, err := os.OpenFile(destPath, flags, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Copy with progress
	written, err := io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("download interrupted: %w", err)
	}

	// Verify complete
	finalSize := startByte + written
	if finalSize != totalSize {
		return fmt.Errorf("incomplete download: got %d bytes, expected %d bytes", finalSize, totalSize)
	}

	return nil
}

// isModelFile checks if a file is a model weight file (not metadata)
func isModelFile(filename string) bool {
	// Include model files
	if strings.HasSuffix(filename, ".safetensors") {
		return true
	}
	if strings.HasSuffix(filename, ".bin") {
		return true
	}
	if strings.HasSuffix(filename, ".pt") {
		return true
	}
	if strings.HasSuffix(filename, ".pth") {
		return true
	}
	if strings.HasSuffix(filename, ".gguf") {
		return true
	}

	// Exclude metadata files
	if filename == ".gitattributes" {
		return false
	}
	if strings.HasSuffix(filename, ".md") {
		return false
	}
	if strings.HasSuffix(filename, ".json") && !strings.Contains(filename, ".safetensors") {
		return false
	}
	if filename == "README.md" {
		return false
	}
	if filename == "LICENSE" {
		return false
	}

	// For other files, be conservative (include only known weight formats)
	return false
}
