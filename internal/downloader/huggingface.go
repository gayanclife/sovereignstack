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
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gayanclife/sovereignstack/internal/config"
	"github.com/schollz/progressbar/v3"
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
		token:    token,
		cacheDir: cacheDir,
		auditor:  auditor,
		httpClient: &http.Client{
			Timeout: 30 * time.Minute, // Large models can take 10-20+ minutes to download
		},
	}
}

// DownloadModel downloads a model from Hugging Face
func (d *HFDownloader) DownloadModel(modelID string, destDir string) error {
	// Create destination directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Try common model file names and metadata files
	// Order matters: try weights first, then metadata
	commonFiles := []string{
		// Model weights (required)
		"model.safetensors",
		"pytorch_model.bin",
		"model.bin",
		"adapter_model.bin",
		"model.gguf",
		"model.pt",
		// Critical metadata files (required for loading)
		"config.json",
		"tokenizer.json",
		"tokenizer_config.json",
		// Optional but useful
		"generation_config.json",
		"special_tokens_map.json",
		"vocab.json",
		"merges.txt",
		"spiece.model",
		"added_tokens.json",
		"preprocessor_config.json",
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

		// Validate safetensors files after download
		if strings.HasSuffix(filename, ".safetensors") {
			if err := validateSafetensorsFile(destPath); err != nil {
				fmt.Printf("   ⚠ Downloaded file is corrupt: %v\n", err)
				fmt.Printf("   ⚠ File size: %d bytes\n", resp.ContentLength)
				fmt.Printf("   ⚠ Removing file and retrying download...\n")
				os.Remove(destPath)
				if d.auditor != nil {
					d.auditor.LogModelDownload(modelID, "failed", fmt.Sprintf("safetensors validation failed: %v", err))
				}
				continue
			}
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

	// Create progress bar
	bar := progressbar.NewOptions64(
		totalSize-startByte,
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(50),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionUseANSICodes(true),
	)

	// Copy with progress bar
	proxyReader := io.TeeReader(resp.Body, bar)
	written, err := io.Copy(file, proxyReader)
	bar.Close()

	if err != nil {
		return fmt.Errorf("download interrupted: %w", err)
	}

	// Verify complete
	finalSize := startByte + written
	if finalSize != totalSize {
		return fmt.Errorf("incomplete download: got %d bytes, expected %d bytes (from Content-Length header: %d, written: %d, startByte: %d)", finalSize, totalSize, totalSize, written, startByte)
	}

	// Get actual file size to double-check
	fileInfo, err := os.Stat(destPath)
	if err != nil {
		return fmt.Errorf("failed to verify downloaded file: %w", err)
	}

	if fileInfo.Size() != totalSize {
		return fmt.Errorf("file size mismatch: disk has %d bytes, expected %d bytes", fileInfo.Size(), totalSize)
	}

	fmt.Fprintf(os.Stderr, "\n") // newline after progress bar

	return nil
}

// validateSafetensorsFile checks if a safetensors file is valid by reading its header
func validateSafetensorsFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer file.Close()

	// Check file size
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("cannot stat file: %w", err)
	}
	fileSize := info.Size()
	if fileSize < 8 {
		return fmt.Errorf("file too small: %d bytes (minimum 8 bytes for header)", fileSize)
	}

	// safetensors format: 8-byte little-endian header size + JSON header + tensors
	headerSizeBytes := make([]byte, 8)
	n, err := file.Read(headerSizeBytes)
	if err != nil || n < 8 {
		return fmt.Errorf("cannot read header size: incomplete safetensors file")
	}

	headerSize := binary.LittleEndian.Uint64(headerSizeBytes)
	if headerSize == 0 || headerSize > 1024*1024*100 { // header should be < 100MB
		return fmt.Errorf("invalid safetensors header size: %d bytes", headerSize)
	}

	// Check if file has enough bytes for header
	minFileSize := int64(8) + int64(headerSize)
	if fileSize < minFileSize {
		return fmt.Errorf("incomplete file: expected at least %d bytes (8 + %d header), got %d", minFileSize, headerSize, fileSize)
	}

	// Try to read the header to verify it's readable
	headerBytes := make([]byte, headerSize)
	n, err = file.Read(headerBytes)
	if err != nil || int64(n) < int64(headerSize) {
		return fmt.Errorf("incomplete safetensors file: expected %d header bytes, got %d", headerSize, n)
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
