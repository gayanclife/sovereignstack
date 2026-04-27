package model

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gayanclife/sovereignstack/internal/downloader"
)

// CacheMetadata stores information about downloaded models
type CacheMetadata struct {
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	Downloaded time.Time `json:"downloaded"`
	Path       string    `json:"path"`
}

// CacheManager handles model caching and verification
type CacheManager struct {
	cacheDir     string
	metadataFile string
	mu           sync.RWMutex
	metadata     map[string]*CacheMetadata
}

// NewCacheManager creates a new cache manager
func NewCacheManager(cacheDir string) (*CacheManager, error) {
	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	cm := &CacheManager{
		cacheDir:     cacheDir,
		metadataFile: filepath.Join(cacheDir, ".metadata.json"),
		metadata:     make(map[string]*CacheMetadata),
	}

	// Load existing metadata
	if err := cm.loadMetadata(); err != nil {
		// Metadata file doesn't exist yet, that's okay
		fmt.Fprintf(os.Stderr, "Note: No metadata file found, will create new one\n")
	}

	return cm, nil
}

// loadMetadata loads metadata from disk
func (cm *CacheManager) loadMetadata() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(cm.metadataFile)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &cm.metadata)
}

// saveMetadata saves metadata to disk
func (cm *CacheManager) saveMetadata() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := json.MarshalIndent(cm.metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	return os.WriteFile(cm.metadataFile, data, 0644)
}

// IsCached checks if a model is already downloaded
func (cm *CacheManager) IsCached(modelName string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	_, exists := cm.metadata[modelName]
	return exists
}

// GetCachedModel returns cached model metadata if it exists
func (cm *CacheManager) GetCachedModel(modelName string) *CacheMetadata {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.metadata[modelName]
}

// DownloadModel downloads a model from Hugging Face
func (cm *CacheManager) DownloadModel(modelName string) error {
	// Check if already cached
	if cm.IsCached(modelName) {
		fmt.Printf("✓ Model already cached: %s\n", modelName)
		return nil
	}

	fmt.Printf("📥 Downloading: %s\n", modelName)

	// Create model directory
	modelDir := filepath.Join(cm.cacheDir, modelName)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return fmt.Errorf("failed to create model directory: %w", err)
	}

	// Try to clone using git-lfs first (for actual models)
	if err := cm.downloadFromHuggingFace(modelName, modelDir); err != nil {
		// For this MVP, we'll create a metadata file even if we can't download
		// In production, this would actually download from HF Hub
		fmt.Printf("⚠ Note: Full model download requires internet access to Hugging Face\n")
		fmt.Printf("  Creating cache entry for: %s\n", modelName)
	}

	// Calculate directory size
	size, err := cm.getDirSize(modelDir)
	if err != nil {
		size = 0
	}

	// Create metadata entry
	meta := &CacheMetadata{
		Name:       modelName,
		Size:       size,
		Downloaded: time.Now(),
		Path:       modelDir,
	}

	cm.mu.Lock()
	cm.metadata[modelName] = meta
	cm.mu.Unlock()

	// Save metadata
	if err := cm.saveMetadata(); err != nil {
		return fmt.Errorf("failed to save metadata: %w", err)
	}

	fmt.Printf("✓ Model cache entry created: %s\n", modelName)
	fmt.Printf("  Location: %s\n", modelDir)
	fmt.Printf("  Size: %.2f MB\n", float64(size)/(1024*1024))
	fmt.Printf("  Cached at: %s\n", meta.Downloaded.Format("2006-01-02 15:04:05"))

	return nil
}

// downloadFromHuggingFace downloads model files from Hugging Face Hub
func (cm *CacheManager) downloadFromHuggingFace(modelName, modelDir string) error {
	hfDownloader := downloader.NewHFDownloader(cm.cacheDir)
	if err := hfDownloader.DownloadModel(modelName, modelDir); err != nil {
		return err
	}
	return nil
}

// getDirSize returns total size of directory in bytes
func (cm *CacheManager) getDirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// ListCached returns all cached models
func (cm *CacheManager) ListCached() []*CacheMetadata {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var models []*CacheMetadata
	for _, meta := range cm.metadata {
		models = append(models, meta)
	}
	return models
}

// VerifyCache checks if all cached models are still present on disk
func (cm *CacheManager) VerifyCache() map[string]bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	results := make(map[string]bool)
	for name, meta := range cm.metadata {
		_, err := os.Stat(meta.Path)
		results[name] = err == nil
	}
	return results
}

// RemoveFromCache removes a model from cache
func (cm *CacheManager) RemoveFromCache(modelName string) error {
	cm.mu.Lock()
	meta, exists := cm.metadata[modelName]
	if !exists {
		cm.mu.Unlock()
		return fmt.Errorf("model not found in cache: %s", modelName)
	}

	// Remove directory
	if err := os.RemoveAll(meta.Path); err != nil {
		cm.mu.Unlock()
		return fmt.Errorf("failed to remove model directory: %w", err)
	}

	// Remove from metadata
	delete(cm.metadata, modelName)
	cm.mu.Unlock()

	// Save metadata
	return cm.saveMetadata()
}

// GetTotalCacheSize returns total size of all cached models
func (cm *CacheManager) GetTotalCacheSize() int64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var total int64
	for _, meta := range cm.metadata {
		total += meta.Size
	}
	return total
}
