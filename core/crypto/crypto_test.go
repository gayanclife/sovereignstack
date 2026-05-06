// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package crypto

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, masterKeyLen)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	key := mustKey(t)
	for _, in := range []string{"alice", "platform-eng", "VIP user — 🦀 unicode"} {
		ct, err := Encrypt(key, in)
		if err != nil {
			t.Fatalf("encrypt %q: %v", in, err)
		}
		if !IsEncrypted(ct) {
			t.Errorf("encrypted output should carry prefix: %s", ct)
		}
		pt, err := Decrypt(key, ct)
		if err != nil {
			t.Fatalf("decrypt %q: %v", in, err)
		}
		if pt != in {
			t.Errorf("round-trip: got %q, want %q", pt, in)
		}
	}
}

func TestEncrypt_EmptyInputEmptyOutput(t *testing.T) {
	ct, err := Encrypt(mustKey(t), "")
	if err != nil {
		t.Fatal(err)
	}
	if ct != "" {
		t.Errorf("empty input should yield empty output, got %q", ct)
	}
}

func TestEncrypt_NondeterministicCiphertext(t *testing.T) {
	key := mustKey(t)
	a, _ := Encrypt(key, "alice")
	b, _ := Encrypt(key, "alice")
	if a == b {
		t.Error("two encryptions of the same input should differ (different nonces)")
	}
}

func TestDecrypt_PlaintextPassthrough(t *testing.T) {
	// Pre-Phase-C5 fields stored as plain strings — Decrypt should return as-is.
	got, err := Decrypt(mustKey(t), "platform-eng")
	if err != nil {
		t.Fatal(err)
	}
	if got != "platform-eng" {
		t.Errorf("plaintext passthrough: got %q", got)
	}
}

func TestDecrypt_TamperedCiphertextFails(t *testing.T) {
	key := mustKey(t)
	ct, _ := Encrypt(key, "alice")
	// Flip a byte in the base64 payload (after the prefix).
	tampered := ct[:len(ct)-3] + "AAA"
	if _, err := Decrypt(key, tampered); err == nil {
		t.Error("tampered ciphertext should fail GCM verification")
	}
}

func TestDecrypt_WrongKeyFails(t *testing.T) {
	a, b := mustKey(t), mustKey(t)
	ct, _ := Encrypt(a, "alice")
	if _, err := Decrypt(b, ct); err == nil {
		t.Error("ciphertext under key A should not decrypt under key B")
	}
}

func TestEncrypt_RejectsWrongLengthKey(t *testing.T) {
	if _, err := Encrypt(make([]byte, 16), "x"); err == nil {
		t.Error("expected error for short master key")
	}
}

func TestLoadOrCreateMasterKey_GeneratesOnFirstCall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "master.key")

	k1, err := LoadOrCreateMasterKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(k1) != masterKeyLen {
		t.Errorf("key length: got %d, want %d", len(k1), masterKeyLen)
	}

	// File exists with mode 0o600.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("master-key file mode: got %#o, want 0600", info.Mode().Perm())
	}

	// Second call returns the same key.
	k2, err := LoadOrCreateMasterKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !equalBytes(k1, k2) {
		t.Error("LoadOrCreate should be idempotent on subsequent calls")
	}
}

func TestLoadOrCreateMasterKey_RejectsCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "master.key")
	_ = os.WriteFile(path, []byte("not-base64-!!!"), 0o600)
	if _, err := LoadOrCreateMasterKey(path); err == nil {
		t.Error("corrupt master-key file should error")
	}
}

func TestLoadOrCreateMasterKey_RejectsWrongLengthFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "master.key")
	// Valid base64 but only 8 bytes once decoded.
	_ = os.WriteFile(path, []byte("AAAAAAAAAAA="), 0o600)
	if _, err := LoadOrCreateMasterKey(path); err == nil {
		t.Error("wrong-length master-key file should error")
	}
}

func TestEncrypt_PrefixIsRecognisable(t *testing.T) {
	ct, _ := Encrypt(mustKey(t), "alice")
	if !strings.HasPrefix(ct, encPrefix) {
		t.Errorf("output missing prefix: %q", ct)
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
