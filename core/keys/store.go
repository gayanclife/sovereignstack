// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package keys

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type UserProfile struct {
	ID string `json:"id"`

	// Key holds either an argon2id hash (production) or a plaintext API
	// key (legacy / pre-Phase-C migrations). The runtime code always
	// re-checks via IsHashedKey before using its contents. The CLI's
	// `sovstack keys add` writes hashes; `sovstack keys migrate-hash`
	// converts existing plaintext stores in place.
	Key string `json:"key"`

	// KeyIndex is an HMAC fingerprint of the plaintext key (8 bytes),
	// used by GetByKey to skip the expensive argon2 verification on
	// rows that obviously can't match. Empty for legacy profiles.
	KeyIndex string `json:"key_index,omitempty"`

	Department string `json:"department"`
	Team       string `json:"team"`

	// Role classifies the user. Recognised values:
	//   "user"    — interactive human (default)
	//   "admin"   — can perform management API mutations
	//   "viewer"  — read-only dashboard access (commercial layer enforces)
	//   "service" — machine-to-machine; subject to IPAllowlist if non-empty
	// Unknown values are treated like "user".
	Role string `json:"role"`

	// IPAllowlist, when non-empty AND Role=="service", restricts which
	// source IPs may authenticate as this user. Each entry may be a single
	// IP ("10.0.0.5") or a CIDR ("10.0.0.0/8"). An empty list means no
	// IP restriction. Phase F2.
	IPAllowlist []string `json:"ip_allowlist,omitempty"`

	AllowedModels     []string  `json:"allowed_models"`
	RateLimitPerMin   float64   `json:"rate_limit_per_min"`
	MaxTokensPerDay   int64     `json:"max_tokens_per_day"`
	MaxTokensPerMonth int64     `json:"max_tokens_per_month"`
	CreatedAt         time.Time `json:"created_at"`
	LastUsedAt        time.Time `json:"last_used_at"`
}

// Standard role names. Anything else is treated as "user".
const (
	RoleUser    = "user"
	RoleAdmin   = "admin"
	RoleViewer  = "viewer"
	RoleService = "service"
)

type keyStoreData struct {
	Users map[string]*UserProfile `json:"users"`
}

type KeyStore struct {
	Users map[string]*UserProfile
	path  string
	mu    sync.RWMutex
}

// LoadKeyStore loads the key store from disk. If file doesn't exist, returns empty store.
func LoadKeyStore(path string) (*KeyStore, error) {
	ks := &KeyStore{
		Users: make(map[string]*UserProfile),
		path:  path,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ks, nil
		}
		return nil, err
	}

	var store keyStoreData
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("invalid keys.json: %w", err)
	}

	ks.Users = store.Users
	return ks, nil
}

// Save writes the key store to disk.
func (ks *KeyStore) Save() error {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	data := keyStoreData{Users: ks.Users}
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal keys: %w", err)
	}

	if err := os.WriteFile(ks.path, bytes, 0600); err != nil {
		return fmt.Errorf("failed to write keys.json: %w", err)
	}
	return nil
}

// AddUser adds or updates a user profile. Updates LastUsedAt if already exists.
//
// As of Phase C: if profile.Key is plaintext (not in argon2id format), it is
// hashed in place before persisting. The hash and a fast-lookup index are
// stored; the plaintext is never written to disk. Callers that need to
// surface the plaintext to the operator (e.g. `keys add`) should keep their
// own copy before calling AddUser.
func (ks *KeyStore) AddUser(profile *UserProfile) error {
	if profile.ID == "" {
		return fmt.Errorf("user ID cannot be empty")
	}
	if profile.Key == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	// Hash plaintext keys at write time. Already-hashed values pass through.
	if !IsHashedKey(profile.Key) {
		// Compute the lookup index from the plaintext before we hash it.
		profile.KeyIndex = keyIndex(profile.Key)
		hashed, err := hashAPIKey(profile.Key)
		if err != nil {
			return fmt.Errorf("hash api key: %w", err)
		}
		profile.Key = hashed
	}

	ks.mu.Lock()
	defer ks.mu.Unlock()

	profile.LastUsedAt = time.Now()
	ks.Users[profile.ID] = profile
	return ks.saveLocked()
}

// RemoveUser removes a user by ID.
func (ks *KeyStore) RemoveUser(id string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if _, exists := ks.Users[id]; !exists {
		return fmt.Errorf("user %q not found", id)
	}

	delete(ks.Users, id)
	return ks.saveLocked()
}

// GetByKey returns the user profile matching apiKey, or nil if no match.
//
// Two paths:
//
//   - If profile.Key is hashed (argon2id), the candidate is verified
//     cryptographically. The KeyIndex fingerprint is used as a fast
//     filter so we only run argon2 verify on plausible matches.
//
//   - Legacy plaintext profiles (pre-Phase-C) are matched by direct
//     string equality. These should be migrated via
//     `sovstack keys migrate-hash` — a one-line warning is logged at
//     load time when any are detected.
func (ks *KeyStore) GetByKey(apiKey string) (*UserProfile, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	candidateIndex := keyIndex(apiKey)

	for _, user := range ks.Users {
		if !IsHashedKey(user.Key) {
			// Legacy: plaintext key in store.
			if user.Key == apiKey {
				return user, nil
			}
			continue
		}
		// Hashed: filter by fingerprint, then verify cryptographically.
		if user.KeyIndex != "" && user.KeyIndex != candidateIndex {
			continue
		}
		ok, err := verifyAPIKey(apiKey, user.Key)
		if err != nil {
			return nil, fmt.Errorf("verify hash for user %q: %w", user.ID, err)
		}
		if ok {
			return user, nil
		}
	}
	return nil, nil
}

// GetByID returns the user profile by ID, or nil if not found.
func (ks *KeyStore) GetByID(id string) (*UserProfile, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	if user, exists := ks.Users[id]; exists {
		return user, nil
	}
	return nil, nil
}

// ListUsers returns a list of all user profiles.
func (ks *KeyStore) ListUsers() []*UserProfile {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	users := make([]*UserProfile, 0, len(ks.Users))
	for _, user := range ks.Users {
		users = append(users, user)
	}
	return users
}

// UpdateLastUsed updates the LastUsedAt timestamp for a user.
func (ks *KeyStore) UpdateLastUsed(id string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	user, exists := ks.Users[id]
	if !exists {
		return fmt.Errorf("user %q not found", id)
	}

	user.LastUsedAt = time.Now()
	return nil
}

// GrantModelAccess adds a model to a user's allowed models.
func (ks *KeyStore) GrantModelAccess(id, model string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	user, exists := ks.Users[id]
	if !exists {
		return fmt.Errorf("user %q not found", id)
	}

	for _, m := range user.AllowedModels {
		if m == model || m == "*" {
			return nil
		}
	}

	user.AllowedModels = append(user.AllowedModels, model)
	return ks.saveLocked()
}

// RevokeModelAccess removes a model from a user's allowed models.
func (ks *KeyStore) RevokeModelAccess(id, model string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	user, exists := ks.Users[id]
	if !exists {
		return fmt.Errorf("user %q not found", id)
	}

	filtered := make([]string, 0, len(user.AllowedModels))
	for _, m := range user.AllowedModels {
		if m != model {
			filtered = append(filtered, m)
		}
	}
	user.AllowedModels = filtered
	return ks.saveLocked()
}

// SetQuota updates a user's token quotas.
func (ks *KeyStore) SetQuota(id string, dailyLimit, monthlyLimit int64) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	user, exists := ks.Users[id]
	if !exists {
		return fmt.Errorf("user %q not found", id)
	}

	user.MaxTokensPerDay = dailyLimit
	user.MaxTokensPerMonth = monthlyLimit
	return ks.saveLocked()
}

// MigrateHashes converts every plaintext-format Key in the store to an
// argon2id hash and saves the file. Idempotent: returns 0 if all keys are
// already hashed. Use after upgrading to Phase C.
func (ks *KeyStore) MigrateHashes() (int, error) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	migrated := 0
	for _, user := range ks.Users {
		if IsHashedKey(user.Key) {
			continue
		}
		user.KeyIndex = keyIndex(user.Key)
		hashed, err := hashAPIKey(user.Key)
		if err != nil {
			return migrated, fmt.Errorf("hash key for user %q: %w", user.ID, err)
		}
		user.Key = hashed
		migrated++
	}
	if migrated > 0 {
		if err := ks.saveLocked(); err != nil {
			return migrated, fmt.Errorf("save migrated store: %w", err)
		}
	}
	return migrated, nil
}

// HasPlaintextKeys returns true if any user profile still holds a plaintext
// API key. Callers (the gateway/management startup banner, the keys CLI)
// can use this to nudge operators toward MigrateHashes.
func (ks *KeyStore) HasPlaintextKeys() bool {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	for _, user := range ks.Users {
		if !IsHashedKey(user.Key) {
			return true
		}
	}
	return false
}

// CanAccess checks if a user can access a model.
func (ks *KeyStore) CanAccess(id, model string) bool {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	user, exists := ks.Users[id]
	if !exists {
		return false
	}

	for _, m := range user.AllowedModels {
		if m == "*" || m == model {
			return true
		}
	}
	return false
}

// saveLocked is called when the mutex is already held. It writes the keys
// file atomically (write-to-tmp + rename) and acquires an exclusive flock
// during the write so that a concurrent CLI invocation modifying the same
// file cannot interleave with an in-process management-service write.
//
// Failure modes:
//   - If marshalling fails, the on-disk file is unchanged.
//   - If the temp write fails, the on-disk file is unchanged; the temp file
//     is best-effort cleaned up.
//   - If the rename fails, the on-disk file is unchanged.
//   - A crash between rename and any later step is harmless: the on-disk
//     state always reflects either the previous keys.json or a complete
//     new keys.json, never a half-written one.
func (ks *KeyStore) saveLocked() error {
	data := keyStoreData{Users: ks.Users}
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal keys: %w", err)
	}

	// Acquire cross-process lock on a sibling .lock file. Other writers
	// (the CLI) acquire the same lock; readers don't lock (the on-disk
	// file is always consistent thanks to the rename-based write).
	lockPath := ks.path + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("open lock file: %w", err)
	}
	defer lockFile.Close()
	if err := flockExclusive(lockFile); err != nil {
		return fmt.Errorf("acquire write lock: %w", err)
	}
	defer flockUnlock(lockFile)

	// Write to a temp file in the same directory (so rename is atomic
	// across the same filesystem) then atomically replace the target.
	tmp, err := os.CreateTemp(filepath.Dir(ks.path), filepath.Base(ks.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(bytes); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil { // fsync before rename, durability gate
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("fsync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, ks.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
