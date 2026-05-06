// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package keys

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestHashAPIKey_Format(t *testing.T) {
	hash, err := hashAPIKey("sk_test_xyz")
	if err != nil {
		t.Fatalf("hashAPIKey: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("hash does not start with $argon2id$: %s", hash)
	}
	if !IsHashedKey(hash) {
		t.Error("IsHashedKey returned false for valid hash")
	}
	if IsHashedKey("sk_plaintext") {
		t.Error("IsHashedKey returned true for plaintext")
	}
}

func TestHashAPIKey_DifferentHashesForSameInput(t *testing.T) {
	a, _ := hashAPIKey("sk_same")
	b, _ := hashAPIKey("sk_same")
	if a == b {
		t.Error("two hashes of the same input should differ (different salts)")
	}
}

func TestVerifyAPIKey_RoundTrip(t *testing.T) {
	hash, err := hashAPIKey("sk_correct")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := verifyAPIKey("sk_correct", hash)
	if err != nil {
		t.Fatalf("verify correct: %v", err)
	}
	if !ok {
		t.Error("verify should return true for correct plaintext")
	}

	ok, err = verifyAPIKey("sk_wrong", hash)
	if err != nil {
		t.Fatalf("verify wrong: %v", err)
	}
	if ok {
		t.Error("verify should return false for wrong plaintext")
	}
}

func TestVerifyAPIKey_RejectsNonArgonHash(t *testing.T) {
	_, err := verifyAPIKey("sk_anything", "not-a-hash")
	if err == nil {
		t.Error("expected error for non-argon2 hash format")
	}
}

func TestKeyIndex_Stable(t *testing.T) {
	a := keyIndex("sk_alice")
	b := keyIndex("sk_alice")
	if a != b {
		t.Errorf("keyIndex must be stable for same input: %s vs %s", a, b)
	}
}

func TestKeyIndex_DifferentForDifferentKeys(t *testing.T) {
	a := keyIndex("sk_alice")
	b := keyIndex("sk_bob")
	if a == b {
		t.Errorf("keyIndex collision: %s", a)
	}
}

// End-to-end: AddUser writes a hash; GetByKey with original plaintext finds
// the user; GetByKey with the hash itself does NOT.
func TestKeyStore_HashingEndToEnd(t *testing.T) {
	dir := t.TempDir()
	ks, err := LoadKeyStore(filepath.Join(dir, "keys.json"))
	if err != nil {
		t.Fatal(err)
	}

	if err := ks.AddUser(&UserProfile{ID: "alice", Key: "sk_alice_plaintext"}); err != nil {
		t.Fatal(err)
	}

	// On disk: hash, not plaintext.
	stored := ks.Users["alice"].Key
	if !IsHashedKey(stored) {
		t.Errorf("expected hash, got: %s", stored)
	}
	if stored == "sk_alice_plaintext" {
		t.Error("plaintext leaked through to in-memory profile")
	}

	// Auth with plaintext succeeds.
	got, err := ks.GetByKey("sk_alice_plaintext")
	if err != nil {
		t.Fatalf("GetByKey: %v", err)
	}
	if got == nil || got.ID != "alice" {
		t.Errorf("expected alice, got %v", got)
	}

	// Auth with the hash itself does not (not a privilege escalation path).
	got, _ = ks.GetByKey(stored)
	if got != nil {
		t.Error("authenticating with hash-as-key should not succeed")
	}

	// Auth with a wrong key returns nil.
	got, _ = ks.GetByKey("sk_wrong")
	if got != nil {
		t.Errorf("expected nil for wrong key, got %v", got)
	}
}

// MigrateHashes converts plaintext keys in an existing store to hashes.
func TestKeyStore_MigrateHashes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.json")

	// Bypass AddUser (which would auto-hash) and write a plaintext store
	// directly to simulate a pre-Phase-C file on disk.
	ks, _ := LoadKeyStore(path)
	ks.Users["legacy"] = &UserProfile{
		ID:  "legacy",
		Key: "sk_plaintext_legacy_123",
	}
	if err := ks.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Re-load and migrate.
	ks2, _ := LoadKeyStore(path)
	migrated, err := ks2.MigrateHashes()
	if err != nil {
		t.Fatalf("MigrateHashes: %v", err)
	}
	if migrated != 1 {
		t.Errorf("expected 1 migrated, got %d", migrated)
	}

	// On-disk key is now hashed; lookup with original plaintext still works.
	if !IsHashedKey(ks2.Users["legacy"].Key) {
		t.Error("post-migration key not hashed")
	}
	user, _ := ks2.GetByKey("sk_plaintext_legacy_123")
	if user == nil {
		t.Error("plaintext-based lookup failed after migration")
	}

	// Re-running migrate is idempotent: no further changes.
	again, _ := ks2.MigrateHashes()
	if again != 0 {
		t.Errorf("re-running migrate should be no-op, got %d", again)
	}
}
