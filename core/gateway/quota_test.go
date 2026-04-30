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
	"testing"
	"time"

	"github.com/gayanclife/sovereignstack/core/keys"
)

func TestTokenQuotaManager_CheckQuota_Allowed(t *testing.T) {
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}
	alice := &keys.UserProfile{
		ID:                 "alice",
		Key:                "sk_alice_123",
		MaxTokensPerDay:    1000,
		MaxTokensPerMonth:  10000,
		CreatedAt:          time.Now(),
	}
	ks.Users["alice"] = alice

	tq := NewTokenQuotaManager(ks)

	// Fresh user should be allowed
	err := tq.CheckQuota("alice")
	if err != nil {
		t.Errorf("Fresh user should be allowed, got error: %v", err)
	}
}

func TestTokenQuotaManager_CheckQuota_DailyExceeded(t *testing.T) {
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}
	alice := &keys.UserProfile{
		ID:                 "alice",
		Key:                "sk_alice_123",
		MaxTokensPerDay:    100,
		MaxTokensPerMonth:  10000,
		CreatedAt:          time.Now(),
	}
	ks.Users["alice"] = alice

	tq := NewTokenQuotaManager(ks)

	// Record 100 tokens (hits daily limit)
	tq.Record("alice", 50, 50)

	// Should be rejected on next check
	err := tq.CheckQuota("alice")
	if err == nil {
		t.Errorf("Should reject when daily quota exceeded")
	}
}

func TestTokenQuotaManager_CheckQuota_MonthlyExceeded(t *testing.T) {
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}
	bob := &keys.UserProfile{
		ID:                 "bob",
		Key:                "sk_bob_123",
		MaxTokensPerDay:    100000,
		MaxTokensPerMonth:  200,
		CreatedAt:          time.Now(),
	}
	ks.Users["bob"] = bob

	tq := NewTokenQuotaManager(ks)

	// Record 200 tokens (hits monthly limit)
	tq.Record("bob", 100, 100)

	// Should be rejected on next check
	err := tq.CheckQuota("bob")
	if err == nil {
		t.Errorf("Should reject when monthly quota exceeded")
	}
}

func TestTokenQuotaManager_CheckQuota_Unlimited(t *testing.T) {
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}
	admin := &keys.UserProfile{
		ID:                 "admin",
		Key:                "sk_admin_123",
		MaxTokensPerDay:    0, // unlimited
		MaxTokensPerMonth:  0, // unlimited
		CreatedAt:          time.Now(),
	}
	ks.Users["admin"] = admin

	tq := NewTokenQuotaManager(ks)

	// Record large amounts
	tq.Record("admin", 1000000, 1000000)

	// Should still be allowed
	err := tq.CheckQuota("admin")
	if err != nil {
		t.Errorf("Unlimited user should always be allowed, got error: %v", err)
	}
}

func TestTokenQuotaManager_Record_DailyTracking(t *testing.T) {
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}
	alice := &keys.UserProfile{
		ID:                 "alice",
		Key:                "sk_alice_123",
		MaxTokensPerDay:    10000,
		MaxTokensPerMonth:  100000,
		CreatedAt:          time.Now(),
	}
	ks.Users["alice"] = alice

	tq := NewTokenQuotaManager(ks)

	// Record some tokens
	tq.Record("alice", 100, 200)
	tq.Record("alice", 50, 150)

	usage, _ := tq.GetUsage("alice")
	expected := int64(500) // 100+200+50+150
	if usage.DailyUsed != expected {
		t.Errorf("Expected %d daily tokens, got %d", expected, usage.DailyUsed)
	}
}

func TestTokenQuotaManager_GetUsage_Percentages(t *testing.T) {
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}
	alice := &keys.UserProfile{
		ID:                 "alice",
		Key:                "sk_alice_123",
		MaxTokensPerDay:    1000,
		MaxTokensPerMonth:  10000,
		CreatedAt:          time.Now(),
	}
	ks.Users["alice"] = alice

	tq := NewTokenQuotaManager(ks)

	// Record 500 tokens (50% of daily limit)
	tq.Record("alice", 250, 250)

	usage, _ := tq.GetUsage("alice")

	// Check daily percentage: 500 / 1000 = 50%
	if usage.DailyPercent < 49.0 || usage.DailyPercent > 51.0 {
		t.Errorf("Expected ~50%% daily usage, got %.1f%%", usage.DailyPercent)
	}
}

func TestTokenQuotaManager_GetUsage_ResetAtTimes(t *testing.T) {
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}
	alice := &keys.UserProfile{
		ID:                 "alice",
		Key:                "sk_alice_123",
		MaxTokensPerDay:    1000,
		MaxTokensPerMonth:  10000,
		CreatedAt:          time.Now(),
	}
	ks.Users["alice"] = alice

	tq := NewTokenQuotaManager(ks)
	tq.Record("alice", 100, 100)

	usage, _ := tq.GetUsage("alice")

	// Daily reset should be tomorrow at UTC midnight
	now := time.Now().UTC()
	tomorrow := now.AddDate(0, 0, 1)
	expectedDaily := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, time.UTC)

	// Allow 1 second tolerance
	if usage.DailyResetAt.Sub(expectedDaily).Abs() > time.Second {
		t.Errorf("Daily reset time incorrect. Expected ~%v, got %v", expectedDaily, usage.DailyResetAt)
	}

	// Monthly reset should be 1st of next month at UTC midnight
	nextMonth := now.AddDate(0, 1, 0)
	expectedMonthly := time.Date(nextMonth.Year(), nextMonth.Month(), 1, 0, 0, 0, 0, time.UTC)

	if usage.MonthlyResetAt.Sub(expectedMonthly).Abs() > time.Second {
		t.Errorf("Monthly reset time incorrect. Expected ~%v, got %v", expectedMonthly, usage.MonthlyResetAt)
	}
}

func TestTokenQuotaManager_GetUsage_UnknownUser(t *testing.T) {
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}
	tq := NewTokenQuotaManager(ks)

	_, err := tq.GetUsage("unknown")
	if err == nil {
		t.Errorf("Should return error for unknown user")
	}
}

func TestTokenQuotaManager_Concurrency(t *testing.T) {
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}
	alice := &keys.UserProfile{
		ID:                 "alice",
		Key:                "sk_alice_123",
		MaxTokensPerDay:    100000,
		MaxTokensPerMonth:  1000000,
		CreatedAt:          time.Now(),
	}
	ks.Users["alice"] = alice

	tq := NewTokenQuotaManager(ks)

	// Simulate concurrent requests recording tokens
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				tq.Record("alice", 10, 10)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	usage, _ := tq.GetUsage("alice")
	// 10 goroutines * 100 iterations * 20 tokens = 20000
	expected := int64(20000)
	if usage.DailyUsed != expected {
		t.Errorf("Expected %d tokens from concurrent recording, got %d", expected, usage.DailyUsed)
	}
}

func TestTokenQuotaManager_EmptyAllowanceZero(t *testing.T) {
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}
	bob := &keys.UserProfile{
		ID:                 "bob",
		Key:                "sk_bob_123",
		MaxTokensPerDay:    0,
		MaxTokensPerMonth:  0,
		CreatedAt:          time.Now(),
	}
	ks.Users["bob"] = bob

	tq := NewTokenQuotaManager(ks)

	// User with 0 limits (unlimited) should be allowed
	err := tq.CheckQuota("bob")
	if err != nil {
		t.Errorf("User with 0 quota limits (unlimited) should be allowed, got error: %v", err)
	}
}

func TestTokenQuotaManager_PartialTokenCount(t *testing.T) {
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}
	alice := &keys.UserProfile{
		ID:                 "alice",
		Key:                "sk_alice_123",
		MaxTokensPerDay:    1000,
		MaxTokensPerMonth:  10000,
		CreatedAt:          time.Now(),
	}
	ks.Users["alice"] = alice

	tq := NewTokenQuotaManager(ks)

	// Record with separate input and output tokens
	tq.Record("alice", 300, 400)
	tq.Record("alice", 50, 75)

	usage, _ := tq.GetUsage("alice")
	expected := int64(825) // 300+400+50+75
	if usage.DailyUsed != expected {
		t.Errorf("Expected %d tokens, got %d", expected, usage.DailyUsed)
	}
}

// Benchmark quota checking
func BenchmarkTokenQuotaManager_CheckQuota(b *testing.B) {
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}
	alice := &keys.UserProfile{
		ID:                 "alice",
		Key:                "sk_alice_123",
		MaxTokensPerDay:    1000000,
		MaxTokensPerMonth:  10000000,
		CreatedAt:          time.Now(),
	}
	ks.Users["alice"] = alice

	tq := NewTokenQuotaManager(ks)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tq.CheckQuota("alice")
	}
}

// Benchmark token recording
func BenchmarkTokenQuotaManager_Record(b *testing.B) {
	ks := &keys.KeyStore{Users: make(map[string]*keys.UserProfile)}
	alice := &keys.UserProfile{
		ID:                 "alice",
		Key:                "sk_alice_123",
		MaxTokensPerDay:    1000000,
		MaxTokensPerMonth:  10000000,
		CreatedAt:          time.Now(),
	}
	ks.Users["alice"] = alice

	tq := NewTokenQuotaManager(ks)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tq.Record("alice", 100, 100)
	}
}
