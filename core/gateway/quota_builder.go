// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package gateway

import (
	"fmt"
	"strings"

	"github.com/gayanclife/sovereignstack/core/config"
)

// BuildQuotaBackend constructs a QuotaBackend from a config.QuotaConfig.
//
// Selection rules:
//   - "" or "memory" → in-memory (lost on restart, single-instance)
//   - "sqlite"        → file at cfg.SQLiteDB (defaults handled by Defaults())
//   - "redis"         → Redis at cfg.Redis.Addr; verifies connectivity at startup
//
// Returns the chosen backend and a human-readable name for log/banner output.
func BuildQuotaBackend(cfg config.QuotaConfig) (QuotaBackend, string, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Backend)) {
	case "", "memory":
		return NewMemoryQuotaBackend(), "memory", nil
	case "sqlite":
		if cfg.SQLiteDB == "" {
			return nil, "", fmt.Errorf("quota.sqlite_db must be set when quota.backend=sqlite")
		}
		b, err := NewSQLiteQuotaBackend(cfg.SQLiteDB)
		if err != nil {
			return nil, "", fmt.Errorf("sqlite quota backend: %w", err)
		}
		return b, "sqlite (" + cfg.SQLiteDB + ")", nil
	case "redis":
		if cfg.Redis.Addr == "" {
			return nil, "", fmt.Errorf("quota.redis.addr must be set when quota.backend=redis")
		}
		b, err := NewRedisQuotaBackend(RedisQuotaConfig{
			Addr:      cfg.Redis.Addr,
			Password:  cfg.Redis.Password,
			DB:        cfg.Redis.DB,
			KeyPrefix: cfg.Redis.KeyPrefix,
		})
		if err != nil {
			return nil, "", fmt.Errorf("redis quota backend: %w", err)
		}
		return b, "redis (" + cfg.Redis.Addr + ")", nil
	default:
		return nil, "", fmt.Errorf("quota.backend: unknown value %q (want memory, sqlite, or redis)", cfg.Backend)
	}
}
