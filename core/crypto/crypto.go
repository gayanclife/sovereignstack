// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package crypto provides AES-256-GCM encryption helpers for sensitive
// fields stored in the keys file (department, team, role) — Phase C5.
//
// Threat model:
//
//   - We protect against on-disk leaks: a stolen keys.json without the
//     master key reveals nothing about which user works in which team.
//   - We do NOT protect against a process-memory attacker — once the
//     keystore is loaded, plaintext fields live in RAM.
//   - The master key itself is the keystone: lose it and existing data
//     is unrecoverable; leak it and on-disk encryption no longer helps.
//
// Storage format (base64 + version prefix):
//
//   - Plaintext-mode field: stored as-is (back-compat with pre-C5 files).
//   - Encrypted-mode field: "$enc1$<base64(nonce || ciphertext)>"
//
// Decrypt() looks for the prefix and falls back to "field is plaintext"
// when absent, so a rolling migration is possible.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// encPrefix marks a field as encrypted. Versioned so we can rotate
	// algorithms in future without breaking parse.
	encPrefix    = "$enc1$"
	masterKeyLen = 32 // AES-256
	nonceSize    = 12 // GCM standard
)

// LoadOrCreateMasterKey reads the master key from path, generating a fresh
// 32-byte random key (and writing it with mode 0o600) if the file does
// not exist. Used by the policy service at startup.
//
// path may begin with "~/" — expanded via os.UserHomeDir.
func LoadOrCreateMasterKey(path string) ([]byte, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("expand ~/: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}

	data, err := os.ReadFile(path)
	if err == nil {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
		if err != nil {
			return nil, fmt.Errorf("master key %q is not valid base64: %w", path, err)
		}
		if len(decoded) != masterKeyLen {
			return nil, fmt.Errorf("master key %q wrong length: got %d, want %d", path, len(decoded), masterKeyLen)
		}
		return decoded, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read master key %q: %w", path, err)
	}

	// Generate a new master key.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create master-key dir: %w", err)
	}
	key := make([]byte, masterKeyLen)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate master key: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(key)
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("write master key %q: %w", path, err)
	}
	return key, nil
}

// Encrypt seals plaintext with AES-256-GCM under masterKey and returns
// the storage-format string ("$enc1$<base64>"). An empty input returns
// an empty output (no point encrypting nothing).
func Encrypt(masterKey []byte, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if len(masterKey) != masterKeyLen {
		return "", fmt.Errorf("master key wrong length: got %d, want %d", len(masterKey), masterKeyLen)
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	out := append(nonce, ct...)
	return encPrefix + base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt opens a storage-format string. If the input lacks the encryption
// prefix, it's returned as-is — this is what allows rolling migration
// from a plaintext keys.json without breaking unconverted profiles.
func Decrypt(masterKey []byte, stored string) (string, error) {
	if !strings.HasPrefix(stored, encPrefix) {
		return stored, nil
	}
	if len(masterKey) != masterKeyLen {
		return "", fmt.Errorf("master key wrong length: got %d, want %d", len(masterKey), masterKeyLen)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, encPrefix))
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	if len(raw) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := raw[:nonceSize], raw[nonceSize:]

	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(pt), nil
}

// IsEncrypted reports whether stored carries the encryption prefix.
func IsEncrypted(stored string) bool {
	return strings.HasPrefix(stored, encPrefix)
}
