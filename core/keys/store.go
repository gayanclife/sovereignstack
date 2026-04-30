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
	"sync"
	"time"
)

type UserProfile struct {
	ID                string    `json:"id"`
	Key               string    `json:"key"`
	Department        string    `json:"department"`
	Team              string    `json:"team"`
	Role              string    `json:"role"`
	AllowedModels     []string  `json:"allowed_models"`
	RateLimitPerMin   float64   `json:"rate_limit_per_min"`
	MaxTokensPerDay   int64     `json:"max_tokens_per_day"`
	MaxTokensPerMonth int64     `json:"max_tokens_per_month"`
	CreatedAt         time.Time `json:"created_at"`
	LastUsedAt        time.Time `json:"last_used_at"`
}

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
func (ks *KeyStore) AddUser(profile *UserProfile) error {
	if profile.ID == "" {
		return fmt.Errorf("user ID cannot be empty")
	}
	if profile.Key == "" {
		return fmt.Errorf("API key cannot be empty")
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

// GetByKey returns the user profile for an API key, or nil if not found.
func (ks *KeyStore) GetByKey(apiKey string) (*UserProfile, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	for _, user := range ks.Users {
		if user.Key == apiKey {
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

// saveLocked is called when mutex is already held.
func (ks *KeyStore) saveLocked() error {
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
