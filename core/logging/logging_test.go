// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/gayanclife/sovereignstack/core/config"
)

func TestInit_DefaultsToTextInfo(t *testing.T) {
	var buf bytes.Buffer
	level, err := InitTo(config.LogConfig{}, &buf)
	if err != nil {
		t.Fatalf("InitTo: %v", err)
	}
	if level != slog.LevelInfo {
		t.Errorf("default level: got %v, want INFO", level)
	}

	slog.Info("hello", "k", "v")
	out := buf.String()
	if !strings.Contains(out, "level=INFO") || !strings.Contains(out, "msg=hello") || !strings.Contains(out, "k=v") {
		t.Errorf("text output missing expected fields: %s", out)
	}
}

func TestInit_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	if _, err := InitTo(config.LogConfig{Format: "json", Level: "info"}, &buf); err != nil {
		t.Fatalf("InitTo: %v", err)
	}

	slog.Info("hello", "k", "v")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("expected valid JSON, got: %s (err: %v)", buf.String(), err)
	}
	if rec["msg"] != "hello" {
		t.Errorf("msg: got %v, want hello", rec["msg"])
	}
	if rec["level"] != "INFO" {
		t.Errorf("level: got %v, want INFO", rec["level"])
	}
	if rec["k"] != "v" {
		t.Errorf("k: got %v, want v", rec["k"])
	}
}

func TestInit_DebugLevelEmitsDebugLogs(t *testing.T) {
	var buf bytes.Buffer
	if _, err := InitTo(config.LogConfig{Format: "text", Level: "debug"}, &buf); err != nil {
		t.Fatalf("InitTo: %v", err)
	}
	slog.Debug("trace-event")
	if !strings.Contains(buf.String(), "trace-event") {
		t.Errorf("debug log not emitted: %s", buf.String())
	}
}

func TestInit_InfoLevelSuppressesDebug(t *testing.T) {
	var buf bytes.Buffer
	if _, err := InitTo(config.LogConfig{Format: "text", Level: "info"}, &buf); err != nil {
		t.Fatalf("InitTo: %v", err)
	}
	slog.Debug("should-not-appear")
	if strings.Contains(buf.String(), "should-not-appear") {
		t.Errorf("debug log should be suppressed at info level: %s", buf.String())
	}
}

func TestInit_RejectsUnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	_, err := InitTo(config.LogConfig{Format: "xml", Level: "info"}, &buf)
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
}

func TestInit_RejectsUnknownLevel(t *testing.T) {
	var buf bytes.Buffer
	_, err := InitTo(config.LogConfig{Format: "text", Level: "yelling"}, &buf)
	if err == nil {
		t.Fatal("expected error for unknown level")
	}
}

func TestService_TagsLogger(t *testing.T) {
	var buf bytes.Buffer
	if _, err := InitTo(config.LogConfig{Format: "json", Level: "info"}, &buf); err != nil {
		t.Fatalf("InitTo: %v", err)
	}
	logger := Service("gateway")
	logger.Info("started", "port", 8001)

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("json: %v", err)
	}
	if rec["service"] != "gateway" {
		t.Errorf("service tag missing: %v", rec)
	}
}

func TestParseLevel_WarnAlias(t *testing.T) {
	for _, in := range []string{"warn", "warning", "WARN", "Warning"} {
		l, err := parseLevel(in)
		if err != nil {
			t.Errorf("parseLevel(%q): %v", in, err)
		}
		if l != slog.LevelWarn {
			t.Errorf("parseLevel(%q): got %v, want WARN", in, l)
		}
	}
}
