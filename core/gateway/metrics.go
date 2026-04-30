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
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// GatewayMetrics tracks Prometheus metrics for the gateway
type GatewayMetrics struct {
	// Counters (hot path - use atomic)
	requestsTotal            int64 // Total requests
	authFailuresTotal        int64 // Auth failures
	accessDeniedTotal        int64 // Access denials
	rateLimitHitsTotal       int64 // Rate limit rejections
	tokenQuotaExceededTotal  int64 // Token quota rejections
	tokensInputTotal         int64 // Total input tokens
	tokensOutputTotal        int64 // Total output tokens
	activeRequests           int64 // Currently processing requests

	// Labeled counters (require locking)
	mu                  sync.RWMutex
	requestsByUserModel map[string]int64      // "user:model" → count
	requestsByStatus    map[string]int64      // "status_code" → count
	requestsByMethod    map[string]int64      // "method" → count
	tokensByUserModel   map[string][2]int64   // "user:model" → [input, output]
	latencyByModel      map[string][]int64    // "model" → latencies in ms
	accessDeniedByUser  map[string]int64      // "user" → count
	authFailuresByReason map[string]int64     // "reason" → count
}

// NewGatewayMetrics creates a new metrics tracker
func NewGatewayMetrics() *GatewayMetrics {
	return &GatewayMetrics{
		requestsByUserModel:  make(map[string]int64),
		requestsByStatus:     make(map[string]int64),
		requestsByMethod:     make(map[string]int64),
		tokensByUserModel:    make(map[string][2]int64),
		latencyByModel:       make(map[string][]int64),
		accessDeniedByUser:   make(map[string]int64),
		authFailuresByReason: make(map[string]int64),
	}
}

// RecordRequest increments total request count
func (m *GatewayMetrics) RecordRequest() {
	atomic.AddInt64(&m.requestsTotal, 1)
	atomic.AddInt64(&m.activeRequests, 1)
}

// RecordRequestComplete decrements active requests and records status/method/user/model
func (m *GatewayMetrics) RecordRequestComplete(statusCode int, method, userID, modelName string) {
	atomic.AddInt64(&m.activeRequests, -1)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Record by status code
	statusKey := fmt.Sprintf("%d", statusCode)
	m.requestsByStatus[statusKey]++

	// Record by method
	m.requestsByMethod[method]++

	// Record by user and model
	if userID != "" && modelName != "" {
		key := userID + ":" + modelName
		m.requestsByUserModel[key]++
	}
}

// RecordLatency records request latency for a model
func (m *GatewayMetrics) RecordLatency(modelName string, latencyMs int64) {
	if modelName == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.latencyByModel[modelName] = append(m.latencyByModel[modelName], latencyMs)
}

// RecordTokens records token usage for a user and model
func (m *GatewayMetrics) RecordTokens(userID, modelName string, inputTokens, outputTokens int64) {
	atomic.AddInt64(&m.tokensInputTotal, inputTokens)
	atomic.AddInt64(&m.tokensOutputTotal, outputTokens)

	if userID == "" || modelName == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := userID + ":" + modelName
	current := m.tokensByUserModel[key]
	current[0] += inputTokens
	current[1] += outputTokens
	m.tokensByUserModel[key] = current
}

// RecordAuthFailure records an authentication failure
func (m *GatewayMetrics) RecordAuthFailure(reason string) {
	atomic.AddInt64(&m.authFailuresTotal, 1)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.authFailuresByReason[reason]++
}

// RecordAccessDenied records an access denied event
func (m *GatewayMetrics) RecordAccessDenied(userID string) {
	atomic.AddInt64(&m.accessDeniedTotal, 1)

	if userID == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.accessDeniedByUser[userID]++
}

// RecordRateLimitHit records a rate limit rejection
func (m *GatewayMetrics) RecordRateLimitHit() {
	atomic.AddInt64(&m.rateLimitHitsTotal, 1)
}

// RecordTokenQuotaExceeded records a token quota rejection
func (m *GatewayMetrics) RecordTokenQuotaExceeded() {
	atomic.AddInt64(&m.tokenQuotaExceededTotal, 1)
}

// WritePrometheusText writes metrics in Prometheus text format
func (m *GatewayMetrics) WritePrometheusText() string {
	var output strings.Builder

	output.WriteString("# HELP gateway_requests_total Total HTTP requests processed\n")
	output.WriteString("# TYPE gateway_requests_total counter\n")
	output.WriteString(fmt.Sprintf("gateway_requests_total %d\n", atomic.LoadInt64(&m.requestsTotal)))
	output.WriteString("\n")

	output.WriteString("# HELP gateway_active_requests Currently active requests\n")
	output.WriteString("# TYPE gateway_active_requests gauge\n")
	output.WriteString(fmt.Sprintf("gateway_active_requests %d\n", atomic.LoadInt64(&m.activeRequests)))
	output.WriteString("\n")

	// Requests by status
	output.WriteString("# HELP gateway_requests_by_status Requests by status code\n")
	output.WriteString("# TYPE gateway_requests_by_status counter\n")
	m.mu.RLock()
	statusKeys := make([]string, 0, len(m.requestsByStatus))
	for k := range m.requestsByStatus {
		statusKeys = append(statusKeys, k)
	}
	sort.Strings(statusKeys)
	for _, status := range statusKeys {
		output.WriteString(fmt.Sprintf("gateway_requests_by_status{status=\"%s\"} %d\n", status, m.requestsByStatus[status]))
	}
	m.mu.RUnlock()
	output.WriteString("\n")

	// Requests by method
	output.WriteString("# HELP gateway_requests_by_method Requests by HTTP method\n")
	output.WriteString("# TYPE gateway_requests_by_method counter\n")
	m.mu.RLock()
	methodKeys := make([]string, 0, len(m.requestsByMethod))
	for k := range m.requestsByMethod {
		methodKeys = append(methodKeys, k)
	}
	sort.Strings(methodKeys)
	for _, method := range methodKeys {
		output.WriteString(fmt.Sprintf("gateway_requests_by_method{method=\"%s\"} %d\n", method, m.requestsByMethod[method]))
	}
	m.mu.RUnlock()
	output.WriteString("\n")

	// Tokens total
	output.WriteString("# HELP gateway_tokens_input_total Total input tokens\n")
	output.WriteString("# TYPE gateway_tokens_input_total counter\n")
	output.WriteString(fmt.Sprintf("gateway_tokens_input_total %d\n", atomic.LoadInt64(&m.tokensInputTotal)))
	output.WriteString("\n")

	output.WriteString("# HELP gateway_tokens_output_total Total output tokens\n")
	output.WriteString("# TYPE gateway_tokens_output_total counter\n")
	output.WriteString(fmt.Sprintf("gateway_tokens_output_total %d\n", atomic.LoadInt64(&m.tokensOutputTotal)))
	output.WriteString("\n")

	// Auth failures
	output.WriteString("# HELP gateway_auth_failures_total Total authentication failures\n")
	output.WriteString("# TYPE gateway_auth_failures_total counter\n")
	output.WriteString(fmt.Sprintf("gateway_auth_failures_total %d\n", atomic.LoadInt64(&m.authFailuresTotal)))
	output.WriteString("\n")

	// Access denied
	output.WriteString("# HELP gateway_access_denied_total Total access denied events\n")
	output.WriteString("# TYPE gateway_access_denied_total counter\n")
	output.WriteString(fmt.Sprintf("gateway_access_denied_total %d\n", atomic.LoadInt64(&m.accessDeniedTotal)))
	output.WriteString("\n")

	// Rate limit hits
	output.WriteString("# HELP gateway_rate_limit_hits_total Total rate limit rejections\n")
	output.WriteString("# TYPE gateway_rate_limit_hits_total counter\n")
	output.WriteString(fmt.Sprintf("gateway_rate_limit_hits_total %d\n", atomic.LoadInt64(&m.rateLimitHitsTotal)))
	output.WriteString("\n")

	// Token quota exceeded
	output.WriteString("# HELP gateway_token_quota_exceeded_total Total token quota rejections\n")
	output.WriteString("# TYPE gateway_token_quota_exceeded_total counter\n")
	output.WriteString(fmt.Sprintf("gateway_token_quota_exceeded_total %d\n", atomic.LoadInt64(&m.tokenQuotaExceededTotal)))
	output.WriteString("\n")

	// Per-model latency percentiles
	output.WriteString("# HELP gateway_request_duration_seconds Request duration percentiles\n")
	output.WriteString("# TYPE gateway_request_duration_seconds summary\n")
	m.mu.RLock()
	modelKeys := make([]string, 0, len(m.latencyByModel))
	for k := range m.latencyByModel {
		modelKeys = append(modelKeys, k)
	}
	sort.Strings(modelKeys)
	for _, model := range modelKeys {
		latencies := m.latencyByModel[model]
		if len(latencies) > 0 {
			sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
			p50 := latencies[len(latencies)/2]
			p95 := latencies[int(float64(len(latencies))*0.95)]
			p99 := latencies[int(float64(len(latencies))*0.99)]

			output.WriteString(fmt.Sprintf("gateway_request_duration_seconds{model=\"%s\",quantile=\"0.5\"} %d\n", model, p50))
			output.WriteString(fmt.Sprintf("gateway_request_duration_seconds{model=\"%s\",quantile=\"0.95\"} %d\n", model, p95))
			output.WriteString(fmt.Sprintf("gateway_request_duration_seconds{model=\"%s\",quantile=\"0.99\"} %d\n", model, p99))
		}
	}
	m.mu.RUnlock()
	output.WriteString("\n")

	return output.String()
}

// GetMetricsSnapshot returns a snapshot of all metrics
func (m *GatewayMetrics) GetMetricsSnapshot() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"requests_total":           atomic.LoadInt64(&m.requestsTotal),
		"active_requests":          atomic.LoadInt64(&m.activeRequests),
		"auth_failures_total":      atomic.LoadInt64(&m.authFailuresTotal),
		"access_denied_total":      atomic.LoadInt64(&m.accessDeniedTotal),
		"rate_limit_hits_total":    atomic.LoadInt64(&m.rateLimitHitsTotal),
		"token_quota_exceeded_total": atomic.LoadInt64(&m.tokenQuotaExceededTotal),
		"tokens_input_total":       atomic.LoadInt64(&m.tokensInputTotal),
		"tokens_output_total":      atomic.LoadInt64(&m.tokensOutputTotal),
		"requests_by_status":       copyStringIntMap(m.requestsByStatus),
		"requests_by_method":       copyStringIntMap(m.requestsByMethod),
		"access_denied_by_user":    copyStringIntMap(m.accessDeniedByUser),
		"auth_failures_by_reason":  copyStringIntMap(m.authFailuresByReason),
	}
}

// Helper functions

func copyStringIntMap(m map[string]int64) map[string]int64 {
	copy := make(map[string]int64)
	for k, v := range m {
		copy[k] = v
	}
	return copy
}
