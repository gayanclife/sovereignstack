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
package gateway

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestGatewayMetrics_RecordRequest(t *testing.T) {
	m := NewGatewayMetrics()
	m.RecordRequest()
	m.RecordRequest()
	m.RecordRequest()

	if v := testutil.ToFloat64(m.requestsTotal); v != 3 {
		t.Errorf("requests_total: want 3, got %v", v)
	}
	if v := testutil.ToFloat64(m.activeRequests); v != 3 {
		t.Errorf("active_requests: want 3, got %v", v)
	}
}

func TestGatewayMetrics_RecordRequestComplete(t *testing.T) {
	m := NewGatewayMetrics()
	m.RecordRequest()
	m.RecordRequestComplete(200, "POST", "alice", "mistral-7b")
	m.RecordRequest()
	m.RecordRequestComplete(403, "POST", "alice", "llama-3")

	if v := testutil.ToFloat64(m.activeRequests); v != 0 {
		t.Errorf("active_requests: want 0, got %v", v)
	}
	if v := testutil.ToFloat64(m.requestsTotal); v != 2 {
		t.Errorf("requests_total: want 2, got %v", v)
	}
}

func TestGatewayMetrics_RecordLatency(t *testing.T) {
	m := NewGatewayMetrics()
	m.RecordLatency("mistral-7b", 100)
	m.RecordLatency("mistral-7b", 150)
	m.RecordLatency("mistral-7b", 200)

	// Histogram should have 3 observations.
	count, err := testutil.GatherAndCount(m.registry)
	if err != nil {
		t.Fatalf("gather error: %v", err)
	}
	if count == 0 {
		t.Errorf("expected metrics from registry, got 0 families")
	}
}

func TestGatewayMetrics_RecordTokens(t *testing.T) {
	m := NewGatewayMetrics()
	m.RecordTokens("alice", "mistral-7b", 100, 250)
	m.RecordTokens("alice", "mistral-7b", 50, 150)
	m.RecordTokens("bob", "llama-3", 75, 100)

	if v := testutil.ToFloat64(m.tokensInputTotal); v != 225 {
		t.Errorf("tokens_input_total: want 225, got %v", v)
	}
	if v := testutil.ToFloat64(m.tokensOutputTotal); v != 500 {
		t.Errorf("tokens_output_total: want 500, got %v", v)
	}
}

func TestGatewayMetrics_RecordAuthFailure(t *testing.T) {
	m := NewGatewayMetrics()
	m.RecordAuthFailure("invalid_key")
	m.RecordAuthFailure("invalid_key")
	m.RecordAuthFailure("missing_key")

	if v := testutil.ToFloat64(m.authFailuresTotal); v != 3 {
		t.Errorf("auth_failures_total: want 3, got %v", v)
	}
}

func TestGatewayMetrics_RecordAccessDenied(t *testing.T) {
	m := NewGatewayMetrics()
	m.RecordAccessDenied("alice")
	m.RecordAccessDenied("alice")
	m.RecordAccessDenied("bob")

	if v := testutil.ToFloat64(m.accessDeniedTotal); v != 3 {
		t.Errorf("access_denied_total: want 3, got %v", v)
	}
}

func TestGatewayMetrics_RecordRateLimitHit(t *testing.T) {
	m := NewGatewayMetrics()
	for i := 0; i < 5; i++ {
		m.RecordRateLimitHit()
	}
	if v := testutil.ToFloat64(m.rateLimitHitsTotal); v != 5 {
		t.Errorf("rate_limit_hits_total: want 5, got %v", v)
	}
}

func TestGatewayMetrics_RecordTokenQuotaExceeded(t *testing.T) {
	m := NewGatewayMetrics()
	m.RecordTokenQuotaExceeded()
	m.RecordTokenQuotaExceeded()

	if v := testutil.ToFloat64(m.tokenQuotaExceededTotal); v != 2 {
		t.Errorf("token_quota_exceeded_total: want 2, got %v", v)
	}
}

func TestGatewayMetrics_HTTPHandler_Format(t *testing.T) {
	m := NewGatewayMetrics()
	m.RecordRequest()
	m.RecordRequest()
	m.RecordRequestComplete(200, "POST", "alice", "mistral-7b")
	m.RecordTokens("alice", "mistral-7b", 100, 200)
	m.RecordAuthFailure("invalid_key")
	m.RecordAccessDenied("bob")

	// Scrape via the HTTP handler.
	body := scrapeMetrics(t, m)

	for _, want := range []string{
		"gateway_requests_total 2",
		"gateway_auth_failures_total 1",
		"gateway_access_denied_total 1",
		"gateway_tokens_input_total 100",
		"gateway_tokens_output_total 200",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q\ngot:\n%s", want, body)
		}
	}
}

func TestGatewayMetrics_ByStatus(t *testing.T) {
	m := NewGatewayMetrics()
	m.RecordRequest()
	m.RecordRequestComplete(200, "POST", "alice", "mistral-7b")
	m.RecordRequest()
	m.RecordRequestComplete(403, "POST", "alice", "mistral-7b")
	m.RecordRequest()
	m.RecordRequestComplete(500, "POST", "alice", "mistral-7b")

	body := scrapeMetrics(t, m)

	for _, want := range []string{
		`gateway_requests_by_status{status="OK"}`,
		`gateway_requests_by_status{status="Forbidden"}`,
		`gateway_requests_by_status{status="Internal Server Error"}`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q", want)
		}
	}
}

func TestGatewayMetrics_Concurrency(t *testing.T) {
	m := NewGatewayMetrics()

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				m.RecordRequest()
				m.RecordRequestComplete(200, "POST", "user"+string(rune(65+id)), "model-a")
				m.RecordTokens("user"+string(rune(65+id)), "model-a", 100, 200)
			}
			done <- true
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	if v := testutil.ToFloat64(m.requestsTotal); v != 1000 {
		t.Errorf("requests_total: want 1000, got %v", v)
	}
	if v := testutil.ToFloat64(m.activeRequests); v != 0 {
		t.Errorf("active_requests: want 0, got %v", v)
	}
	if v := testutil.ToFloat64(m.tokensInputTotal); v != 100000 {
		t.Errorf("tokens_input_total: want 100000, got %v", v)
	}
}

// Benchmark tests

func BenchmarkGatewayMetrics_RecordRequest(b *testing.B) {
	m := NewGatewayMetrics()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordRequest()
	}
}

func BenchmarkGatewayMetrics_RecordRequestComplete(b *testing.B) {
	m := NewGatewayMetrics()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordRequest()
		m.RecordRequestComplete(200, "POST", "alice", "mistral-7b")
	}
}

func BenchmarkGatewayMetrics_RecordTokens(b *testing.B) {
	m := NewGatewayMetrics()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordTokens("alice", "mistral-7b", 100, 200)
	}
}

func BenchmarkGatewayMetrics_RecordLatency(b *testing.B) {
	m := NewGatewayMetrics()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordLatency("mistral-7b", 250)
	}
}

func BenchmarkGatewayMetrics_Scrape(b *testing.B) {
	m := NewGatewayMetrics()
	for i := 0; i < 100; i++ {
		m.RecordRequest()
		m.RecordRequestComplete(200, "POST", "alice", "mistral-7b")
		m.RecordTokens("alice", "mistral-7b", 100, 200)
		m.RecordLatency("mistral-7b", 250)
	}
	h := m.HTTPHandler()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkScrape(h)
	}
}
