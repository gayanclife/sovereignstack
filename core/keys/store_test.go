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
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadKeyStore_NewFile(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "keys.json")

	ks, err := LoadKeyStore(tmpFile)
	if err != nil {
		t.Fatalf("LoadKeyStore failed: %v", err)
	}

	if ks == nil {
		t.Fatal("KeyStore is nil")
	}

	if len(ks.Users) != 0 {
		t.Errorf("Expected empty Users map, got %d users", len(ks.Users))
	}
}

func TestAddUser_Basic(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "keys.json")
	ks, _ := LoadKeyStore(tmpFile)

	profile := &UserProfile{
		ID:              "alice",
		Key:             "sk_alice_123",
		Department:      "research",
		Team:            "nlp",
		Role:            "analyst",
		RateLimitPerMin: 100,
	}

	err := ks.AddUser(profile)
	if err != nil {
		t.Fatalf("AddUser failed: %v", err)
	}

	if len(ks.Users) != 1 {
		t.Errorf("Expected 1 user, got %d", len(ks.Users))
	}

	if ks.Users["alice"].ID != "alice" {
		t.Errorf("User ID not stored correctly")
	}

	if _, err := os.Stat(tmpFile); err != nil {
		t.Fatalf("keys.json file not created: %v", err)
	}
}

func TestAddUser_EmptyID(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "keys.json")
	ks, _ := LoadKeyStore(tmpFile)

	profile := &UserProfile{
		ID:  "",
		Key: "sk_test",
	}

	err := ks.AddUser(profile)
	if err == nil {
		t.Fatal("Expected error for empty ID, got nil")
	}
}

func TestGetByKey(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "keys.json")
	ks, _ := LoadKeyStore(tmpFile)

	profile := &UserProfile{
		ID:  "alice",
		Key: "sk_alice_123",
	}
	ks.AddUser(profile)

	user, _ := ks.GetByKey("sk_alice_123")
	if user == nil {
		t.Fatal("GetByKey returned nil for existing key")
	}

	if user.ID != "alice" {
		t.Errorf("Got wrong user: %s", user.ID)
	}

	user, _ = ks.GetByKey("sk_nonexistent")
	if user != nil {
		t.Fatal("GetByKey should return nil for non-existent key")
	}
}

func TestGetByID(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "keys.json")
	ks, _ := LoadKeyStore(tmpFile)

	profile := &UserProfile{
		ID:  "bob",
		Key: "sk_bob_456",
	}
	ks.AddUser(profile)

	user, _ := ks.GetByID("bob")
	if user == nil {
		t.Fatal("GetByID returned nil for existing user")
	}

	// Phase C: stored key is now a hash. Verify by attempting an auth.
	if !IsHashedKey(user.Key) {
		t.Errorf("expected stored key to be hashed, got plaintext: %s", user.Key)
	}
	authedUser, _ := ks.GetByKey("sk_bob_456")
	if authedUser == nil || authedUser.ID != "bob" {
		t.Errorf("GetByKey with original plaintext failed; got %v", authedUser)
	}

	user, _ = ks.GetByID("nonexistent")
	if user != nil {
		t.Fatal("GetByID should return nil for non-existent user")
	}
}

func TestRemoveUser(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "keys.json")
	ks, _ := LoadKeyStore(tmpFile)

	profile := &UserProfile{ID: "charlie", Key: "sk_charlie"}
	ks.AddUser(profile)

	if len(ks.Users) != 1 {
		t.Fatalf("Expected 1 user before removal, got %d", len(ks.Users))
	}

	err := ks.RemoveUser("charlie")
	if err != nil {
		t.Fatalf("RemoveUser failed: %v", err)
	}

	if len(ks.Users) != 0 {
		t.Errorf("Expected 0 users after removal, got %d", len(ks.Users))
	}

	err = ks.RemoveUser("nonexistent")
	if err == nil {
		t.Fatal("RemoveUser should fail for non-existent user")
	}
}

func TestListUsers(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "keys.json")
	ks, _ := LoadKeyStore(tmpFile)

	users := ks.ListUsers()
	if len(users) != 0 {
		t.Errorf("Expected empty list, got %d users", len(users))
	}

	for i := 1; i <= 3; i++ {
		p := &UserProfile{ID: string(rune(96 + i)), Key: "sk_" + string(rune(96+i))}
		ks.AddUser(p)
	}

	users = ks.ListUsers()
	if len(users) != 3 {
		t.Errorf("Expected 3 users, got %d", len(users))
	}
}

func TestPersistence(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "keys.json")

	ks1, _ := LoadKeyStore(tmpFile)
	profile := &UserProfile{
		ID:         "diana",
		Key:        "sk_diana_789",
		Department: "ops",
	}
	ks1.AddUser(profile)

	ks2, err := LoadKeyStore(tmpFile)
	if err != nil {
		t.Fatalf("Failed to reload KeyStore: %v", err)
	}

	user, _ := ks2.GetByID("diana")
	if user == nil {
		t.Fatal("User not persisted to disk")
	}

	// Phase C: stored key is hashed; verify auth still works after reload.
	if !IsHashedKey(user.Key) {
		t.Errorf("expected key hashed after reload, got plaintext: %s", user.Key)
	}
	authedUser, _ := ks2.GetByKey("sk_diana_789")
	if authedUser == nil || authedUser.ID != "diana" {
		t.Errorf("GetByKey with original plaintext failed after reload; got %v", authedUser)
	}

	if user.Department != "ops" {
		t.Errorf("User department not persisted correctly: %s", user.Department)
	}
}

func TestUpdateLastUsed(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "keys.json")
	ks, _ := LoadKeyStore(tmpFile)

	profile := &UserProfile{ID: "eve", Key: "sk_eve"}
	ks.AddUser(profile)

	user, _ := ks.GetByID("eve")
	oldTime := user.LastUsedAt

	time.Sleep(100 * time.Millisecond)

	err := ks.UpdateLastUsed("eve")
	if err != nil {
		t.Fatalf("UpdateLastUsed failed: %v", err)
	}

	user, _ = ks.GetByID("eve")
	if user.LastUsedAt.Before(oldTime) || user.LastUsedAt.Equal(oldTime) {
		t.Errorf("LastUsedAt not updated")
	}
}

func TestConcurrentAddGet(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "keys.json")
	ks, _ := LoadKeyStore(tmpFile)

	done := make(chan bool)

	for i := 1; i <= 10; i++ {
		go func(id int) {
			p := &UserProfile{
				ID:  string(rune(96 + id)),
				Key: "sk_" + string(rune(96+id)),
			}
			ks.AddUser(p)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if len(ks.Users) != 10 {
		t.Errorf("Expected 10 users after concurrent adds, got %d", len(ks.Users))
	}
}

func TestGrantModelAccess(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "keys.json")
	ks, _ := LoadKeyStore(tmpFile)

	profile := &UserProfile{
		ID:            "frank",
		Key:           "sk_frank",
		AllowedModels: []string{},
	}
	ks.AddUser(profile)

	err := ks.GrantModelAccess("frank", "mistral-7b")
	if err != nil {
		t.Fatalf("GrantModelAccess failed: %v", err)
	}

	user, _ := ks.GetByID("frank")
	if len(user.AllowedModels) != 1 || user.AllowedModels[0] != "mistral-7b" {
		t.Errorf("Model not granted correctly: %v", user.AllowedModels)
	}

	err = ks.GrantModelAccess("frank", "mistral-7b")
	if err != nil {
		t.Fatalf("GrantModelAccess duplicate should not fail: %v", err)
	}

	user, _ = ks.GetByID("frank")
	if len(user.AllowedModels) != 1 {
		t.Errorf("Duplicate grant should not add model again: %d", len(user.AllowedModels))
	}
}

func TestRevokeModelAccess(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "keys.json")
	ks, _ := LoadKeyStore(tmpFile)

	profile := &UserProfile{
		ID:            "grace",
		Key:           "sk_grace",
		AllowedModels: []string{"llama-3", "mistral-7b"},
	}
	ks.AddUser(profile)

	err := ks.RevokeModelAccess("grace", "mistral-7b")
	if err != nil {
		t.Fatalf("RevokeModelAccess failed: %v", err)
	}

	user, _ := ks.GetByID("grace")
	if len(user.AllowedModels) != 1 || user.AllowedModels[0] != "llama-3" {
		t.Errorf("Model not revoked correctly: %v", user.AllowedModels)
	}
}

func TestSetQuota(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "keys.json")
	ks, _ := LoadKeyStore(tmpFile)

	profile := &UserProfile{
		ID:  "hank",
		Key: "sk_hank",
	}
	ks.AddUser(profile)

	err := ks.SetQuota("hank", 500000, 10000000)
	if err != nil {
		t.Fatalf("SetQuota failed: %v", err)
	}

	user, _ := ks.GetByID("hank")
	if user.MaxTokensPerDay != 500000 || user.MaxTokensPerMonth != 10000000 {
		t.Errorf("Quota not set correctly: day=%d, month=%d", user.MaxTokensPerDay, user.MaxTokensPerMonth)
	}
}

func TestCanAccess(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "keys.json")
	ks, _ := LoadKeyStore(tmpFile)

	profile := &UserProfile{
		ID:            "iris",
		Key:           "sk_iris",
		AllowedModels: []string{"mistral-7b"},
	}
	ks.AddUser(profile)

	if !ks.CanAccess("iris", "mistral-7b") {
		t.Fatal("CanAccess should return true for allowed model")
	}

	if ks.CanAccess("iris", "llama-3") {
		t.Fatal("CanAccess should return false for non-allowed model")
	}

	// Test wildcard access
	profile2 := &UserProfile{
		ID:            "jack",
		Key:           "sk_jack",
		AllowedModels: []string{"*"},
	}
	ks.AddUser(profile2)

	if !ks.CanAccess("jack", "mistral-7b") || !ks.CanAccess("jack", "llama-3") || !ks.CanAccess("jack", "any-model") {
		t.Fatal("CanAccess should return true for all models with wildcard")
	}

	if ks.CanAccess("nonexistent", "mistral-7b") {
		t.Fatal("CanAccess should return false for non-existent user")
	}
}
