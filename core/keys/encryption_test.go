// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package keys

import (
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeMasterKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestEncryption_FieldsCiphertextOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	mk := makeMasterKey(t)

	ks, err := LoadKeyStore(path)
	if err != nil {
		t.Fatal(err)
	}
	ks.SetMasterKey(mk)

	if err := ks.AddUser(&UserProfile{
		ID:         "alice",
		Key:        "sk_alice",
		Department: "platform",
		Team:       "infra",
		Role:       "admin",
	}); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(path)
	body := string(raw)

	// On disk: plaintext team/department must NOT appear.
	for _, plain := range []string{"platform", "infra"} {
		if strings.Contains(body, `"`+plain+`"`) {
			t.Errorf("plaintext field %q leaked to disk: %s", plain, body)
		}
	}
	// Encrypted prefix must appear.
	if !strings.Contains(body, "$enc1$") {
		t.Errorf("expected $enc1$ prefix in on-disk JSON: %s", body)
	}

	// In-memory profile is still plaintext (so callers don't pay
	// decrypt-on-every-read).
	got, _ := ks.GetByID("alice")
	if got.Department != "platform" {
		t.Errorf("in-memory department should still be plaintext, got %q", got.Department)
	}
}

func TestEncryption_LoadAndDecrypt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	mk := makeMasterKey(t)

	// Round 1: write encrypted.
	ks1, _ := LoadKeyStore(path)
	ks1.SetMasterKey(mk)
	_ = ks1.AddUser(&UserProfile{
		ID: "alice", Key: "sk_alice",
		Department: "platform", Team: "infra", Role: "admin",
	})

	// Round 2: fresh load, verify ciphertext is in memory until decrypted.
	ks2, _ := LoadKeyStore(path)
	got, _ := ks2.GetByID("alice")
	if !strings.HasPrefix(got.Department, "$enc1$") {
		t.Errorf("freshly-loaded profile should have ciphertext department, got %q", got.Department)
	}

	// Apply master key and decrypt in place.
	ks2.SetMasterKey(mk)
	if err := ks2.DecryptProfilesInPlace(mk); err != nil {
		t.Fatal(err)
	}
	got, _ = ks2.GetByID("alice")
	if got.Department != "platform" {
		t.Errorf("decrypt-in-place: got %q, want platform", got.Department)
	}
}

func TestEncryption_WrongMasterKeyFailsDecrypt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	mkA := makeMasterKey(t)
	mkB := makeMasterKey(t)

	ks1, _ := LoadKeyStore(path)
	ks1.SetMasterKey(mkA)
	_ = ks1.AddUser(&UserProfile{ID: "alice", Key: "sk_alice", Department: "platform"})

	ks2, _ := LoadKeyStore(path)
	if err := ks2.DecryptProfilesInPlace(mkB); err == nil {
		t.Error("decrypt under wrong key should fail")
	}
}

func TestEncryption_MigrationFromPlaintext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")

	// Write a pre-Phase-C5 plaintext keys.json directly.
	plain := `{"users":{"alice":{"id":"alice","key":"sk_a","department":"platform","team":"infra","role":"admin","allowed_models":[],"rate_limit_per_min":100,"max_tokens_per_day":0,"max_tokens_per_month":0,"created_at":"2026-04-01T00:00:00Z","last_used_at":"2026-04-01T00:00:00Z"}}}`
	_ = os.WriteFile(path, []byte(plain), 0o600)

	mk := makeMasterKey(t)
	ks, _ := LoadKeyStore(path)
	ks.SetMasterKey(mk)

	if !strings.Contains(getFileContent(t, path), `"platform"`) {
		t.Fatal("test setup: expected plaintext on disk before migration")
	}

	plaintextCount, err := ks.MigrateEncryptFields()
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if plaintextCount != 1 {
		t.Errorf("expected 1 plaintext profile detected, got %d", plaintextCount)
	}

	// On-disk: ciphertext now.
	body := getFileContent(t, path)
	if strings.Contains(body, `"department":"platform"`) {
		t.Errorf("plaintext department still on disk after migrate: %s", body)
	}
	if !strings.Contains(body, "$enc1$") {
		t.Error("expected $enc1$ prefix after migrate")
	}
}

func TestEncryption_MigrationIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	mk := makeMasterKey(t)

	ks, _ := LoadKeyStore(path)
	ks.SetMasterKey(mk)
	_ = ks.AddUser(&UserProfile{ID: "alice", Key: "sk_a", Department: "platform"})

	// Re-running migrate finds nothing plaintext.
	plaintextCount, err := ks.MigrateEncryptFields()
	if err != nil {
		t.Fatal(err)
	}
	if plaintextCount != 0 {
		t.Errorf("expected 0 plaintext on repeat migrate, got %d", plaintextCount)
	}
}

func TestEncryption_HasEncryptedFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	mk := makeMasterKey(t)

	ks, _ := LoadKeyStore(path)
	if ks.HasEncryptedFields() {
		t.Error("empty store reports encrypted fields")
	}

	ks.SetMasterKey(mk)
	_ = ks.AddUser(&UserProfile{ID: "alice", Key: "sk_a", Department: "platform"})

	// In-memory profiles are plaintext; the on-disk JSON is encrypted.
	if ks.HasEncryptedFields() {
		t.Error("in-memory profiles should be plaintext after SetMasterKey + AddUser")
	}

	// Now re-load — fresh load surfaces ciphertext until DecryptProfilesInPlace.
	ks2, _ := LoadKeyStore(path)
	if !ks2.HasEncryptedFields() {
		t.Error("freshly-loaded store should report encrypted fields")
	}
}

// helper
func getFileContent(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// guard against accidental json import removal
var _ = json.Marshal
