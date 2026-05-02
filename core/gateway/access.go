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

package gateway

import (
	"github.com/gayanclife/sovereignstack/core/keys"
)

// AccessController determines if a user can access a specific model.
type AccessController interface {
	CanAccess(userID, modelName string) bool
}

// KeyStoreAccessController implements AccessController using a KeyStore.
type KeyStoreAccessController struct {
	store *keys.KeyStore
}

// NewKeyStoreAccessController creates a new access controller backed by a KeyStore.
func NewKeyStoreAccessController(store *keys.KeyStore) *KeyStoreAccessController {
	return &KeyStoreAccessController{store: store}
}

// CanAccess checks if a user has access to a model.
// Returns true if:
// - User's allowed_models contains the model name, OR
// - User's allowed_models contains "*" (wildcard = all models)
// Returns false if user not found or model not in their list.
func (ac *KeyStoreAccessController) CanAccess(userID, modelName string) bool {
	profile, err := ac.store.GetByID(userID)
	if err != nil || profile == nil {
		return false
	}

	// Check if user has wildcard access
	for _, model := range profile.AllowedModels {
		if model == "*" {
			return true
		}
		if model == modelName {
			return true
		}
	}

	return false
}

// IsSourceIPAllowed returns false only when the user is a service account
// with a non-empty IPAllowlist that excludes the given source. Phase F2.
//
// Returns true (allowed) for users we can't look up — auth has already
// gated this call, and silently denying based on lookup failure would be
// surprising. The keystore-not-found case can't happen here in practice
// because the same user just authenticated.
func (ac *KeyStoreAccessController) IsSourceIPAllowed(userID, sourceIP string) bool {
	profile, err := ac.store.GetByID(userID)
	if err != nil || profile == nil {
		return true
	}
	return profile.IsIPAllowed(sourceIP)
}

// DenyAllAccessController denies all access (useful for testing).
type DenyAllAccessController struct{}

func (ac *DenyAllAccessController) CanAccess(userID, modelName string) bool {
	return false
}

// AllowAllAccessController allows all access (default when no controller set).
type AllowAllAccessController struct{}

func (ac *AllowAllAccessController) CanAccess(userID, modelName string) bool {
	return true
}
