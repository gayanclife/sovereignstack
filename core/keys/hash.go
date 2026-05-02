// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package keys

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2 parameters tuned for ~50ms/auth on a 2024-class server CPU.
// These are conservative defaults; deployments with stricter latency
// budgets can tune them down. Don't go lower than the OWASP minimum
// (memory >= 19 MiB, time >= 2).
const (
	argonTime    = uint32(2)        // iterations
	argonMemory  = uint32(64 * 1024) // KiB → 64 MiB
	argonThreads = uint8(2)
	argonKeyLen  = uint32(32)
	saltLen      = 16
)

// hashAPIKey returns a self-describing hash string of the form
//
//	$argon2id$v=19$m=65536,t=2,p=2$<salt-b64>$<hash-b64>
//
// suitable for storing in keys.json. The string contains everything
// needed to re-verify a candidate without external metadata.
func hashAPIKey(plaintext string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("salt: %w", err)
	}
	hash := argon2.IDKey([]byte(plaintext), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	encSalt := base64.RawStdEncoding.EncodeToString(salt)
	encHash := base64.RawStdEncoding.EncodeToString(hash)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads, encSalt, encHash), nil
}

// verifyAPIKey returns true iff plaintext, hashed under the parameters
// encoded in stored, equals the stored hash. Constant-time compare.
func verifyAPIKey(plaintext, stored string) (bool, error) {
	parts := strings.Split(stored, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("not an argon2id hash")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, fmt.Errorf("unsupported argon2 version")
	}
	var memory uint32
	var time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, fmt.Errorf("bad argon2 params: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("bad salt: %w", err)
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("bad hash: %w", err)
	}

	got := argon2.IDKey([]byte(plaintext), salt, time, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// IsHashedKey returns true if s is in the argon2id hash format. Used by
// the loader to detect plaintext-format keys.json files that need migration.
func IsHashedKey(s string) bool {
	return strings.HasPrefix(s, "$argon2id$")
}

// keyIndex computes a short HMAC fingerprint of the plaintext API key.
// The KeyStore stores it alongside each user so GetByKey can fast-path
// lookups: hash the candidate's first 16 chars with HMAC-SHA256 under a
// stable server secret, then iterate only profiles whose Index matches.
//
// The server secret is a fixed string here because the lookup index is
// not a security boundary — a leaked index reveals "this is the prefix
// that exists" but not the underlying key. Argon2 is the actual
// authentication primitive.
func keyIndex(plaintext string) string {
	const indexSecret = "sovstack-keystore-lookup-index-v1"
	mac := hmac.New(sha256.New, []byte(indexSecret))
	mac.Write([]byte(plaintext))
	return base64.RawStdEncoding.EncodeToString(mac.Sum(nil)[:8])
}
