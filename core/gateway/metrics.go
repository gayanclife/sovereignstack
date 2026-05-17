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
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// GatewayMetrics holds all Prometheus collectors for the gateway.
//
// Recording methods (RecordRequest, RecordTokens, etc.) are the hot path and
// map directly to prometheus atomic operations — no additional locking or
// allocation occurs per-request. Latency is stored in a HistogramVec whose
// Observe() path holds a single short-lived lock per label combination, which
// is negligible compared to the round-trip to the model backend.
//
// The registry is private so Go runtime and process collectors are NOT
// included — node-exporter covers those.
type GatewayMetrics struct {
	registry *prometheus.Registry

	requestsTotal           prometheus.Counter
	activeRequests          prometheus.Gauge
	tokensInputTotal        prometheus.Counter
	tokensOutputTotal       prometheus.Counter
	authFailuresTotal       prometheus.Counter
	accessDeniedTotal       prometheus.Counter
	rateLimitHitsTotal      prometheus.Counter
	tokenQuotaExceededTotal prometheus.Counter
	admissionShedTotal      prometheus.Counter

	requestsByStatus     *prometheus.CounterVec
	requestsByMethod     *prometheus.CounterVec
	requestsByUserModel  *prometheus.CounterVec
	tokensByUserModel    *prometheus.CounterVec
	accessDeniedByUser   *prometheus.CounterVec
	authFailuresByReason *prometheus.CounterVec
	admissionShedByModel *prometheus.CounterVec

	// HistogramVec for per-model latency. Buckets cover the full LLM latency
	// spectrum: fast embeddings (50ms) through slow long-form generation (60s).
	requestDuration *prometheus.HistogramVec
}

// llmLatencyBuckets covers the range from fast embedding calls to slow
// long-form generation responses.
var llmLatencyBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 30, 60}

// NewGatewayMetrics creates and registers all collectors on a private registry.
func NewGatewayMetrics() *GatewayMetrics {
	reg := prometheus.NewRegistry()

	m := &GatewayMetrics{
		registry: reg,

		requestsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gateway_requests_total",
			Help: "Total HTTP requests received by the gateway.",
		}),
		activeRequests: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gateway_active_requests",
			Help: "Requests currently being processed.",
		}),
		tokensInputTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gateway_tokens_input_total",
			Help: "Total prompt tokens forwarded to backends.",
		}),
		tokensOutputTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gateway_tokens_output_total",
			Help: "Total completion tokens returned from backends.",
		}),
		authFailuresTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gateway_auth_failures_total",
			Help: "Total authentication failures.",
		}),
		accessDeniedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gateway_access_denied_total",
			Help: "Total access-denied rejections.",
		}),
		rateLimitHitsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gateway_rate_limit_hits_total",
			Help: "Total requests rejected by the per-user rate limiter.",
		}),
		tokenQuotaExceededTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gateway_token_quota_exceeded_total",
			Help: "Total requests rejected because the user's token quota was exhausted.",
		}),
		admissionShedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gateway_admission_shed_total",
			Help: "Total requests shed by the host-aware admission controller.",
		}),

		requestsByStatus: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_requests_by_status",
			Help: "Requests broken down by HTTP status code.",
		}, []string{"status"}),

		requestsByMethod: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_requests_by_method",
			Help: "Requests broken down by HTTP method.",
		}, []string{"method"}),

		requestsByUserModel: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_requests_by_user_model",
			Help: "Successful requests broken down by user and model.",
		}, []string{"user", "model"}),

		tokensByUserModel: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_tokens_by_user_model",
			Help: "Token usage broken down by user, model, and direction (input/output).",
		}, []string{"user", "model", "direction"}),

		accessDeniedByUser: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_access_denied_by_user",
			Help: "Access-denied events broken down by user.",
		}, []string{"user"}),

		authFailuresByReason: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_auth_failures_by_reason",
			Help: "Authentication failures broken down by reason.",
		}, []string{"reason"}),

		admissionShedByModel: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_admission_shed_by_model",
			Help: "Admission-shed events broken down by target model.",
		}, []string{"model"}),

		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gateway_request_duration_seconds",
			Help:    "End-to-end request latency from gateway receipt to backend response.",
			Buckets: llmLatencyBuckets,
		}, []string{"model"}),
	}

	reg.MustRegister(
		m.requestsTotal,
		m.activeRequests,
		m.tokensInputTotal,
		m.tokensOutputTotal,
		m.authFailuresTotal,
		m.accessDeniedTotal,
		m.rateLimitHitsTotal,
		m.tokenQuotaExceededTotal,
		m.admissionShedTotal,
		m.requestsByStatus,
		m.requestsByMethod,
		m.requestsByUserModel,
		m.tokensByUserModel,
		m.accessDeniedByUser,
		m.authFailuresByReason,
		m.admissionShedByModel,
		m.requestDuration,
	)

	return m
}

// HTTPHandler returns an http.Handler that serves the Prometheus text format.
// Mount it at /metrics on the gateway mux.
func (m *GatewayMetrics) HTTPHandler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// RecordRequest increments the total and active request counters.
func (m *GatewayMetrics) RecordRequest() {
	m.requestsTotal.Inc()
	m.activeRequests.Inc()
}

// RecordRequestComplete decrements active requests and records per-status,
// per-method, and per-user/model breakdowns.
func (m *GatewayMetrics) RecordRequestComplete(statusCode int, method, userID, modelName string) {
	m.activeRequests.Dec()
	m.requestsByStatus.WithLabelValues(http.StatusText(statusCode)).Inc()
	m.requestsByMethod.WithLabelValues(method).Inc()
	if userID != "" && modelName != "" {
		m.requestsByUserModel.WithLabelValues(userID, modelName).Inc()
	}
}

// RecordLatency records end-to-end latency for a model in milliseconds.
// Converted to seconds internally to match Prometheus conventions.
func (m *GatewayMetrics) RecordLatency(modelName string, latencyMs int64) {
	if modelName == "" {
		return
	}
	m.requestDuration.WithLabelValues(modelName).Observe(float64(latencyMs) / 1000.0)
}

// RecordTokens records prompt and completion token counts.
func (m *GatewayMetrics) RecordTokens(userID, modelName string, inputTokens, outputTokens int64) {
	m.tokensInputTotal.Add(float64(inputTokens))
	m.tokensOutputTotal.Add(float64(outputTokens))
	if userID != "" && modelName != "" {
		m.tokensByUserModel.WithLabelValues(userID, modelName, "input").Add(float64(inputTokens))
		m.tokensByUserModel.WithLabelValues(userID, modelName, "output").Add(float64(outputTokens))
	}
}

// RecordAuthFailure records an authentication failure with a reason label.
func (m *GatewayMetrics) RecordAuthFailure(reason string) {
	m.authFailuresTotal.Inc()
	m.authFailuresByReason.WithLabelValues(reason).Inc()
}

// RecordAccessDenied records an access-denied event for a user.
func (m *GatewayMetrics) RecordAccessDenied(userID string) {
	m.accessDeniedTotal.Inc()
	if userID != "" {
		m.accessDeniedByUser.WithLabelValues(userID).Inc()
	}
}

// RecordRateLimitHit records a rate-limit rejection.
func (m *GatewayMetrics) RecordRateLimitHit() {
	m.rateLimitHitsTotal.Inc()
}

// RecordTokenQuotaExceeded records a token-quota rejection.
func (m *GatewayMetrics) RecordTokenQuotaExceeded() {
	m.tokenQuotaExceededTotal.Inc()
}

// RecordAdmissionShed records a host-aware admission rejection.
func (m *GatewayMetrics) RecordAdmissionShed(modelName string) {
	m.admissionShedTotal.Inc()
	if modelName != "" {
		m.admissionShedByModel.WithLabelValues(modelName).Inc()
	}
}
