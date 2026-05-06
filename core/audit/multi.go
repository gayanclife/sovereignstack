// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package audit

import "time"

// MultiSinkLogger fans out every audit event to a list of underlying loggers.
// Every sink sees every event; failures in one sink do not prevent the others
// from receiving the record (the underlying loggers are responsible for
// their own error handling).
//
// Reads (GetLogs, GetStats) go to the first sink; sinks that store more
// data (e.g. SQLite or MySQL) should be listed first if you want richer
// query results from the gateway's debug endpoints.
type MultiSinkLogger struct {
	sinks []AuditLogger
}

// NewMultiSinkLogger returns a fan-out logger. At least one sink should be
// provided; with zero sinks the logger silently drops everything (useful
// for tests).
func NewMultiSinkLogger(sinks ...AuditLogger) *MultiSinkLogger {
	return &MultiSinkLogger{sinks: sinks}
}

func (m *MultiSinkLogger) Log(entry AuditLog) {
	for _, s := range m.sinks {
		s.Log(entry)
	}
}

func (m *MultiSinkLogger) LogRequest(user, model, endpoint, method, ipAddress, userAgent, correlationID string, requestSize int64) {
	for _, s := range m.sinks {
		s.LogRequest(user, model, endpoint, method, ipAddress, userAgent, correlationID, requestSize)
	}
}

func (m *MultiSinkLogger) LogResponse(user, model, endpoint, correlationID string, statusCode int, responseSize int64, tokensUsed, tokensGenerated int, durationMS int64) {
	for _, s := range m.sinks {
		s.LogResponse(user, model, endpoint, correlationID, statusCode, responseSize, tokensUsed, tokensGenerated, durationMS)
	}
}

func (m *MultiSinkLogger) LogError(user, endpoint, correlationID, errMsg string, statusCode int, ipAddress string) {
	for _, s := range m.sinks {
		s.LogError(user, endpoint, correlationID, errMsg, statusCode, ipAddress)
	}
}

func (m *MultiSinkLogger) LogAuthFailure(user, endpoint, ipAddress, reason string) {
	for _, s := range m.sinks {
		s.LogAuthFailure(user, endpoint, ipAddress, reason)
	}
}

// GetLogs / GetStats / GetLogsBy* delegate to the first sink only.
// If no sinks are registered, returns empty results.
func (m *MultiSinkLogger) GetLogs(n int) []AuditLog {
	if len(m.sinks) == 0 {
		return nil
	}
	return m.sinks[0].GetLogs(n)
}

func (m *MultiSinkLogger) GetLogsByUser(user string) []AuditLog {
	if len(m.sinks) == 0 {
		return nil
	}
	return m.sinks[0].GetLogsByUser(user)
}

func (m *MultiSinkLogger) GetLogsByModel(model string) []AuditLog {
	if len(m.sinks) == 0 {
		return nil
	}
	return m.sinks[0].GetLogsByModel(model)
}

func (m *MultiSinkLogger) GetLogsInTimeRange(start, end time.Time) []AuditLog {
	if len(m.sinks) == 0 {
		return nil
	}
	return m.sinks[0].GetLogsInTimeRange(start, end)
}

func (m *MultiSinkLogger) GetStats() map[string]interface{} {
	if len(m.sinks) == 0 {
		return map[string]interface{}{}
	}
	return m.sinks[0].GetStats()
}

// Sinks returns the registered sinks. Useful for shutdown sequencing if
// any of them need explicit Close() calls.
func (m *MultiSinkLogger) Sinks() []AuditLogger { return m.sinks }
