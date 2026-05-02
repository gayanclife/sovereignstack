// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package logging configures the standard library's log/slog logger from
// SovereignStack's config.LogConfig, and exposes a small set of helpers
// that all services (gateway, management, visibility, CLI) use.
//
// Once Init has been called, callers should use slog.Info/Warn/Error/Debug
// directly. Init replaces the default slog handler — there is no separate
// "package logger" instance to thread through.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/gayanclife/sovereignstack/core/config"
)

// Init configures slog.Default based on cfg.Log. Must be called once at
// program start, before any logging. Returns the resolved level so callers
// can include it in startup banners if they want.
//
// Format defaults to "text" when cfg.Format is empty or unrecognised.
// Level defaults to "info".
func Init(cfg config.LogConfig) (slog.Level, error) {
	return InitTo(cfg, os.Stderr)
}

// InitTo is like Init but writes to the given io.Writer. Used by tests.
func InitTo(cfg config.LogConfig, w io.Writer) (slog.Level, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return level, err
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(cfg.Format)) {
	case "", "text":
		handler = slog.NewTextHandler(w, opts)
	case "json":
		handler = slog.NewJSONHandler(w, opts)
	default:
		return level, fmt.Errorf("log.format: unknown value %q (want text or json)", cfg.Format)
	}

	slog.SetDefault(slog.New(handler))
	return level, nil
}

// parseLevel maps the config string to a slog.Level. Empty defaults to info.
func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("log.level: unknown value %q (want debug, info, warn, error)", s)
	}
}

// Service returns a logger pre-tagged with service="<name>". Services
// should call this at startup and pass the result down to handlers via
// context or struct fields.
func Service(name string) *slog.Logger {
	return slog.Default().With("service", name)
}
