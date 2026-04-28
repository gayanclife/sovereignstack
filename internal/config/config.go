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
package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

type Config struct {
	CacheDir string `json:"cache_dir"`
	LogDir   string `json:"log_dir"`
	HFToken  string `json:"hf_token"` // Encrypted
}

type Manager struct {
	configDir  string
	configFile string
	config     *Config
	encrypted  bool
}

// NewManager creates a new config manager
func NewManager() (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(home, ".sovereignstack")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	m := &Manager{
		configDir:  configDir,
		configFile: filepath.Join(configDir, "config.json"),
		config:     &Config{},
	}

	// Load existing config
	_ = m.load()

	// Set defaults if not set
	if m.config.CacheDir == "" {
		m.config.CacheDir = filepath.Join(configDir, "models")
	}
	if m.config.LogDir == "" {
		m.config.LogDir = m.getDefaultLogDir()
	}

	return m, nil
}

// getDefaultLogDir returns the appropriate log directory based on OS
func (m *Manager) getDefaultLogDir() string {
	switch runtime.GOOS {
	case "linux":
		// Try /var/log first on Linux
		if isWritable("/var/log") {
			return "/var/log/sovereignstack"
		}
	case "darwin":
		// macOS: ~/Library/Logs
		home, _ := os.UserHomeDir()
		logsDir := filepath.Join(home, "Library", "Logs")
		if isWritable(logsDir) {
			return filepath.Join(logsDir, "sovereignstack")
		}
	case "windows":
		// Windows: %APPDATA%/sovereignstack/logs
		if appData := os.Getenv("APPDATA"); appData != "" && isWritable(appData) {
			return filepath.Join(appData, "sovereignstack", "logs")
		}
	}

	// Fallback to config directory
	return filepath.Join(m.configDir, "logs")
}

// isWritable checks if a directory is writable
func isWritable(path string) bool {
	// Create test file
	testFile := filepath.Join(path, ".sovereignstack_write_test")
	err := os.WriteFile(testFile, []byte("test"), 0600)
	if err == nil {
		_ = os.Remove(testFile)
		return true
	}
	return false
}

// GetCacheDir returns the configured cache directory (with env var override)
func (m *Manager) GetCacheDir() string {
	if dir := os.Getenv("SOVEREIGNSTACK_CACHE_DIR"); dir != "" {
		return dir
	}
	return m.config.CacheDir
}

// GetLogDir returns the configured log directory (with env var override)
func (m *Manager) GetLogDir() string {
	if dir := os.Getenv("SOVEREIGNSTACK_LOG_DIR"); dir != "" {
		return dir
	}
	return m.config.LogDir
}

// GetHFToken returns the HF token (with env var override and decryption)
func (m *Manager) GetHFToken() string {
	if token := os.Getenv("HF_TOKEN"); token != "" {
		return token
	}
	if m.config.HFToken == "" {
		return ""
	}
	token, _ := decrypt(m.config.HFToken)
	return token
}

// SetCacheDir sets the cache directory
func (m *Manager) SetCacheDir(dir string) error {
	m.config.CacheDir = dir
	return m.save()
}

// SetLogDir sets the log directory
func (m *Manager) SetLogDir(dir string) error {
	m.config.LogDir = dir
	return m.save()
}

// SetHFToken sets the HF token (encrypted)
func (m *Manager) SetHFToken(token string) error {
	encrypted, err := encrypt(token)
	if err != nil {
		return fmt.Errorf("failed to encrypt token: %w", err)
	}
	m.config.HFToken = encrypted
	m.encrypted = true
	return m.save()
}

// load reads config from disk
func (m *Manager) load() error {
	data, err := os.ReadFile(m.configFile)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, m.config)
}

// save writes config to disk (0600 for security)
func (m *Manager) save() error {
	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	return os.WriteFile(m.configFile, data, 0600)
}

// GetConfigDir returns the config directory path
func (m *Manager) GetConfigDir() string {
	return m.configDir
}

// encrypt encrypts a string using AES-256-GCM with a derived key
func encrypt(plaintext string) (string, error) {
	key := deriveKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt decrypts a string using AES-256-GCM with a derived key
func decrypt(ciphertext string) (string, error) {
	key := deriveKey()
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext_bytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext_bytes, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// deriveKey derives a 256-bit key from the user's home directory path
// This ensures the same user can decrypt, but different users cannot
func deriveKey() []byte {
	home, _ := os.UserHomeDir()
	hash := sha256.Sum256([]byte(home))
	return hash[:]
}
