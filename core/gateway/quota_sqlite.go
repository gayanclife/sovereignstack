// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package gateway

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteQuotaBackend persists token-usage counters to a local SQLite file.
// Survives gateway restarts. Suitable for single-instance OSS deployments
// where the gateway and the SQLite file live on the same host.
//
// Schema:
//
//	CREATE TABLE quota_daily(
//	    user_id   TEXT NOT NULL,
//	    day_key   TEXT NOT NULL,        -- 'YYYY-MM-DD' UTC
//	    tokens    INTEGER NOT NULL DEFAULT 0,
//	    PRIMARY KEY (user_id, day_key)
//	);
//	CREATE TABLE quota_monthly(
//	    user_id   TEXT NOT NULL,
//	    month_key TEXT NOT NULL,        -- 'YYYY-MM' UTC
//	    tokens    INTEGER NOT NULL DEFAULT 0,
//	    PRIMARY KEY (user_id, month_key)
//	);
//
// Old rows (>90 days for daily, >24 months for monthly) are not auto-pruned;
// run `sovstack quota prune` periodically if you need to keep the DB small.
type SQLiteQuotaBackend struct {
	db *sql.DB
}

// NewSQLiteQuotaBackend opens (or creates) a SQLite database at path and
// applies the quota schema if missing. The caller is responsible for calling
// Close() at shutdown.
func NewSQLiteQuotaBackend(path string) (*SQLiteQuotaBackend, error) {
	db, err := sql.Open("sqlite3", path+"?_journal=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open quota db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping quota db: %w", err)
	}
	if _, err := db.Exec(quotaSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init quota schema: %w", err)
	}
	return &SQLiteQuotaBackend{db: db}, nil
}

const quotaSchema = `
CREATE TABLE IF NOT EXISTS quota_daily (
    user_id   TEXT NOT NULL,
    day_key   TEXT NOT NULL,
    tokens    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, day_key)
);
CREATE TABLE IF NOT EXISTS quota_monthly (
    user_id   TEXT NOT NULL,
    month_key TEXT NOT NULL,
    tokens    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, month_key)
);
CREATE INDEX IF NOT EXISTS idx_quota_daily_day ON quota_daily(day_key);
CREATE INDEX IF NOT EXISTS idx_quota_monthly_month ON quota_monthly(month_key);
`

func (b *SQLiteQuotaBackend) AddDaily(userID string, tokens int64) error {
	_, err := b.db.Exec(`
		INSERT INTO quota_daily (user_id, day_key, tokens)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id, day_key) DO UPDATE SET tokens = tokens + excluded.tokens
	`, userID, dayKey(time.Now()), tokens)
	if err != nil {
		return fmt.Errorf("add daily: %w", err)
	}
	return nil
}

func (b *SQLiteQuotaBackend) AddMonthly(userID string, tokens int64) error {
	_, err := b.db.Exec(`
		INSERT INTO quota_monthly (user_id, month_key, tokens)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id, month_key) DO UPDATE SET tokens = tokens + excluded.tokens
	`, userID, monthKey(time.Now()), tokens)
	if err != nil {
		return fmt.Errorf("add monthly: %w", err)
	}
	return nil
}

func (b *SQLiteQuotaBackend) GetDaily(userID string) (int64, error) {
	var tokens int64
	err := b.db.QueryRow(`
		SELECT COALESCE(tokens, 0) FROM quota_daily WHERE user_id = ? AND day_key = ?
	`, userID, dayKey(time.Now())).Scan(&tokens)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get daily: %w", err)
	}
	return tokens, nil
}

func (b *SQLiteQuotaBackend) GetMonthly(userID string) (int64, error) {
	var tokens int64
	err := b.db.QueryRow(`
		SELECT COALESCE(tokens, 0) FROM quota_monthly WHERE user_id = ? AND month_key = ?
	`, userID, monthKey(time.Now())).Scan(&tokens)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get monthly: %w", err)
	}
	return tokens, nil
}

func (b *SQLiteQuotaBackend) Close() error {
	return b.db.Close()
}
