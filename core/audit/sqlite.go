/*
Copyright 2026 SovereignStack Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package audit

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	_ "modernc.org/sqlite"
	"golang.org/x/crypto/pbkdf2"
	"crypto/sha256"
)

// SQLiteLogger persists audit logs to an encrypted SQLite database
type SQLiteLogger struct {
	db  *sql.DB
	key []byte // 32-byte AES-256 key derived via PBKDF2
}

// NewSQLiteLogger creates a new SQLite-backed audit logger
// encryptionKey is derived to a 32-byte key via PBKDF2-SHA256
func NewSQLiteLogger(dbPath string, encryptionKey string) (*SQLiteLogger, error) {
	// Open or create database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create tables if they don't exist
	if err := createSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	// Get or create salt and derive key
	key, err := deriveKey(db, encryptionKey)
	if err != nil {
		db.Close()
		return nil, err
	}

	logger := &SQLiteLogger{
		db:  db,
		key: key,
	}

	return logger, nil
}

// LogRequest logs an incoming API request
func (l *SQLiteLogger) LogRequest(user, model, endpoint, method, ipAddress, userAgent, correlationID string, requestSize int64) {
	l.Log(AuditLog{
		Timestamp:     time.Now(),
		EventType:     "request",
		Level:         AuditLevelInfo,
		User:          user,
		Model:         model,
		Method:        method,
		Endpoint:      endpoint,
		RequestSize:   requestSize,
		IPAddress:     ipAddress,
		UserAgent:     userAgent,
		CorrelationID: correlationID,
	})
}

// LogResponse logs a completed API response
func (l *SQLiteLogger) LogResponse(user, model, endpoint, correlationID string, statusCode int, responseSize int64, tokensUsed, tokensGenerated int, durationMS int64) {
	l.Log(AuditLog{
		Timestamp:       time.Now(),
		EventType:       "response",
		Level:           AuditLevelInfo,
		User:            user,
		Model:           model,
		Endpoint:        endpoint,
		ResponseSize:    responseSize,
		TokensUsed:      tokensUsed,
		TokensGenerated: tokensGenerated,
		DurationMS:      durationMS,
		StatusCode:      statusCode,
		CorrelationID:   correlationID,
	})
}

// LogError logs an error event
func (l *SQLiteLogger) LogError(user, endpoint, correlationID, errMsg string, statusCode int, ipAddress string) {
	l.Log(AuditLog{
		Timestamp:     time.Now(),
		EventType:     "error",
		Level:         AuditLevelError,
		User:          user,
		Endpoint:      endpoint,
		ErrorMessage:  errMsg,
		StatusCode:    statusCode,
		IPAddress:     ipAddress,
		CorrelationID: correlationID,
	})
}

// LogAuthFailure logs authentication failures
func (l *SQLiteLogger) LogAuthFailure(user, endpoint, ipAddress, reason string) {
	l.Log(AuditLog{
		Timestamp:    time.Now(),
		EventType:    "auth_failure",
		Level:        AuditLevelWarn,
		User:         user,
		Endpoint:     endpoint,
		ErrorMessage: fmt.Sprintf("auth failed: %s", reason),
		StatusCode:   401,
		IPAddress:    ipAddress,
	})
}

// Log adds an audit entry to the database
func (l *SQLiteLogger) Log(entry AuditLog) {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	// Encrypt sensitive fields
	encIPAddr, _ := encrypt(l.key, entry.IPAddress)
	encUserAgent, _ := encrypt(l.key, entry.UserAgent)
	encErrMsg, _ := encrypt(l.key, entry.ErrorMessage)

	// Generate ID
	id := generateID()

	// Insert into database
	stmt := `INSERT INTO audit_logs (
		id, timestamp, event_type, level, user, model, method, endpoint,
		request_size, response_size, tokens_used, tokens_generated,
		duration_ms, status_code, error_message, ip_address, user_agent, correlation_id
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := l.db.Exec(stmt,
		id, entry.Timestamp, entry.EventType, entry.Level, entry.User, entry.Model,
		entry.Method, entry.Endpoint, entry.RequestSize, entry.ResponseSize,
		entry.TokensUsed, entry.TokensGenerated, entry.DurationMS, entry.StatusCode,
		encErrMsg, encIPAddr, encUserAgent, entry.CorrelationID,
	)
	if err != nil {
		fmt.Printf("Error logging to database: %v\n", err)
	}
}

// GetLogs returns the last n audit logs
func (l *SQLiteLogger) GetLogs(n int) []AuditLog {
	stmt := `SELECT id, timestamp, event_type, level, user, model, method, endpoint,
		request_size, response_size, tokens_used, tokens_generated,
		duration_ms, status_code, error_message, ip_address, user_agent, correlation_id
		FROM audit_logs ORDER BY timestamp DESC LIMIT ?`

	rows, err := l.db.Query(stmt, n)
	if err != nil {
		return nil
	}
	defer rows.Close()

	return scanLogs(rows, l.key)
}

// GetLogsByUser returns logs for a specific user
func (l *SQLiteLogger) GetLogsByUser(user string) []AuditLog {
	stmt := `SELECT id, timestamp, event_type, level, user, model, method, endpoint,
		request_size, response_size, tokens_used, tokens_generated,
		duration_ms, status_code, error_message, ip_address, user_agent, correlation_id
		FROM audit_logs WHERE user = ? ORDER BY timestamp DESC`

	rows, err := l.db.Query(stmt, user)
	if err != nil {
		return nil
	}
	defer rows.Close()

	return scanLogs(rows, l.key)
}

// GetLogsByModel returns logs for a specific model
func (l *SQLiteLogger) GetLogsByModel(model string) []AuditLog {
	stmt := `SELECT id, timestamp, event_type, level, user, model, method, endpoint,
		request_size, response_size, tokens_used, tokens_generated,
		duration_ms, status_code, error_message, ip_address, user_agent, correlation_id
		FROM audit_logs WHERE model = ? ORDER BY timestamp DESC`

	rows, err := l.db.Query(stmt, model)
	if err != nil {
		return nil
	}
	defer rows.Close()

	return scanLogs(rows, l.key)
}

// GetLogsInTimeRange returns logs within a time range
func (l *SQLiteLogger) GetLogsInTimeRange(start, end time.Time) []AuditLog {
	stmt := `SELECT id, timestamp, event_type, level, user, model, method, endpoint,
		request_size, response_size, tokens_used, tokens_generated,
		duration_ms, status_code, error_message, ip_address, user_agent, correlation_id
		FROM audit_logs WHERE timestamp BETWEEN ? AND ? ORDER BY timestamp DESC`

	rows, err := l.db.Query(stmt, start, end)
	if err != nil {
		return nil
	}
	defer rows.Close()

	return scanLogs(rows, l.key)
}

// GetStats returns aggregate statistics
func (l *SQLiteLogger) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// Total logs
	var totalLogs int
	l.db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&totalLogs)
	stats["total_logs"] = totalLogs

	// Total requests
	var totalRequests int
	l.db.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE event_type = 'request'").Scan(&totalRequests)
	stats["total_requests"] = totalRequests

	// Total errors
	var totalErrors int
	l.db.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE event_type = 'error'").Scan(&totalErrors)
	stats["total_errors"] = totalErrors

	// Token usage
	var totalTokensUsed, totalTokensGenerated int64
	l.db.QueryRow("SELECT COALESCE(SUM(tokens_used), 0) FROM audit_logs WHERE event_type = 'response'").Scan(&totalTokensUsed)
	l.db.QueryRow("SELECT COALESCE(SUM(tokens_generated), 0) FROM audit_logs WHERE event_type = 'response'").Scan(&totalTokensGenerated)
	stats["total_tokens_used"] = totalTokensUsed
	stats["total_tokens_generated"] = totalTokensGenerated

	// Unique users and models
	var uniqueUsers, uniqueModels int
	l.db.QueryRow("SELECT COUNT(DISTINCT user) FROM audit_logs WHERE user IS NOT NULL").Scan(&uniqueUsers)
	l.db.QueryRow("SELECT COUNT(DISTINCT model) FROM audit_logs WHERE model IS NOT NULL").Scan(&uniqueModels)
	stats["unique_users"] = uniqueUsers
	stats["unique_models"] = uniqueModels

	return stats
}

// Helper functions

func createSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS config (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS audit_logs (
		id               TEXT PRIMARY KEY,
		timestamp        DATETIME NOT NULL,
		event_type       TEXT NOT NULL,
		level            TEXT NOT NULL,
		user             TEXT,
		model            TEXT,
		method           TEXT,
		endpoint         TEXT,
		request_size     INTEGER,
		response_size    INTEGER,
		tokens_used      INTEGER,
		tokens_generated INTEGER,
		duration_ms      INTEGER,
		status_code      INTEGER,
		error_message    TEXT,
		ip_address       TEXT,
		user_agent       TEXT,
		correlation_id   TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_user      ON audit_logs(user);
	CREATE INDEX IF NOT EXISTS idx_model     ON audit_logs(model);
	CREATE INDEX IF NOT EXISTS idx_timestamp ON audit_logs(timestamp);
	`

	_, err := db.Exec(schema)
	return err
}

func deriveKey(db *sql.DB, encryptionKey string) ([]byte, error) {
	// Check if salt exists
	var salt string
	err := db.QueryRow("SELECT value FROM config WHERE key = 'pbkdf2_salt'").Scan(&salt)

	if err == sql.ErrNoRows {
		// Generate new salt
		saltBytes := make([]byte, 16)
		if _, err := io.ReadFull(rand.Reader, saltBytes); err != nil {
			return nil, fmt.Errorf("failed to generate salt: %w", err)
		}
		salt = hex.EncodeToString(saltBytes)

		// Store salt
		_, err = db.Exec("INSERT OR REPLACE INTO config (key, value) VALUES (?, ?)",
			"pbkdf2_salt", salt)
		if err != nil {
			return nil, fmt.Errorf("failed to store salt: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to read salt: %w", err)
	}

	// Decode salt
	saltBytes, err := hex.DecodeString(salt)
	if err != nil {
		return nil, fmt.Errorf("failed to decode salt: %w", err)
	}

	// Derive key using PBKDF2
	key := pbkdf2.Key([]byte(encryptionKey), saltBytes, 100000, 32, sha256.New)
	return key, nil
}

func encrypt(key []byte, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func decrypt(key []byte, ciphertextStr string) string {
	if ciphertextStr == "" {
		return ""
	}

	ciphertextBytes, err := base64.StdEncoding.DecodeString(ciphertextStr)
	if err != nil {
		return ""
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return ""
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return ""
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertextBytes) < nonceSize {
		return ""
	}

	nonce, ciphertext := ciphertextBytes[:nonceSize], ciphertextBytes[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return ""
	}

	return string(plaintext)
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func scanLogs(rows *sql.Rows, key []byte) []AuditLog {
	var logs []AuditLog

	for rows.Next() {
		var log AuditLog
		var encIPAddr, encUserAgent, encErrMsg string
		var id string

		err := rows.Scan(
			&id, // id (scanned but not used)
			&log.Timestamp, &log.EventType, &log.Level, &log.User, &log.Model,
			&log.Method, &log.Endpoint, &log.RequestSize, &log.ResponseSize,
			&log.TokensUsed, &log.TokensGenerated, &log.DurationMS, &log.StatusCode,
			&encErrMsg, &encIPAddr, &encUserAgent, &log.CorrelationID,
		)
		if err != nil {
			continue
		}

		// Decrypt sensitive fields
		log.ErrorMessage = decrypt(key, encErrMsg)
		log.IPAddress = decrypt(key, encIPAddr)
		log.UserAgent = decrypt(key, encUserAgent)

		logs = append(logs, log)
	}

	return logs
}
