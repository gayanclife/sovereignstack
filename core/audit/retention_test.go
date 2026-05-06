// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package audit

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestPruner_DeletesOldRows(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewSQLiteLogger(filepath.Join(dir, "audit.db"), "test-key")
	if err != nil {
		t.Fatal(err)
	}

	// Three rows: one old, two recent.
	logger.Log(AuditLog{Timestamp: time.Now().Add(-48 * time.Hour), EventType: "request", User: "alice"})
	logger.Log(AuditLog{Timestamp: time.Now(), EventType: "request", User: "bob"})
	logger.Log(AuditLog{Timestamp: time.Now(), EventType: "request", User: "carol"})

	// Retain 24h → delete the 48h-old row.
	pruner := NewPruner(logger.DB(), 24*time.Hour, time.Hour, nil)
	deleted, err := pruner.PruneOnce(context.Background())
	if err != nil {
		t.Fatalf("PruneOnce: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 row deleted, got %d", deleted)
	}

	remaining := logger.GetLogs(100)
	if len(remaining) != 2 {
		t.Errorf("expected 2 rows remaining, got %d", len(remaining))
	}
}

func TestPruner_NoOpWhenRetentionZero(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewSQLiteLogger(filepath.Join(dir, "audit.db"), "test-key")
	if err != nil {
		t.Fatal(err)
	}
	logger.Log(AuditLog{Timestamp: time.Now().Add(-365 * 24 * time.Hour), User: "ancient"})

	pruner := NewPruner(logger.DB(), 0, time.Hour, nil)
	deleted, err := pruner.PruneOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 0 {
		t.Errorf("zero retention should be a no-op, got deleted=%d", deleted)
	}
}

func TestPruner_PruneOnceIdempotent(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewSQLiteLogger(filepath.Join(dir, "audit.db"), "test-key")
	if err != nil {
		t.Fatal(err)
	}
	logger.Log(AuditLog{Timestamp: time.Now().Add(-72 * time.Hour), User: "old"})
	logger.Log(AuditLog{Timestamp: time.Now(), User: "fresh"})

	p := NewPruner(logger.DB(), 24*time.Hour, time.Hour, nil)
	first, _ := p.PruneOnce(context.Background())
	second, _ := p.PruneOnce(context.Background())
	if first != 1 || second != 0 {
		t.Errorf("first prune deleted=%d (want 1); second=%d (want 0)", first, second)
	}
}

func TestPruner_StartStop(t *testing.T) {
	dir := t.TempDir()
	logger, _ := NewSQLiteLogger(filepath.Join(dir, "audit.db"), "test-key")
	p := NewPruner(logger.DB(), 24*time.Hour, time.Hour, nil)

	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)
	cancel() // should terminate the goroutine cleanly

	// Calling Stop after ctx.Cancel must not panic.
	p.Stop()
	p.Stop() // double-stop also safe
}
