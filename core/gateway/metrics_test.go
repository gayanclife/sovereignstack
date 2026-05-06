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
	"strings"
	"testing"
)

func TestGatewayMetrics_RecordRequest(t *testing.T) {
	m := NewGatewayMetrics()

	m.RecordRequest()
	m.RecordRequest()
	m.RecordRequest()

	snapshot := m.GetMetricsSnapshot()
	if snapshot["requests_total"] != int64(3) {
		t.Errorf("Expected requests_total=3, got %d", snapshot["requests_total"])
	}
	if snapshot["active_requests"] != int64(3) {
		t.Errorf("Expected active_requests=3, got %d", snapshot["active_requests"])
	}
}

func TestGatewayMetrics_RecordRequestComplete(t *testing.T) {
	m := NewGatewayMetrics()

	m.RecordRequest()
	m.RecordRequestComplete(200, "POST", "alice", "mistral-7b")

	m.RecordRequest()
	m.RecordRequestComplete(403, "POST", "alice", "llama-3")

	snapshot := m.GetMetricsSnapshot()
	if snapshot["active_requests"] != int64(0) {
		t.Errorf("Expected active_requests=0, got %d", snapshot["active_requests"])
	}
}

func TestGatewayMetrics_RecordLatency(t *testing.T) {
	m := NewGatewayMetrics()

	m.RecordLatency("mistral-7b", 100)
	m.RecordLatency("mistral-7b", 150)
	m.RecordLatency("mistral-7b", 200)

	text := m.WritePrometheusText()
	if !strings.Contains(text, "gateway_request_duration_seconds") {
		t.Errorf("Expected latency metrics in output")
	}
}

func TestGatewayMetrics_RecordTokens(t *testing.T) {
	m := NewGatewayMetrics()

	m.RecordTokens("alice", "mistral-7b", 100, 250)
	m.RecordTokens("alice", "mistral-7b", 50, 150)
	m.RecordTokens("bob", "llama-3", 75, 100)

	snapshot := m.GetMetricsSnapshot()
	if snapshot["tokens_input_total"] != int64(225) {
		t.Errorf("Expected tokens_input_total=225, got %d", snapshot["tokens_input_total"])
	}
	if snapshot["tokens_output_total"] != int64(500) {
		t.Errorf("Expected tokens_output_total=500, got %d", snapshot["tokens_output_total"])
	}
}

func TestGatewayMetrics_RecordAuthFailure(t *testing.T) {
	m := NewGatewayMetrics()

	m.RecordAuthFailure("invalid_key")
	m.RecordAuthFailure("invalid_key")
	m.RecordAuthFailure("missing_key")

	snapshot := m.GetMetricsSnapshot()
	if snapshot["auth_failures_total"] != int64(3) {
		t.Errorf("Expected auth_failures_total=3, got %d", snapshot["auth_failures_total"])
	}
}

func TestGatewayMetrics_RecordAccessDenied(t *testing.T) {
	m := NewGatewayMetrics()

	m.RecordAccessDenied("alice")
	m.RecordAccessDenied("alice")
	m.RecordAccessDenied("bob")

	snapshot := m.GetMetricsSnapshot()
	if snapshot["access_denied_total"] != int64(3) {
		t.Errorf("Expected access_denied_total=3, got %d", snapshot["access_denied_total"])
	}
}

func TestGatewayMetrics_RecordRateLimitHit(t *testing.T) {
	m := NewGatewayMetrics()

	for i := 0; i < 5; i++ {
		m.RecordRateLimitHit()
	}

	snapshot := m.GetMetricsSnapshot()
	if snapshot["rate_limit_hits_total"] != int64(5) {
		t.Errorf("Expected rate_limit_hits_total=5, got %d", snapshot["rate_limit_hits_total"])
	}
}

func TestGatewayMetrics_RecordTokenQuotaExceeded(t *testing.T) {
	m := NewGatewayMetrics()

	m.RecordTokenQuotaExceeded()
	m.RecordTokenQuotaExceeded()

	snapshot := m.GetMetricsSnapshot()
	if snapshot["token_quota_exceeded_total"] != int64(2) {
		t.Errorf("Expected token_quota_exceeded_total=2, got %d", snapshot["token_quota_exceeded_total"])
	}
}

func TestGatewayMetrics_WritePrometheusText_Format(t *testing.T) {
	m := NewGatewayMetrics()

	m.RecordRequest()
	m.RecordRequest()
	m.RecordRequestComplete(200, "POST", "alice", "mistral-7b")
	m.RecordTokens("alice", "mistral-7b", 100, 200)
	m.RecordAuthFailure("invalid_key")
	m.RecordAccessDenied("bob")

	text := m.WritePrometheusText()

	// Check for HELP and TYPE lines
	if !strings.Contains(text, "# HELP gateway_requests_total") {
		t.Errorf("Missing HELP comment for requests_total")
	}
	if !strings.Contains(text, "# TYPE gateway_requests_total counter") {
		t.Errorf("Missing TYPE comment for requests_total")
	}

	// Check for actual metrics
	if !strings.Contains(text, "gateway_requests_total 2") {
		t.Errorf("Expected gateway_requests_total 2 in output")
	}
	if !strings.Contains(text, "gateway_auth_failures_total 1") {
		t.Errorf("Expected gateway_auth_failures_total 1 in output")
	}
	if !strings.Contains(text, "gateway_access_denied_total 1") {
		t.Errorf("Expected gateway_access_denied_total 1 in output")
	}
}

func TestGatewayMetrics_WritePrometheusText_ByStatus(t *testing.T) {
	m := NewGatewayMetrics()

	m.RecordRequest()
	m.RecordRequestComplete(200, "POST", "alice", "mistral-7b")
	m.RecordRequest()
	m.RecordRequestComplete(403, "POST", "alice", "mistral-7b")
	m.RecordRequest()
	m.RecordRequestComplete(500, "POST", "alice", "mistral-7b")

	text := m.WritePrometheusText()

	if !strings.Contains(text, "gateway_requests_by_status{status=\"200\"}") {
		t.Errorf("Expected status 200 in output")
	}
	if !strings.Contains(text, "gateway_requests_by_status{status=\"403\"}") {
		t.Errorf("Expected status 403 in output")
	}
	if !strings.Contains(text, "gateway_requests_by_status{status=\"500\"}") {
		t.Errorf("Expected status 500 in output")
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

	snapshot := m.GetMetricsSnapshot()
	if snapshot["requests_total"] != int64(1000) {
		t.Errorf("Expected requests_total=1000, got %d", snapshot["requests_total"])
	}
	if snapshot["active_requests"] != int64(0) {
		t.Errorf("Expected active_requests=0, got %d", snapshot["active_requests"])
	}
	if snapshot["tokens_input_total"] != int64(100000) {
		t.Errorf("Expected tokens_input_total=100000, got %d", snapshot["tokens_input_total"])
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

func BenchmarkGatewayMetrics_WritePrometheusText(b *testing.B) {
	m := NewGatewayMetrics()
	for i := 0; i < 100; i++ {
		m.RecordRequest()
		m.RecordRequestComplete(200, "POST", "alice", "mistral-7b")
		m.RecordTokens("alice", "mistral-7b", 100, 200)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.WritePrometheusText()
	}
}
