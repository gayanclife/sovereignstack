// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gateway

import (
	"fmt"
	"sync"
	"time"

	"github.com/gayanclife/sovereignstack/core/keys"
)

// TokenQuotaManager tracks and enforces per-user token quotas (daily and monthly).
type TokenQuotaManager struct {
	store  *keys.KeyStore
	daily  map[string]*quotaCounter   // Reset at UTC midnight
	monthly map[string]*quotaCounter  // Reset on 1st of month
	mu     sync.RWMutex
}

// quotaCounter tracks token usage within a time window
type quotaCounter struct {
	used    int64     // Tokens used in current window
	resetAt time.Time // When this window resets
	mu      sync.Mutex
}

// QuotaUsage represents current quota status for a user
type QuotaUsage struct {
	UserID           string
	DailyUsed        int64
	DailyLimit       int64
	DailyResetAt     time.Time
	MonthlyUsed      int64
	MonthlyLimit     int64
	MonthlyResetAt   time.Time
	DailyPercent     float64  // 0-100
	MonthlyPercent   float64  // 0-100
}

// NewTokenQuotaManager creates a new quota manager backed by a KeyStore.
func NewTokenQuotaManager(store *keys.KeyStore) *TokenQuotaManager {
	return &TokenQuotaManager{
		store:  store,
		daily:  make(map[string]*quotaCounter),
		monthly: make(map[string]*quotaCounter),
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
		dailyUsed := tq.getDailyUsed(userID)
		if dailyUsed >= profile.MaxTokensPerDay {
			return fmt.Errorf("daily token quota exceeded: %d/%d", dailyUsed, profile.MaxTokensPerDay)
		}
	}

	// Check monthly limit
	if profile.MaxTokensPerMonth > 0 {
		monthlyUsed := tq.getMonthlyUsed(userID)
		if monthlyUsed >= profile.MaxTokensPerMonth {
			return fmt.Errorf("monthly token quota exceeded: %d/%d", monthlyUsed, profile.MaxTokensPerMonth)
		}
	}

	return nil
}

// Record adds tokens to user's quota counters after a completed request.
// Called AFTER response received from backend.
func (tq *TokenQuotaManager) Record(userID string, inputTokens, outputTokens int64) {
	if userID == "" {
		return
	}

	totalTokens := inputTokens + outputTokens

	// Record daily
	tq.recordDaily(userID, totalTokens)

	// Record monthly
	tq.recordMonthly(userID, totalTokens)
}

// GetUsage returns current quota status for a user.
func (tq *TokenQuotaManager) GetUsage(userID string) (*QuotaUsage, error) {
	profile, err := tq.store.GetByID(userID)
	if err != nil || profile == nil {
		return nil, fmt.Errorf("user not found: %s", userID)
	}

	dailyUsed := tq.getDailyUsed(userID)
	monthlyUsed := tq.getMonthlyUsed(userID)

	usage := &QuotaUsage{
		UserID:         userID,
		DailyUsed:      dailyUsed,
		DailyLimit:     profile.MaxTokensPerDay,
		MonthlyUsed:    monthlyUsed,
		MonthlyLimit:   profile.MaxTokensPerMonth,
		DailyResetAt:   tq.nextDailyReset(),
		MonthlyResetAt: tq.nextMonthlyReset(),
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

// getDailyUsed returns tokens used today (since last UTC midnight).
func (tq *TokenQuotaManager) getDailyUsed(userID string) int64 {
	tq.mu.RLock()
	counter, exists := tq.daily[userID]
	tq.mu.RUnlock()

	if !exists {
		return 0
	}

	counter.mu.Lock()
	defer counter.mu.Unlock()

	// Check if counter expired (new day)
	if time.Now().After(counter.resetAt) {
		return 0
	}

	return counter.used
}

// getMonthlyUsed returns tokens used this month (since 1st of month UTC).
func (tq *TokenQuotaManager) getMonthlyUsed(userID string) int64 {
	tq.mu.RLock()
	counter, exists := tq.monthly[userID]
	tq.mu.RUnlock()

	if !exists {
		return 0
	}

	counter.mu.Lock()
	defer counter.mu.Unlock()

	// Check if counter expired (new month)
	if time.Now().After(counter.resetAt) {
		return 0
	}

	return counter.used
}

// recordDaily adds tokens to today's quota counter.
func (tq *TokenQuotaManager) recordDaily(userID string, tokens int64) {
	tq.mu.Lock()
	counter, exists := tq.daily[userID]
	if !exists {
		counter = &quotaCounter{
			resetAt: tq.nextDailyReset(),
		}
		tq.daily[userID] = counter
	}
	tq.mu.Unlock()

	counter.mu.Lock()
	defer counter.mu.Unlock()

	// Check if counter expired (new day)
	if time.Now().After(counter.resetAt) {
		counter.used = 0
		counter.resetAt = tq.nextDailyReset()
	}

	counter.used += tokens
}

// recordMonthly adds tokens to this month's quota counter.
func (tq *TokenQuotaManager) recordMonthly(userID string, tokens int64) {
	tq.mu.Lock()
	counter, exists := tq.monthly[userID]
	if !exists {
		counter = &quotaCounter{
			resetAt: tq.nextMonthlyReset(),
		}
		tq.monthly[userID] = counter
	}
	tq.mu.Unlock()

	counter.mu.Lock()
	defer counter.mu.Unlock()

	// Check if counter expired (new month)
	if time.Now().After(counter.resetAt) {
		counter.used = 0
		counter.resetAt = tq.nextMonthlyReset()
	}

	counter.used += tokens
}

// nextDailyReset returns the time of next UTC midnight.
func (tq *TokenQuotaManager) nextDailyReset() time.Time {
	now := time.Now().UTC()
	tomorrow := now.AddDate(0, 0, 1)
	return time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, time.UTC)
}

// nextMonthlyReset returns the time of next 1st of month at UTC midnight.
func (tq *TokenQuotaManager) nextMonthlyReset() time.Time {
	now := time.Now().UTC()
	nextMonth := now.AddDate(0, 1, 0)
	return time.Date(nextMonth.Year(), nextMonth.Month(), 1, 0, 0, 0, 0, time.UTC)
}
