// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package keys

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestAtomicWrite_NoTempFileLeftover verifies that after a successful save
// no .tmp-* sibling files remain in the keys directory.
func TestAtomicWrite_NoTempFileLeftover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")

	ks, err := LoadKeyStore(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := ks.AddUser(&UserProfile{ID: "alice", Key: "sk_alice"}); err != nil {
		t.Fatalf("AddUser: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		name := e.Name()
		if name != "keys.json" && name != "keys.json.lock" {
			t.Errorf("unexpected file in dir after write: %s", name)
		}
	}
}

// TestAtomicWrite_FileIsAlwaysValidJSON sweeps writes from multiple
// goroutines and verifies the on-disk file is parseable at every observation.
// This catches half-written files (the bug atomic writes are designed to prevent).
func TestAtomicWrite_FileIsAlwaysValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")

	ks, err := LoadKeyStore(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Writers
	for w := 0; w < 5; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				select {
				case <-stop:
					return
				default:
				}
				_ = ks.AddUser(&UserProfile{
					ID:  string(rune('a'+w)) + "-" + string(rune('0'+i%10)),
					Key: "sk_x",
				})
			}
		}(w)
	}

	// Reader: every parse must succeed even if the writers race.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			data, err := os.ReadFile(path)
			if err != nil {
				continue // file may briefly not exist mid-rename — that's fine
			}
			if len(data) == 0 {
				continue
			}
			var probe keyStoreData
			if err := json.Unmarshal(data, &probe); err != nil {
				t.Errorf("read iter %d saw partial JSON: %v\nfile: %s", i, err, string(data))
				return
			}
		}
	}()

	wg.Wait()
	close(stop)
}

// TestAtomicWrite_LockFileCreated verifies the .lock sibling exists after
// any write so cross-process flock has somewhere to grab.
func TestAtomicWrite_LockFileCreated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")

	ks, _ := LoadKeyStore(path)
	_ = ks.AddUser(&UserProfile{ID: "alice", Key: "sk_alice"})

	if _, err := os.Stat(path + ".lock"); err != nil {
		t.Errorf("expected lock file at %s.lock, got: %v", path, err)
	}
}

// TestAtomicWrite_PermissionsArePrivate verifies the on-disk file is mode 0600.
func TestAtomicWrite_PermissionsArePrivate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")

	ks, _ := LoadKeyStore(path)
	_ = ks.AddUser(&UserProfile{ID: "alice", Key: "sk_alice"})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	mode := info.Mode().Perm()
	if mode != 0o600 {
		t.Errorf("file mode: got %#o, want 0600", mode)
	}
}
