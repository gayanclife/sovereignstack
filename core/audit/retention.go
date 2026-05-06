// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package audit

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Pruner deletes audit_logs rows older than retention from a SQLite DB
// on a periodic interval. Phase G2.
//
// Use case: on a long-running gateway, the SQLite hot-path file would
// otherwise grow forever. Operators set retention via
// `gateway.audit.retention_days`; the pruner runs every interval (1h
// default) and DELETEs anything older.
//
// JSONL files are NOT pruned by this — they're cold-path archives,
// rotated daily and gzipped, and operators are expected to manage them
// with logrotate or a cron job.
type Pruner struct {
	db        *sql.DB
	retention time.Duration
	interval  time.Duration

	// notify, if non-nil, is invoked after every prune (success or
	// failure) with the rows-deleted count and the error. Useful for
	// emitting structured logs without coupling this package to slog.
	notify func(deleted int64, err error)

	stop chan struct{}
}

// NewPruner returns a Pruner that runs every interval (1 hour minimum)
// and deletes rows older than retention. Pass retention <= 0 to get a
// no-op Pruner — handy for "feature gated" call sites that pass through
// configuration unconditionally.
func NewPruner(db *sql.DB, retention, interval time.Duration, notify func(int64, error)) *Pruner {
	if interval < time.Minute {
		interval = time.Hour
	}
	return &Pruner{
		db:        db,
		retention: retention,
		interval:  interval,
		notify:    notify,
		stop:      make(chan struct{}),
	}
}

// Start kicks off the background loop. Returns immediately. The first
// prune happens after `interval`, not at start (so process boot doesn't
// hold up on a big delete). Cancelling ctx (or calling Stop) terminates.
func (p *Pruner) Start(ctx context.Context) {
	if p.retention <= 0 {
		return // no-op when retention is disabled
	}
	go p.loop(ctx)
}

// Stop halts the background loop. Safe to call multiple times.
func (p *Pruner) Stop() {
	select {
	case <-p.stop:
		// already stopped
	default:
		close(p.stop)
	}
}

func (p *Pruner) loop(ctx context.Context) {
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stop:
			return
		case <-t.C:
			deleted, err := p.PruneOnce(ctx)
			if p.notify != nil {
				p.notify(deleted, err)
			}
		}
	}
}

// PruneOnce runs a single DELETE pass and returns the number of rows
// removed. Exposed so the keys-CLI can offer `sovstack audit prune` for
// one-shot manual cleanup.
func (p *Pruner) PruneOnce(ctx context.Context) (int64, error) {
	if p.retention <= 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-p.retention)
	res, err := p.db.ExecContext(ctx,
		`DELETE FROM audit_logs WHERE timestamp < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("audit prune: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
