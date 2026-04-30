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
	"path/filepath"
	"testing"
	"time"

	"github.com/gayanclife/sovereignstack/core/keys"
)

func TestKeyStoreAccessController_CanAccess_Allowed(t *testing.T) {
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}

	profile := &keys.UserProfile{
		ID:            "alice",
		Key:           "sk_alice",
		AllowedModels: []string{"mistral-7b", "llama-3-8b"},
		CreatedAt:     time.Now(),
	}
	ks.Users["alice"] = profile

	ac := NewKeyStoreAccessController(ks)

	if !ac.CanAccess("alice", "mistral-7b") {
		t.Error("alice should have access to mistral-7b")
	}

	if !ac.CanAccess("alice", "llama-3-8b") {
		t.Error("alice should have access to llama-3-8b")
	}
}

func TestKeyStoreAccessController_CanAccess_Denied(t *testing.T) {
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}

	profile := &keys.UserProfile{
		ID:            "alice",
		Key:           "sk_alice",
		AllowedModels: []string{"mistral-7b"},
		CreatedAt:     time.Now(),
	}
	ks.Users["alice"] = profile

	ac := NewKeyStoreAccessController(ks)

	if ac.CanAccess("alice", "proprietary-model") {
		t.Error("alice should NOT have access to proprietary-model")
	}
}

func TestKeyStoreAccessController_Wildcard(t *testing.T) {
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}

	profile := &keys.UserProfile{
		ID:            "admin",
		Key:           "sk_admin",
		AllowedModels: []string{"*"},
		CreatedAt:     time.Now(),
	}
	ks.Users["admin"] = profile

	ac := NewKeyStoreAccessController(ks)

	if !ac.CanAccess("admin", "mistral-7b") {
		t.Error("admin should have access to all models (wildcard)")
	}

	if !ac.CanAccess("admin", "proprietary-model") {
		t.Error("admin should have access to all models (wildcard)")
	}

	if !ac.CanAccess("admin", "unknown-model") {
		t.Error("admin should have access to all models (wildcard)")
	}
}

func TestKeyStoreAccessController_NotFound(t *testing.T) {
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}

	ac := NewKeyStoreAccessController(ks)

	if ac.CanAccess("nonexistent", "mistral-7b") {
		t.Error("nonexistent user should NOT have access")
	}
}

func TestKeyStoreAccessController_Empty(t *testing.T) {
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}

	profile := &keys.UserProfile{
		ID:            "bob",
		Key:           "sk_bob",
		AllowedModels: []string{},
		CreatedAt:     time.Now(),
	}
	ks.Users["bob"] = profile

	ac := NewKeyStoreAccessController(ks)

	if ac.CanAccess("bob", "mistral-7b") {
		t.Error("bob with empty allowed_models should NOT have access")
	}
}

func TestDenyAllAccessController(t *testing.T) {
	ac := &DenyAllAccessController{}

	if ac.CanAccess("anyone", "any-model") {
		t.Error("DenyAllAccessController should always deny")
	}
}

func TestAllowAllAccessController(t *testing.T) {
	ac := &AllowAllAccessController{}

	if !ac.CanAccess("anyone", "any-model") {
		t.Error("AllowAllAccessController should always allow")
	}
}

func TestAccessControlIntegration(t *testing.T) {
	// Create a temporary KeyStore
	tmpFile := filepath.Join(t.TempDir(), "keys.json")
	ks, _ := keys.LoadKeyStore(tmpFile)

	// Add users
	alice := &keys.UserProfile{
		ID:            "alice",
		Key:           "sk_alice",
		AllowedModels: []string{"mistral-7b"},
		CreatedAt:     time.Now(),
	}
	bob := &keys.UserProfile{
		ID:            "bob",
		Key:           "sk_bob",
		AllowedModels: []string{"*"},
		CreatedAt:     time.Now(),
	}
	charlie := &keys.UserProfile{
		ID:            "charlie",
		Key:           "sk_charlie",
		AllowedModels: []string{},
		CreatedAt:     time.Now(),
	}

	ks.AddUser(alice)
	ks.AddUser(bob)
	ks.AddUser(charlie)

	// Create access controller
	ac := NewKeyStoreAccessController(ks)

	// Test cases
	tests := []struct {
		userID    string
		model     string
		allowed   bool
		name      string
	}{
		{"alice", "mistral-7b", true, "alice allowed to mistral-7b"},
		{"alice", "llama-3-8b", false, "alice denied llama-3-8b"},
		{"bob", "mistral-7b", true, "bob wildcard allows mistral-7b"},
		{"bob", "anything", true, "bob wildcard allows anything"},
		{"charlie", "mistral-7b", false, "charlie empty list denies all"},
		{"unknown", "mistral-7b", false, "unknown user denied"},
	}

	for _, tc := range tests {
		result := ac.CanAccess(tc.userID, tc.model)
		if result != tc.allowed {
			t.Errorf("%s: expected %v, got %v", tc.name, tc.allowed, result)
		}
	}
}
