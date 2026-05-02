// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package audit

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// JSONLLogger writes audit records to a daily-rotated JSON-lines file. It is
// the cold-path companion to the in-memory and SQLite/MySQL hot paths:
// rotated files are append-only and well-suited to long-term retention,
// log shippers, and offline tooling (jq, awk).
//
// File layout:
//
//	<base-dir>/audit-2026-05-01.jsonl       ← today (open file handle)
//	<base-dir>/audit-2026-04-30.jsonl.gz    ← yesterday (gzipped)
//	<base-dir>/audit-2026-04-29.jsonl.gz    ← older
//
// At UTC midnight the writer closes the current file and gzips it in the
// background, then opens a new file for today. Read-side tools (the
// visibility backend's compliance reporter, log shippers) glob the dir.
//
// Only the Log* methods are persisted to disk — the read methods (GetLogs,
// GetStats etc.) operate over a small in-memory ring buffer for the
// gateway's debug endpoints. To query historical data, read the JSONL
// files directly.
type JSONLLogger struct {
	dir         string
	mu          sync.Mutex
	currentFile *os.File
	currentDay  string // YYYY-MM-DD UTC
	encoder     *json.Encoder
	ringBuffer  []AuditLog
	ringMax     int
	ringMu      sync.RWMutex
}

// NewJSONLLogger opens (or creates) the directory and starts writing to
// today's file. The caller is responsible for calling Close() at shutdown.
func NewJSONLLogger(dir string) (*JSONLLogger, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create audit dir: %w", err)
	}
	l := &JSONLLogger{
		dir:        dir,
		ringBuffer: make([]AuditLog, 0, 1000),
		ringMax:    1000,
	}
	if err := l.rotate(); err != nil {
		return nil, fmt.Errorf("open initial audit file: %w", err)
	}
	return l, nil
}

func (l *JSONLLogger) currentDayUTC() string {
	return time.Now().UTC().Format("2006-01-02")
}

// rotate must be called with l.mu held (or before the file is in use).
// Closes the current file (if any) and opens today's file.
func (l *JSONLLogger) rotate() error {
	if l.currentFile != nil {
		oldPath := l.currentFile.Name()
		_ = l.currentFile.Close()
		// Best-effort: gzip the previous file in the background.
		go gzipFile(oldPath)
	}

	day := l.currentDayUTC()
	path := filepath.Join(l.dir, "audit-"+day+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	l.currentFile = f
	l.currentDay = day
	l.encoder = json.NewEncoder(f)
	return nil
}

// gzipFile compresses src into src+".gz" and removes src on success.
// Errors are silently dropped — JSONL files remain readable even if gzip fails.
func gzipFile(src string) {
	if filepath.Ext(src) == ".gz" {
		return
	}
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()

	out, err := os.Create(src + ".gz")
	if err != nil {
		return
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	if _, err := io.Copy(gz, in); err != nil {
		gz.Close()
		return
	}
	if err := gz.Close(); err != nil {
		return
	}
	_ = os.Remove(src)
}

// Log writes a single record to today's file (rotating if the day changed)
// and to the in-memory ring buffer.
func (l *JSONLLogger) Log(entry AuditLog) {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	l.mu.Lock()
	if l.currentDayUTC() != l.currentDay {
		if err := l.rotate(); err != nil {
			// Logging errors are best-effort — we don't fail the request path.
			l.mu.Unlock()
			return
		}
	}
	_ = l.encoder.Encode(entry)
	l.mu.Unlock()

	l.ringMu.Lock()
	l.ringBuffer = append(l.ringBuffer, entry)
	if len(l.ringBuffer) > l.ringMax {
		l.ringBuffer = l.ringBuffer[len(l.ringBuffer)-l.ringMax:]
	}
	l.ringMu.Unlock()
}

func (l *JSONLLogger) LogRequest(user, model, endpoint, method, ipAddress, userAgent, correlationID string, requestSize int64) {
	l.Log(AuditLog{
		Timestamp: time.Now(), EventType: "request", Level: AuditLevelInfo,
		User: user, Model: model, Method: method, Endpoint: endpoint,
		RequestSize: requestSize, IPAddress: ipAddress, UserAgent: userAgent,
		CorrelationID: correlationID,
	})
}

func (l *JSONLLogger) LogResponse(user, model, endpoint, correlationID string, statusCode int, responseSize int64, tokensUsed, tokensGenerated int, durationMS int64) {
	l.Log(AuditLog{
		Timestamp: time.Now(), EventType: "response", Level: AuditLevelInfo,
		User: user, Model: model, Endpoint: endpoint,
		ResponseSize: responseSize, TokensUsed: tokensUsed,
		TokensGenerated: tokensGenerated, DurationMS: durationMS,
		StatusCode: statusCode, CorrelationID: correlationID,
	})
}

func (l *JSONLLogger) LogError(user, endpoint, correlationID, errMsg string, statusCode int, ipAddress string) {
	l.Log(AuditLog{
		Timestamp: time.Now(), EventType: "error", Level: AuditLevelError,
		User: user, Endpoint: endpoint, ErrorMessage: errMsg,
		StatusCode: statusCode, IPAddress: ipAddress, CorrelationID: correlationID,
	})
}

func (l *JSONLLogger) LogAuthFailure(user, endpoint, ipAddress, reason string) {
	l.Log(AuditLog{
		Timestamp: time.Now(), EventType: "auth_failure", Level: AuditLevelWarn,
		User: user, Endpoint: endpoint,
		ErrorMessage: fmt.Sprintf("auth failed: %s", reason),
		StatusCode:   401, IPAddress: ipAddress,
	})
}

// GetLogs returns the most-recent N entries from the in-memory ring buffer.
// For historical data beyond the ring window, read the JSONL files directly.
func (l *JSONLLogger) GetLogs(n int) []AuditLog {
	l.ringMu.RLock()
	defer l.ringMu.RUnlock()
	if n <= 0 || n > len(l.ringBuffer) {
		n = len(l.ringBuffer)
	}
	start := len(l.ringBuffer) - n
	out := make([]AuditLog, n)
	copy(out, l.ringBuffer[start:])
	return out
}

func (l *JSONLLogger) GetLogsByUser(user string) []AuditLog {
	l.ringMu.RLock()
	defer l.ringMu.RUnlock()
	var out []AuditLog
	for _, e := range l.ringBuffer {
		if e.User == user {
			out = append(out, e)
		}
	}
	return out
}

func (l *JSONLLogger) GetLogsByModel(model string) []AuditLog {
	l.ringMu.RLock()
	defer l.ringMu.RUnlock()
	var out []AuditLog
	for _, e := range l.ringBuffer {
		if e.Model == model {
			out = append(out, e)
		}
	}
	return out
}

func (l *JSONLLogger) GetLogsInTimeRange(start, end time.Time) []AuditLog {
	l.ringMu.RLock()
	defer l.ringMu.RUnlock()
	var out []AuditLog
	for _, e := range l.ringBuffer {
		if e.Timestamp.After(start) && e.Timestamp.Before(end) {
			out = append(out, e)
		}
	}
	return out
}

func (l *JSONLLogger) GetStats() map[string]interface{} {
	l.ringMu.RLock()
	defer l.ringMu.RUnlock()
	return map[string]interface{}{
		"total_logs": len(l.ringBuffer),
		"file_dir":   l.dir,
		"current_day": l.currentDay,
		"note":       "GetStats only reflects the in-memory ring buffer; query JSONL files for historical totals",
	}
}

// Close flushes and closes the current file. The file is then ready to be
// gzipped (which happens automatically on the next process startup that
// rotates over it, or you can call BackgroundCompress).
func (l *JSONLLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.currentFile == nil {
		return nil
	}
	err := l.currentFile.Close()
	l.currentFile = nil
	return err
}
