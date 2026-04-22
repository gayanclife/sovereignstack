package audit

import (
	"fmt"
	"sync"
	"time"
)

// AuditLevel represents the severity level of an audit event
type AuditLevel string

const (
	AuditLevelInfo  AuditLevel = "info"
	AuditLevelWarn  AuditLevel = "warn"
	AuditLevelError AuditLevel = "error"
)

// AuditLog represents a single audit log entry
type AuditLog struct {
	Timestamp       time.Time  `json:"timestamp"`
	EventType       string     `json:"event_type"` // "request", "response", "auth", "error"
	Level           AuditLevel `json:"level"`
	User            string     `json:"user"`          // API key or user identifier
	Model           string     `json:"model"`         // Model name
	Method          string     `json:"method"`        // HTTP method (GET, POST, etc.)
	Endpoint        string     `json:"endpoint"`      // API endpoint called
	RequestSize     int64      `json:"request_size"`  // Bytes
	ResponseSize    int64      `json:"response_size"` // Bytes
	TokensUsed      int        `json:"tokens_used"`
	TokensGenerated int        `json:"tokens_generated"`
	DurationMS      int64      `json:"duration_ms"`
	StatusCode      int        `json:"status_code"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	IPAddress       string     `json:"ip_address"`
	UserAgent       string     `json:"user_agent"`
	CorrelationID   string     `json:"correlation_id"` // Trace ID for request linking
}

// Logger manages audit log collection and storage
type Logger struct {
	logs    []AuditLog
	mu      sync.RWMutex
	maxLogs int
}

// NewLogger creates a new audit logger
func NewLogger(maxLogs int) *Logger {
	return &Logger{
		logs:    make([]AuditLog, 0, maxLogs),
		maxLogs: maxLogs,
	}
}

// Log adds a new audit entry
func (l *Logger) Log(entry AuditLog) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Set timestamp if not already set
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	l.logs = append(l.logs, entry)

	// Keep only the last maxLogs entries in memory
	if len(l.logs) > l.maxLogs {
		l.logs = l.logs[len(l.logs)-l.maxLogs:]
	}
}

// LogRequest logs an incoming API request
func (l *Logger) LogRequest(user, model, endpoint, method, ipAddress, userAgent, correlationID string, requestSize int64) {
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
func (l *Logger) LogResponse(user, model, endpoint, correlationID string, statusCode int, responseSize int64, tokensUsed, tokensGenerated int, durationMS int64) {
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
func (l *Logger) LogError(user, endpoint, correlationID, errMsg string, statusCode int, ipAddress string) {
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
func (l *Logger) LogAuthFailure(user, endpoint, ipAddress, reason string) {
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

// GetLogs returns the last n audit logs
func (l *Logger) GetLogs(n int) []AuditLog {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if n <= 0 || n > len(l.logs) {
		n = len(l.logs)
	}

	// Return the last n logs
	start := len(l.logs) - n
	result := make([]AuditLog, n)
	copy(result, l.logs[start:])
	return result
}

// GetLogsByUser returns all logs for a specific user
func (l *Logger) GetLogsByUser(user string) []AuditLog {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []AuditLog
	for _, log := range l.logs {
		if log.User == user {
			result = append(result, log)
		}
	}
	return result
}

// GetLogsByModel returns all logs for a specific model
func (l *Logger) GetLogsByModel(model string) []AuditLog {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []AuditLog
	for _, log := range l.logs {
		if log.Model == model {
			result = append(result, log)
		}
	}
	return result
}

// GetLogsInTimeRange returns logs within a time range
func (l *Logger) GetLogsInTimeRange(start, end time.Time) []AuditLog {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []AuditLog
	for _, log := range l.logs {
		if log.Timestamp.After(start) && log.Timestamp.Before(end) {
			result = append(result, log)
		}
	}
	return result
}

// ClearLogs clears all audit logs (use with caution)
func (l *Logger) ClearLogs() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = make([]AuditLog, 0, l.maxLogs)
}

// GetStats returns aggregate statistics from the logs
func (l *Logger) GetStats() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	totalRequests := 0
	totalErrors := 0
	totalTokensUsed := int64(0)
	totalTokensGenerated := int64(0)
	totalDuration := int64(0)
	userCounts := make(map[string]int)
	modelCounts := make(map[string]int)

	for _, log := range l.logs {
		switch log.EventType {
		case "request":
			totalRequests++
		case "error":
			totalErrors++
		case "response":
			totalTokensUsed += int64(log.TokensUsed)
			totalTokensGenerated += int64(log.TokensGenerated)
			totalDuration += log.DurationMS
		}

		if log.User != "" {
			userCounts[log.User]++
		}
		if log.Model != "" {
			modelCounts[log.Model]++
		}
	}

	return map[string]interface{}{
		"total_logs":                len(l.logs),
		"total_requests":            totalRequests,
		"total_errors":              totalErrors,
		"total_tokens_used":         totalTokensUsed,
		"total_tokens_generated":    totalTokensGenerated,
		"total_duration_ms":         totalDuration,
		"unique_users":              len(userCounts),
		"unique_models":             len(modelCounts),
		"user_request_distribution": userCounts,
		"model_usage_distribution":  modelCounts,
	}
}
