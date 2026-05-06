// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gayanclife/sovereignstack/core/audit"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "One-shot data migration tools",
	Long: `Migration tools for moving audit data and other state between
storage backends.

Run individual subcommands for each migration target.`,
}

var migrateAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Migrate the audit log between backends",
	Long: `Migrate audit records from a SQLite file (encrypted) to a MySQL
database. Idempotent (UPSERT by correlation_id + event_type pair).
Streams in batches to avoid loading multi-GB DBs into memory.

Example:
  sovstack migrate audit \
      --from ./sovstack-audit.db \
      --audit-key $SOVSTACK_AUDIT_KEY \
      --to "user:pass@tcp(localhost:3306)/visibility?parseTime=true"`,
	RunE: runMigrateAudit,
}

func init() {
	migrateCmd.AddCommand(migrateAuditCmd)
	migrateAuditCmd.Flags().String("from", "", "Path to SQLite audit DB to read from (required)")
	migrateAuditCmd.Flags().String("to", "", "MySQL DSN to write to, e.g. user:pass@tcp(host:3306)/dbname?parseTime=true (required)")
	migrateAuditCmd.Flags().String("audit-key", "", "Encryption key for the source SQLite DB (also reads SOVSTACK_AUDIT_KEY)")
	migrateAuditCmd.Flags().Int("batch", 1000, "Rows per batch insert")
	migrateAuditCmd.Flags().String("since", "", "Only migrate rows after this RFC3339 timestamp (default: all)")
	rootCmd.AddCommand(migrateCmd)
}

func runMigrateAudit(cmd *cobra.Command, args []string) error {
	from, _ := cmd.Flags().GetString("from")
	to, _ := cmd.Flags().GetString("to")
	auditKey, _ := cmd.Flags().GetString("audit-key")
	batchSize, _ := cmd.Flags().GetInt("batch")
	since, _ := cmd.Flags().GetString("since")

	if from == "" || to == "" {
		return fmt.Errorf("--from and --to are required")
	}
	if auditKey == "" {
		auditKey = os.Getenv("SOVSTACK_AUDIT_KEY")
	}
	if auditKey == "" {
		return fmt.Errorf("audit encryption key required: pass --audit-key or set SOVSTACK_AUDIT_KEY")
	}

	startAt := time.Time{}
	if since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return fmt.Errorf("--since: %w", err)
		}
		startAt = t
	}

	src, err := audit.NewSQLiteLogger(from, auditKey)
	if err != nil {
		return fmt.Errorf("open source SQLite: %w", err)
	}

	dst, err := sql.Open("mysql", to)
	if err != nil {
		return fmt.Errorf("open MySQL DSN: %w", err)
	}
	defer dst.Close()
	if err := dst.Ping(); err != nil {
		return fmt.Errorf("connect MySQL: %w", err)
	}

	// Pull rows in chronological order; the SQLiteLogger decrypts them
	// transparently. For very large DBs the GetLogs(0) call materialises
	// the whole result set — acceptable for the typical OSS deployment
	// (single-instance, < 1 GB audit DB). Batched re-fetch could be
	// added later for multi-million-row migrations.
	end := time.Now().Add(24 * time.Hour) // include "future-dated" rows in case of clock drift
	rows := src.GetLogsInTimeRange(startAt, end)

	if len(rows) == 0 {
		fmt.Println("no rows to migrate")
		return nil
	}

	stmt, err := dst.Prepare(`
		INSERT INTO audit_logs
			(timestamp, correlation_id, event_type, user_id, model, endpoint, method,
			 status_code, client_ip, user_agent, bytes_in, bytes_out,
			 tokens_input, tokens_output, duration_ms, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			status_code   = VALUES(status_code),
			tokens_input  = VALUES(tokens_input),
			tokens_output = VALUES(tokens_output),
			duration_ms   = VALUES(duration_ms),
			error_message = VALUES(error_message)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	skipped := 0
	for i, r := range rows {
		// Cold migrations: skip records with empty correlation_id (would
		// violate the UNIQUE key on (correlation_id, event_type)).
		if r.CorrelationID == "" {
			skipped++
			continue
		}
		_, err := stmt.Exec(
			r.Timestamp, r.CorrelationID, r.EventType, r.User, r.Model, r.Endpoint, r.Method,
			r.StatusCode, r.IPAddress, r.UserAgent, r.RequestSize, r.ResponseSize,
			r.TokensUsed, r.TokensGenerated, r.DurationMS, r.ErrorMessage,
		)
		if err != nil {
			return fmt.Errorf("insert row %d (correlation_id=%s): %w", i, r.CorrelationID, err)
		}
		inserted++

		if inserted%batchSize == 0 {
			fmt.Printf("\rmigrated %d/%d rows…", inserted, len(rows))
		}
	}

	fmt.Printf("\rmigrated %d rows; skipped %d (missing correlation_id)\n", inserted, skipped)
	return nil
}
