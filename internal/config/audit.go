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
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"time"
)

type AuditEvent struct {
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"`
	User      string `json:"user"`
	Details   string `json:"details"`
	Status    string `json:"status"`
}

type AuditLogger struct {
	logFile string
	logDir  string
}

// NewAuditLogger creates a new audit logger with automatic directory creation
func NewAuditLogger(logDir string) *AuditLogger {
	// Ensure log directory exists
	_ = os.MkdirAll(logDir, 0755)

	return &AuditLogger{
		logDir:  logDir,
		logFile: filepath.Join(logDir, "audit.log"),
	}
}

// LogModelDownload logs a model download attempt
func (al *AuditLogger) LogModelDownload(modelID string, status string, details string) error {
	event := AuditEvent{
		Timestamp: time.Now().Format(time.RFC3339),
		Action:    "model_download",
		User:      getCurrentUser(),
		Details:   fmt.Sprintf("model=%s %s", modelID, details),
		Status:    status,
	}
	return al.log(event)
}

// LogConfigChange logs config changes
func (al *AuditLogger) LogConfigChange(key string, status string) error {
	event := AuditEvent{
		Timestamp: time.Now().Format(time.RFC3339),
		Action:    "config_change",
		User:      getCurrentUser(),
		Details:   fmt.Sprintf("key=%s", key),
		Status:    status,
	}
	return al.log(event)
}

// LogTokenAccess logs when credentials are accessed
func (al *AuditLogger) LogTokenAccess(source string) error {
	event := AuditEvent{
		Timestamp: time.Now().Format(time.RFC3339),
		Action:    "token_access",
		User:      getCurrentUser(),
		Details:   fmt.Sprintf("source=%s", source),
		Status:    "accessed",
	}
	return al.log(event)
}

// log appends an event to the audit log
func (al *AuditLogger) log(event AuditEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(al.logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(string(data) + "\n")
	return err
}

func getCurrentUser() string {
	u, err := user.Current()
	if err != nil {
		return "unknown"
	}
	return u.Username
}
