// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package gateway

import (
	"fmt"
	"time"

	"github.com/gayanclife/sovereignstack/core/keys"
)

// TokenQuotaManager tracks and enforces per-user token quotas (daily and monthly)
// against a pluggable QuotaBackend. The manager itself is stateless — all
// counter state lives in the backend so swapping memory/sqlite/redis is a
// configuration change with no code touch.
type TokenQuotaManager struct {
	store   *keys.KeyStore
	backend QuotaBackend
}

// QuotaUsage represents current quota status for a user.
type QuotaUsage struct {
	UserID         string
	DailyUsed      int64
	DailyLimit     int64
	DailyResetAt   time.Time
	MonthlyUsed    int64
	MonthlyLimit   int64
	MonthlyResetAt time.Time
	DailyPercent   float64 // 0-100
	MonthlyPercent float64 // 0-100
}

// NewTokenQuotaManager creates a manager that reads limits from the KeyStore
// and tracks usage via the in-memory backend. Equivalent to
// NewTokenQuotaManagerWithBackend(store, NewMemoryQuotaBackend()).
//
// Use NewTokenQuotaManagerWithBackend for SQLite or Redis persistence.
func NewTokenQuotaManager(store *keys.KeyStore) *TokenQuotaManager {
	return NewTokenQuotaManagerWithBackend(store, NewMemoryQuotaBackend())
}

// NewTokenQuotaManagerWithBackend creates a manager backed by the given
// QuotaBackend implementation. The manager does not take ownership of the
// backend's lifecycle — call backend.Close() yourself at shutdown.
func NewTokenQuotaManagerWithBackend(store *keys.KeyStore, backend QuotaBackend) *TokenQuotaManager {
	return &TokenQuotaManager{
		store:   store,
		backend: backend,
	}
}

// CheckQuota returns error if user has exceeded daily or monthly limit.
// Called BEFORE forwarding request to backend.
func (tq *TokenQuotaManager) CheckQuota(userID string) error {
	if userID == "" {
		return nil
	}

	profile, err := tq.store.GetByID(userID)
	if err != nil || profile == nil {
		return fmt.Errorf("user not found: %s", userID)
	}

	// Check daily limit
	if profile.MaxTokensPerDay > 0 {
		dailyUsed, err := tq.backend.GetDaily(userID)
		if err != nil {
			return fmt.Errorf("read daily quota: %w", err)
		}
		if dailyUsed >= profile.MaxTokensPerDay {
			return fmt.Errorf("daily token quota exceeded: %d/%d", dailyUsed, profile.MaxTokensPerDay)
		}
	}

	// Check monthly limit
	if profile.MaxTokensPerMonth > 0 {
		monthlyUsed, err := tq.backend.GetMonthly(userID)
		if err != nil {
			return fmt.Errorf("read monthly quota: %w", err)
		}
		if monthlyUsed >= profile.MaxTokensPerMonth {
			return fmt.Errorf("monthly token quota exceeded: %d/%d", monthlyUsed, profile.MaxTokensPerMonth)
		}
	}

	return nil
}

// Record adds tokens to user's quota counters after a completed request.
// Called AFTER response received from backend.
//
// Errors from the backend are not currently surfaced to the caller — the
// gateway request has already succeeded by this point, and aborting on
// quota-write failure would be worse than slight under-counting. Backends
// log their own errors.
func (tq *TokenQuotaManager) Record(userID string, inputTokens, outputTokens int64) {
	if userID == "" {
		return
	}

	totalTokens := inputTokens + outputTokens
	_ = tq.backend.AddDaily(userID, totalTokens)
	_ = tq.backend.AddMonthly(userID, totalTokens)
}

// GetUsage returns current quota status for a user.
func (tq *TokenQuotaManager) GetUsage(userID string) (*QuotaUsage, error) {
	profile, err := tq.store.GetByID(userID)
	if err != nil || profile == nil {
		return nil, fmt.Errorf("user not found: %s", userID)
	}

	dailyUsed, err := tq.backend.GetDaily(userID)
	if err != nil {
		return nil, fmt.Errorf("read daily quota: %w", err)
	}
	monthlyUsed, err := tq.backend.GetMonthly(userID)
	if err != nil {
		return nil, fmt.Errorf("read monthly quota: %w", err)
	}

	usage := &QuotaUsage{
		UserID:         userID,
		DailyUsed:      dailyUsed,
		DailyLimit:     profile.MaxTokensPerDay,
		MonthlyUsed:    monthlyUsed,
		MonthlyLimit:   profile.MaxTokensPerMonth,
		DailyResetAt:   nextDailyReset(time.Now()),
		MonthlyResetAt: nextMonthlyReset(time.Now()),
	}

	// Calculate percentages
	if profile.MaxTokensPerDay > 0 {
		usage.DailyPercent = float64(dailyUsed) / float64(profile.MaxTokensPerDay) * 100
	}
	if profile.MaxTokensPerMonth > 0 {
		usage.MonthlyPercent = float64(monthlyUsed) / float64(profile.MaxTokensPerMonth) * 100
	}

	return usage, nil
}

// Backend exposes the underlying backend for shutdown / inspection.
func (tq *TokenQuotaManager) Backend() QuotaBackend { return tq.backend }
