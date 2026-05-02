// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package tls

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve_InsecureHTTPReturnsEmpty(t *testing.T) {
	cert, key, fp, err := Resolve("", "", t.TempDir(), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cert != "" || key != "" || fp != "" {
		t.Errorf("insecure http should return empty paths/fp; got %q %q %q", cert, key, fp)
	}
}

func TestResolve_GeneratesSelfSigned(t *testing.T) {
	dir := t.TempDir()
	cert, key, fp, err := Resolve("", "", dir, false, []string{"localhost", "127.0.0.1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cert == "" || key == "" {
		t.Fatal("expected paths populated")
	}
	if !strings.HasPrefix(fp, "sha256:") {
		t.Errorf("fingerprint should start with sha256:, got %s", fp)
	}

	// Files exist and are loadable as a TLS keypair.
	if _, err := os.Stat(cert); err != nil {
		t.Errorf("cert file missing: %v", err)
	}
	if _, err := os.Stat(key); err != nil {
		t.Errorf("key file missing: %v", err)
	}
	if _, err := tls.LoadX509KeyPair(cert, key); err != nil {
		t.Errorf("generated pair not loadable: %v", err)
	}
}

func TestResolve_ReusesExistingFiles(t *testing.T) {
	dir := t.TempDir()
	// First call generates.
	c1, k1, fp1, err := Resolve("", "", dir, false, []string{"localhost"})
	if err != nil {
		t.Fatal(err)
	}
	// Second call should be deterministic in path and fingerprint
	// (same files, no regeneration).
	c2, k2, fp2, err := Resolve("", "", dir, false, []string{"localhost"})
	if err != nil {
		t.Fatal(err)
	}
	if c1 != c2 || k1 != k2 {
		t.Errorf("paths changed: %s/%s -> %s/%s", c1, k1, c2, k2)
	}
	if fp1 != fp2 {
		t.Errorf("fingerprint changed (cert was regenerated): %s -> %s", fp1, fp2)
	}
}

func TestResolve_ProvidedCertsPassThrough(t *testing.T) {
	dir := t.TempDir()
	// First, generate a cert/key pair we can hand back.
	cert, key, _, err := Resolve("", "", dir, false, []string{"localhost"})
	if err != nil {
		t.Fatal(err)
	}

	// Now resolve with explicit paths in a different "self-signed dir".
	otherDir := t.TempDir()
	gotCert, gotKey, fp, err := Resolve(cert, key, otherDir, false, nil)
	if err != nil {
		t.Fatalf("Resolve with explicit paths: %v", err)
	}
	if gotCert != cert || gotKey != key {
		t.Errorf("expected explicit paths returned; got %s/%s", gotCert, gotKey)
	}
	if fp == "" {
		t.Error("fingerprint should still be populated for caller-supplied cert")
	}

	// otherDir should not contain auto-generated files.
	entries, _ := os.ReadDir(otherDir)
	if len(entries) != 0 {
		t.Errorf("auto-gen dir should be empty when caller provides cert; got %d entries", len(entries))
	}
}

func TestResolve_MissingProvidedCertIsError(t *testing.T) {
	dir := t.TempDir()
	_, _, _, err := Resolve(filepath.Join(dir, "nope.pem"), filepath.Join(dir, "nope.key"), dir, false, nil)
	if err == nil {
		t.Error("expected error for missing --tls-cert path")
	}
}

func TestCertFingerprint_Format(t *testing.T) {
	dir := t.TempDir()
	cert, _, _, _ := Resolve("", "", dir, false, []string{"localhost"})

	fp, err := certFingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(fp, "sha256:") {
		t.Errorf("missing prefix: %s", fp)
	}
	// Expect 64 hex chars + 31 colons = 95 + len("sha256:") = 102
	if len(fp) != len("sha256:")+64+31 {
		t.Errorf("unexpected fingerprint length %d: %s", len(fp), fp)
	}
}
