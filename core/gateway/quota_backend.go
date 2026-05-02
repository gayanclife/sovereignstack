// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package gateway

import (
	"sync"
	"time"
)

// QuotaBackend stores per-user token usage counters in time-bucketed windows
// (daily UTC, monthly UTC). Implementations may be in-memory, on-disk, or
// distributed; the gateway treats them all interchangeably.
//
// Reads (Get) are eventually consistent across processes — for hard caps in
// multi-instance deployments, use a Redis or other shared backend rather
// than the per-process Memory implementation.
type QuotaBackend interface {
	// AddDaily atomically adds tokens to userID's "current UTC day" counter.
	// Counters reset at the next UTC midnight.
	AddDaily(userID string, tokens int64) error

	// AddMonthly atomically adds tokens to userID's "current UTC month"
	// counter. Counters reset on the 1st of the next month at UTC midnight.
	AddMonthly(userID string, tokens int64) error

	// GetDaily returns tokens used today by userID (or 0 if no record).
	GetDaily(userID string) (int64, error)

	// GetMonthly returns tokens used this month by userID (or 0 if no record).
	GetMonthly(userID string) (int64, error)

	// Close releases resources (DB handles, network connections, etc.).
	Close() error
}

// nextDailyReset returns the next UTC midnight.
func nextDailyReset(now time.Time) time.Time {
	now = now.UTC()
	tomorrow := now.AddDate(0, 0, 1)
	return time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, time.UTC)
}

// nextMonthlyReset returns the 1st of the next month at UTC midnight.
func nextMonthlyReset(now time.Time) time.Time {
	now = now.UTC()
	nextMonth := now.AddDate(0, 1, 0)
	return time.Date(nextMonth.Year(), nextMonth.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// dayKey returns "YYYY-MM-DD" for the given time, in UTC.
// Used by persistent backends to bucket counters by day.
func dayKey(t time.Time) string {
	t = t.UTC()
	return t.Format("2006-01-02")
}

// monthKey returns "YYYY-MM" for the given time, in UTC.
func monthKey(t time.Time) string {
	t = t.UTC()
	return t.Format("2006-01")
}

// ─── In-memory backend ──────────────────────────────────────────────────────

// MemoryQuotaBackend is the default backend: a thread-safe in-process map.
// State is lost on restart — only suitable for development or single-instance
// gateways where quota loss on restart is acceptable.
type MemoryQuotaBackend struct {
	mu      sync.RWMutex
	daily   map[string]*memoryCounter
	monthly map[string]*memoryCounter
}

type memoryCounter struct {
	used    int64
	resetAt time.Time
	mu      sync.Mutex
}

// NewMemoryQuotaBackend returns a new in-memory backend.
func NewMemoryQuotaBackend() *MemoryQuotaBackend {
	return &MemoryQuotaBackend{
		daily:   make(map[string]*memoryCounter),
		monthly: make(map[string]*memoryCounter),
	}
}

func (b *MemoryQuotaBackend) addTo(m map[string]*memoryCounter, userID string, tokens int64, resetFn func(time.Time) time.Time) error {
	b.mu.Lock()
	counter, ok := m[userID]
	if !ok {
		counter = &memoryCounter{resetAt: resetFn(time.Now())}
		m[userID] = counter
	}
	b.mu.Unlock()

	counter.mu.Lock()
	defer counter.mu.Unlock()
	if time.Now().After(counter.resetAt) {
		counter.used = 0
		counter.resetAt = resetFn(time.Now())
	}
	counter.used += tokens
	return nil
}

func (b *MemoryQuotaBackend) getFrom(m map[string]*memoryCounter, userID string) (int64, error) {
	b.mu.RLock()
	counter, ok := m[userID]
	b.mu.RUnlock()
	if !ok {
		return 0, nil
	}
	counter.mu.Lock()
	defer counter.mu.Unlock()
	if time.Now().After(counter.resetAt) {
		return 0, nil
	}
	return counter.used, nil
}

func (b *MemoryQuotaBackend) AddDaily(userID string, tokens int64) error {
	return b.addTo(b.daily, userID, tokens, nextDailyReset)
}

func (b *MemoryQuotaBackend) AddMonthly(userID string, tokens int64) error {
	return b.addTo(b.monthly, userID, tokens, nextMonthlyReset)
}

func (b *MemoryQuotaBackend) GetDaily(userID string) (int64, error) {
	return b.getFrom(b.daily, userID)
}

func (b *MemoryQuotaBackend) GetMonthly(userID string) (int64, error) {
	return b.getFrom(b.monthly, userID)
}

func (b *MemoryQuotaBackend) Close() error { return nil }
