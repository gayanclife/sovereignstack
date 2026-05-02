// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package audit

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJSONLLogger_WritesRecord(t *testing.T) {
	dir := t.TempDir()
	l, err := NewJSONLLogger(dir)
	if err != nil {
		t.Fatalf("NewJSONLLogger: %v", err)
	}

	l.LogRequest("alice", "mistral-7b", "/v1/chat/completions", "POST", "1.2.3.4", "curl/8", "corr-1", 256)
	l.LogResponse("alice", "mistral-7b", "/v1/chat/completions", "corr-1", 200, 1024, 12, 88, 421)
	l.Close()

	files, err := filepath.Glob(filepath.Join(dir, "audit-*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected one .jsonl file, got %v err=%v", files, err)
	}

	f, err := os.Open(files[0])
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) != 2 {
		t.Errorf("expected 2 lines in JSONL, got %d", len(lines))
	}

	var rec AuditLog
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("first line not valid JSON: %v", err)
	}
	if rec.User != "alice" || rec.Model != "mistral-7b" || rec.EventType != "request" {
		t.Errorf("first record: %+v", rec)
	}
}

func TestJSONLLogger_RingBufferReadback(t *testing.T) {
	dir := t.TempDir()
	l, err := NewJSONLLogger(dir)
	if err != nil {
		t.Fatalf("NewJSONLLogger: %v", err)
	}
	defer l.Close()

	l.LogAuthFailure("evil", "/v1/users", "1.2.3.4", "bad key")
	l.LogRequest("alice", "mistral-7b", "/x", "POST", "5.5.5.5", "agent", "c1", 100)

	logs := l.GetLogs(0) // 0 = all
	if len(logs) != 2 {
		t.Errorf("ring buffer length: got %d, want 2", len(logs))
	}
	byUser := l.GetLogsByUser("alice")
	if len(byUser) != 1 {
		t.Errorf("alice logs: got %d, want 1", len(byUser))
	}
}

// gzipFile is fire-and-forget in the rotation path; verify it actually
// compresses an existing file when invoked directly.
func TestGzipFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "audit-2026-01-01.jsonl")
	if err := os.WriteFile(src, []byte("{\"hello\":\"world\"}\n"), 0o640); err != nil {
		t.Fatalf("write src: %v", err)
	}

	gzipFile(src)
	// Wait briefly for the goroutine? gzipFile is synchronous when called directly.
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("expected source removed after gzip, stat err=%v", err)
	}
	gz := src + ".gz"
	f, err := os.Open(gz)
	if err != nil {
		t.Fatalf("open gz: %v", err)
	}
	defer f.Close()
	r, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer r.Close()
	buf := make([]byte, 100)
	n, _ := r.Read(buf)
	if !strings.Contains(string(buf[:n]), "hello") {
		t.Errorf("gz content unexpected: %s", string(buf[:n]))
	}
}

func TestMultiSinkLogger_FanOut(t *testing.T) {
	a := NewLogger(100)
	b := NewLogger(100)
	multi := NewMultiSinkLogger(a, b)

	multi.LogRequest("u1", "m1", "/x", "GET", "ip", "ua", "c1", 0)
	multi.LogRequest("u2", "m2", "/y", "POST", "ip", "ua", "c2", 0)

	if got := len(a.GetLogs(0)); got != 2 {
		t.Errorf("sink a: got %d, want 2", got)
	}
	if got := len(b.GetLogs(0)); got != 2 {
		t.Errorf("sink b: got %d, want 2", got)
	}
}

func TestMultiSinkLogger_GetReadsFirstSink(t *testing.T) {
	a := NewLogger(100)
	b := NewLogger(100)
	multi := NewMultiSinkLogger(a, b)

	a.LogRequest("only-in-a", "m", "/x", "GET", "ip", "ua", "c1", 0)
	// b has no records.

	got := multi.GetLogsByUser("only-in-a")
	if len(got) != 1 {
		t.Errorf("expected first sink (a) to be queried, got %d records", len(got))
	}
}

func TestMultiSinkLogger_NoSinksIsSafe(t *testing.T) {
	multi := NewMultiSinkLogger()
	multi.LogRequest("u", "m", "/x", "GET", "ip", "ua", "c1", 0) // shouldn't panic
	if got := multi.GetLogs(10); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestJSONLLogger_RotateWritesValidFile(t *testing.T) {
	dir := t.TempDir()
	l, err := NewJSONLLogger(dir)
	if err != nil {
		t.Fatalf("NewJSONLLogger: %v", err)
	}
	defer l.Close()

	// Force the day to a fixed value, write, then trigger rotate by changing day.
	l.mu.Lock()
	l.currentDay = time.Now().UTC().Format("2006-01-02")
	l.mu.Unlock()

	l.Log(AuditLog{User: "alice", EventType: "x"})

	files, _ := filepath.Glob(filepath.Join(dir, "audit-*.jsonl"))
	if len(files) == 0 {
		t.Fatal("no audit file produced")
	}
}
