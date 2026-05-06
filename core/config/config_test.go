// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.Gateway.Port != 8001 {
		t.Errorf("expected gateway port 8001, got %d", cfg.Gateway.Port)
	}
	if cfg.Management.Port != 8888 {
		t.Errorf("expected management port 8888, got %d", cfg.Management.Port)
	}
	if cfg.Visibility.Port != 9000 {
		t.Errorf("expected visibility port 9000, got %d", cfg.Visibility.Port)
	}
	if cfg.Log.Format != "text" {
		t.Errorf("expected log format text, got %s", cfg.Log.Format)
	}
}

func TestLoad_MissingFileIsOK(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err != nil {
		t.Fatalf("Load on missing file should not error: %v", err)
	}
	if cfg.Gateway.Port != 8001 {
		t.Errorf("expected defaults preserved, got port %d", cfg.Gateway.Port)
	}
}

func TestLoad_EmptyPathReturnsDefaultsPlusEnv(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load with empty path: %v", err)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("expected default log level, got %s", cfg.Log.Level)
	}
}

func TestLoad_YAMLOverridesDefaults(t *testing.T) {
	yamlContent := `
log:
  format: json
  level: debug
gateway:
  port: 9999
  rate_limit: 250
  backend: http://api.local:8000
cors:
  origins:
    - http://localhost:3000
    - https://app.example.com
  allow_dev_wildcard: false
`
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Log.Format != "json" {
		t.Errorf("log.format: got %s, want json", cfg.Log.Format)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("log.level: got %s, want debug", cfg.Log.Level)
	}
	if cfg.Gateway.Port != 9999 {
		t.Errorf("gateway.port: got %d, want 9999", cfg.Gateway.Port)
	}
	if cfg.Gateway.RateLimit != 250 {
		t.Errorf("gateway.rate_limit: got %v, want 250", cfg.Gateway.RateLimit)
	}
	if cfg.Gateway.Backend != "http://api.local:8000" {
		t.Errorf("gateway.backend: got %s", cfg.Gateway.Backend)
	}
	if len(cfg.CORS.Origins) != 2 {
		t.Errorf("cors.origins: got %v", cfg.CORS.Origins)
	}
}

func TestLoad_EnvOverridesYAML(t *testing.T) {
	yamlContent := `
log:
  level: info
gateway:
  port: 8001
management:
  admin_key: from_yaml
`
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	t.Setenv("SOVSTACK_LOG_LEVEL", "debug")
	t.Setenv("SOVSTACK_GATEWAY_PORT", "7777")
	t.Setenv("SOVSTACK_ADMIN_KEY", "from_env")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Log.Level != "debug" {
		t.Errorf("log.level: env override failed, got %s", cfg.Log.Level)
	}
	if cfg.Gateway.Port != 7777 {
		t.Errorf("gateway.port: env override failed, got %d", cfg.Gateway.Port)
	}
	if cfg.Management.AdminKey != "from_env" {
		t.Errorf("admin_key: env override failed, got %s", cfg.Management.AdminKey)
	}
}

func TestLoad_MalformedYAMLIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte("gateway:\n  port: not-a-number\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestApplyEnv_InvalidIntFails(t *testing.T) {
	t.Setenv("SOVSTACK_GATEWAY_PORT", "not-a-port")

	_, err := Load("")
	if err == nil {
		t.Fatal("expected error for non-integer port env var")
	}
}

func TestApplyEnv_BoolParsing(t *testing.T) {
	yamlContent := `
cors:
  allow_dev_wildcard: false
`
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CORS.AllowDevWildcard {
		t.Error("expected allow_dev_wildcard false from yaml")
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := expandPath("~/foo/bar")
	want := filepath.Join(home, "foo/bar")
	if got != want {
		t.Errorf("expandPath: got %s, want %s", got, want)
	}

	if expandPath("/abs/path") != "/abs/path" {
		t.Error("expandPath should leave absolute paths alone")
	}
}

func TestLoad_ResolvesTildeInPath(t *testing.T) {
	// Resolve the home dir, write a config there with a unique name, then
	// load via the ~ path and verify it parses.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	name := ".sovstack-test-" + t.Name() + ".yaml"
	abs := filepath.Join(home, name)
	t.Cleanup(func() { os.Remove(abs) })

	yamlContent := "gateway:\n  port: 4242\n"
	if err := os.WriteFile(abs, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := Load("~/" + name)
	if err != nil {
		t.Fatalf("Load via ~: %v", err)
	}
	if cfg.Gateway.Port != 4242 {
		t.Errorf("expected port 4242 from ~ path, got %d", cfg.Gateway.Port)
	}
}
