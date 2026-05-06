// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package gateway

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gayanclife/sovereignstack/core/config"
)

// runBackendContract verifies the QuotaBackend contract against any implementation.
// Used by both memory and sqlite tests.
func runBackendContract(t *testing.T, b QuotaBackend) {
	t.Helper()

	// Empty user starts at 0
	got, err := b.GetDaily("alice")
	if err != nil {
		t.Fatalf("GetDaily on empty: %v", err)
	}
	if got != 0 {
		t.Errorf("empty daily: got %d, want 0", got)
	}

	got, err = b.GetMonthly("alice")
	if err != nil {
		t.Fatalf("GetMonthly on empty: %v", err)
	}
	if got != 0 {
		t.Errorf("empty monthly: got %d, want 0", got)
	}

	// Add daily and monthly accumulate independently
	if err := b.AddDaily("alice", 100); err != nil {
		t.Fatalf("AddDaily: %v", err)
	}
	if err := b.AddDaily("alice", 50); err != nil {
		t.Fatalf("AddDaily: %v", err)
	}
	if err := b.AddMonthly("alice", 200); err != nil {
		t.Fatalf("AddMonthly: %v", err)
	}

	got, _ = b.GetDaily("alice")
	if got != 150 {
		t.Errorf("daily after 100+50: got %d, want 150", got)
	}
	got, _ = b.GetMonthly("alice")
	if got != 200 {
		t.Errorf("monthly: got %d, want 200", got)
	}

	// Different users isolated
	if err := b.AddDaily("bob", 99); err != nil {
		t.Fatalf("AddDaily bob: %v", err)
	}
	got, _ = b.GetDaily("alice")
	if got != 150 {
		t.Errorf("alice daily after bob's update: got %d, want 150", got)
	}
	got, _ = b.GetDaily("bob")
	if got != 99 {
		t.Errorf("bob daily: got %d, want 99", got)
	}
}

func TestMemoryQuotaBackend_Contract(t *testing.T) {
	b := NewMemoryQuotaBackend()
	defer b.Close()
	runBackendContract(t, b)
}

func TestSQLiteQuotaBackend_Contract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota.db")
	b, err := NewSQLiteQuotaBackend(path)
	if err != nil {
		t.Fatalf("NewSQLiteQuotaBackend: %v", err)
	}
	defer b.Close()
	runBackendContract(t, b)
}

func TestSQLiteQuotaBackend_PersistsAcrossOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota.db")

	// First instance writes
	b1, err := NewSQLiteQuotaBackend(path)
	if err != nil {
		t.Fatalf("open #1: %v", err)
	}
	if err := b1.AddDaily("alice", 12345); err != nil {
		t.Fatalf("AddDaily: %v", err)
	}
	if err := b1.AddMonthly("alice", 67890); err != nil {
		t.Fatalf("AddMonthly: %v", err)
	}
	b1.Close()

	// Second instance reads — survives the close.
	b2, err := NewSQLiteQuotaBackend(path)
	if err != nil {
		t.Fatalf("open #2: %v", err)
	}
	defer b2.Close()

	if got, _ := b2.GetDaily("alice"); got != 12345 {
		t.Errorf("daily after restart: got %d, want 12345", got)
	}
	if got, _ := b2.GetMonthly("alice"); got != 67890 {
		t.Errorf("monthly after restart: got %d, want 67890", got)
	}
}

func TestMemoryQuotaBackend_DoesNotPersist(t *testing.T) {
	b1 := NewMemoryQuotaBackend()
	_ = b1.AddDaily("alice", 100)
	b1.Close()

	// New instance starts fresh.
	b2 := NewMemoryQuotaBackend()
	defer b2.Close()
	if got, _ := b2.GetDaily("alice"); got != 0 {
		t.Errorf("expected fresh memory backend to be empty, got %d", got)
	}
}

func TestMemoryQuotaBackend_Concurrency(t *testing.T) {
	b := NewMemoryQuotaBackend()
	defer b.Close()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.AddDaily("alice", 1)
		}()
	}
	wg.Wait()

	if got, _ := b.GetDaily("alice"); got != 100 {
		t.Errorf("concurrent adds: got %d, want 100", got)
	}
}

func TestBuildQuotaBackend_Memory(t *testing.T) {
	b, name, err := BuildQuotaBackend(config.QuotaConfig{Backend: "memory"})
	if err != nil {
		t.Fatalf("BuildQuotaBackend(memory): %v", err)
	}
	defer b.Close()
	if _, ok := b.(*MemoryQuotaBackend); !ok {
		t.Errorf("expected *MemoryQuotaBackend, got %T", b)
	}
	if name != "memory" {
		t.Errorf("name: %s", name)
	}
}

func TestBuildQuotaBackend_SQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota.db")
	b, name, err := BuildQuotaBackend(config.QuotaConfig{
		Backend:  "sqlite",
		SQLiteDB: path,
	})
	if err != nil {
		t.Fatalf("BuildQuotaBackend(sqlite): %v", err)
	}
	defer b.Close()
	if _, ok := b.(*SQLiteQuotaBackend); !ok {
		t.Errorf("expected *SQLiteQuotaBackend, got %T", b)
	}
	if name == "" {
		t.Error("name should not be empty")
	}
}

func TestBuildQuotaBackend_DefaultIsMemory(t *testing.T) {
	b, _, err := BuildQuotaBackend(config.QuotaConfig{}) // empty == memory
	if err != nil {
		t.Fatalf("BuildQuotaBackend default: %v", err)
	}
	defer b.Close()
	if _, ok := b.(*MemoryQuotaBackend); !ok {
		t.Errorf("default backend should be memory, got %T", b)
	}
}

func TestBuildQuotaBackend_UnknownErrors(t *testing.T) {
	_, _, err := BuildQuotaBackend(config.QuotaConfig{Backend: "rocksdb"})
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}

func TestBuildQuotaBackend_SQLiteRequiresPath(t *testing.T) {
	_, _, err := BuildQuotaBackend(config.QuotaConfig{Backend: "sqlite", SQLiteDB: ""})
	if err == nil {
		t.Fatal("expected error when sqlite_db is empty")
	}
}

func TestBuildQuotaBackend_RedisRequiresAddr(t *testing.T) {
	_, _, err := BuildQuotaBackend(config.QuotaConfig{Backend: "redis"})
	if err == nil {
		t.Fatal("expected error when redis.addr is empty")
	}
}

// dayKey/monthKey are time-of-day stable for "now"; just sanity-check format.
func TestDayKey_Format(t *testing.T) {
	k := dayKey(time.Date(2026, 5, 1, 14, 30, 0, 0, time.UTC))
	if k != "2026-05-01" {
		t.Errorf("dayKey: got %s, want 2026-05-01", k)
	}
}

func TestMonthKey_Format(t *testing.T) {
	k := monthKey(time.Date(2026, 5, 1, 14, 30, 0, 0, time.UTC))
	if k != "2026-05" {
		t.Errorf("monthKey: got %s, want 2026-05", k)
	}
}
