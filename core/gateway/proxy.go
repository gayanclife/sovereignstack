package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gayanclife/sovereignstack/core/admission"
	"github.com/gayanclife/sovereignstack/core/audit"
)

// AuthProvider defines the authentication interface
type AuthProvider interface {
	ValidateToken(token string) (userID string, err error)
}

// RateLimiter tracks rate limits per user
type RateLimiter struct {
	limits map[string]*UserRateLimit
	mu     sync.RWMutex
}

// UserRateLimit tracks tokens per user
type UserRateLimit struct {
	tokens       float64
	lastRefillAt time.Time
	mu           sync.Mutex
}

// Gateway is the HTTP reverse proxy with auth and audit logging
type Gateway struct {
	targetURL        *url.URL
	proxy            *httputil.ReverseProxy
	authProvider     AuthProvider
	accessController AccessController
	quotaManager     *TokenQuotaManager
	modelRouter      *ModelRouter
	admissionCtrl    *admission.Controller
	Metrics          *GatewayMetrics
	auditLogger      audit.AuditLogger
	rateLimiter      *RateLimiter
	requestsPerMin   float64 // Tokens per minute per user
	APIKeyHeader     string  // Header name for API key (default: "X-API-Key")
}

// GatewayConfig holds gateway configuration
type GatewayConfig struct {
	TargetURL        string                 // Backend vLLM service URL (e.g., http://localhost:8000)
	AuthProvider     AuthProvider           // Custom auth provider
	AccessController AccessController       // Optional access control (Phase 2)
	QuotaManager     *TokenQuotaManager     // Optional token quota manager (Phase 2b)
	ModelRouter      *ModelRouter           // Optional model router for multi-model backends (Phase 3)
	AdmissionCtrl    *admission.Controller  // Optional host-aware circuit breaker (nil disables)
	AuditLogger      audit.AuditLogger      // Audit logger
	RequestsPerMin   float64                // Rate limit: requests per minute (0 = unlimited)
	APIKeyHeader     string                 // Header for API key (default: X-API-Key)
}

// NewGateway creates a new reverse proxy gateway
func NewGateway(config GatewayConfig) (*Gateway, error) {
	// Parse target URL
	target, err := url.Parse(config.TargetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	apiKeyHeader := config.APIKeyHeader
	if apiKeyHeader == "" {
		apiKeyHeader = "X-API-Key"
	}

	gw := &Gateway{
		targetURL:        target,
		authProvider:     config.AuthProvider,
		accessController: config.AccessController,
		quotaManager:     config.QuotaManager,
		modelRouter:      config.ModelRouter,
		admissionCtrl:    config.AdmissionCtrl,
		auditLogger:      config.AuditLogger,
		rateLimiter:      &RateLimiter{limits: make(map[string]*UserRateLimit)},
		requestsPerMin:   config.RequestsPerMin,
		APIKeyHeader:     apiKeyHeader,
	}

	// Create reverse proxy with custom director
	gw.proxy = &httputil.ReverseProxy{
		Director:       gw.director,
		ModifyResponse: gw.modifyResponse,
	}

	return gw, nil
}

// director modifies the request before forwarding to backend
func (gw *Gateway) director(req *http.Request) {
	// Determine target URL based on model router (Phase 3)
	targetURL := gw.targetURL

	// If model router is enabled, check for model-based routing
	if gw.modelRouter != nil {
		modelName := extractModelNameFromPath(req.URL.Path)
		if modelName != "" {
			// Strip unconditionally: /models/{name} is a gateway routing
			// directive that no backend understands. Do this before the
			// registry lookup so the path is clean even on a cache miss
			// (e.g. a concurrent registry refresh removed the model between
			// the ServeHTTP capability check and this director call).
			req.URL.Path = stripModelPrefixFromPath(req.URL.Path, modelName)
			req.URL.RawPath = "" // clear stale hint so EscapedPath uses the new Path
			if backend, exists := gw.modelRouter.GetBackend(modelName); exists {
				u, _ := url.Parse(backend.URL)
				targetURL = u
			}
		}
	}

	req.URL.Scheme = targetURL.Scheme
	req.URL.Host = targetURL.Host
	req.URL.Path = singleJoiningSlash(targetURL.Path, req.URL.Path)
	if targetURL.RawQuery == "" || req.URL.RawQuery == "" {
		req.URL.RawQuery = targetURL.RawQuery + req.URL.RawQuery
	} else {
		req.URL.RawQuery = targetURL.RawQuery + "&" + req.URL.RawQuery
	}

	// Remove authorization headers from being forwarded
	req.Header.Del(gw.APIKeyHeader)

	// Add X-Forwarded headers for backend
	if clientIP := getClientIP(req); clientIP != "" {
		req.Header.Set("X-Forwarded-For", clientIP)
	}
	req.Header.Set("X-Forwarded-Proto", "http")
	if req.Header.Get("X-Forwarded-Host") == "" {
		req.Header.Set("X-Forwarded-Host", req.Host)
	}
}

// modifyResponse processes the response before returning to client
func (gw *Gateway) modifyResponse(resp *http.Response) error {
	// Copy status code and body as-is
	// Could add additional processing here (e.g., response filtering, header modification)
	return nil
}

// ServeHTTP handles incoming requests with authentication and audit logging
func (gw *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	correlationID := generateCorrelationID()
	startTime := time.Now()
	clientIP := getClientIP(r)
	userAgent := r.Header.Get("User-Agent")

	// Record request (Phase 4 metrics)
	if gw.Metrics != nil {
		gw.Metrics.RecordRequest()
	}

	// Extract API key
	apiKey := r.Header.Get(gw.APIKeyHeader)
	if apiKey == "" && strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		apiKey = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}

	var userID string

	// Authenticate user
	if gw.authProvider != nil && apiKey != "" {
		var err error
		userID, err = gw.authProvider.ValidateToken(apiKey)
		if err != nil {
			gw.auditLogger.LogAuthFailure(apiKey[:min(len(apiKey), 8)]+"...", r.RequestURI, clientIP, err.Error())
			if gw.Metrics != nil {
				gw.Metrics.RecordAuthFailure("invalid_key")
			}
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Serve the gateway-level /v1/models list before touching model routing.
	// Authenticated users see only the models they are allowed to access.
	if r.Method == http.MethodGet && r.URL.Path == "/v1/models" && gw.modelRouter != nil {
		gw.serveModelList(w, userID)
		return
	}

	// Prometheus HTTP SD — returns scrape targets for all running model backends.
	// No auth required: this is consumed by Prometheus server-side, not by end users.
	if r.Method == http.MethodGet && r.URL.Path == "/api/v1/prometheus/targets" && gw.modelRouter != nil {
		gw.servePrometheusTargets(w)
		return
	}

	// Per-model metrics proxy: GET /models/{name}/metrics
	// Forwards to the model backend's own /metrics endpoint (vLLM Prometheus exposition).
	// No auth required: metrics are non-sensitive operational data consumed by Prometheus.
	if r.Method == http.MethodGet && gw.modelRouter != nil {
		if name := extractModelNameFromPath(r.URL.Path); name != "" &&
			strings.HasSuffix(r.URL.Path, "/metrics") {
			gw.serveModelMetrics(w, r, name)
			return
		}
	}

	// Extract model name from request if available (needed for access control)
	modelName := extractModelName(r)

	// Phase F2: enforce service-account IP allowlist (no-op for non-service users).
	if ksac, ok := gw.accessController.(interface {
		IsSourceIPAllowed(userID, sourceIP string) bool
	}); ok && userID != "" {
		if !ksac.IsSourceIPAllowed(userID, clientIP) {
			gw.auditLogger.LogError(userID, r.RequestURI, correlationID,
				fmt.Sprintf("source IP %s not in allowlist", clientIP),
				http.StatusForbidden, clientIP)
			if gw.Metrics != nil {
				gw.Metrics.RecordAccessDenied(userID)
			}
			http.Error(w, `{"error":"source IP not allowed for this service account"}`, http.StatusForbidden)
			return
		}
	}

	// Check access control (Phase 2)
	if gw.accessController != nil && userID != "" {
		if !gw.accessController.CanAccess(userID, modelName) {
			gw.auditLogger.LogError(userID, r.RequestURI, correlationID, fmt.Sprintf("access denied to model: %s", modelName), http.StatusForbidden, clientIP)
			if gw.Metrics != nil {
				gw.Metrics.RecordAccessDenied(userID)
			}
			http.Error(w, fmt.Sprintf(`{"error":"access denied","model":"%s"}`, modelName), http.StatusForbidden)
			return
		}
	}

	// Check token quota (Phase 2b)
	if gw.quotaManager != nil && userID != "" {
		if err := gw.quotaManager.CheckQuota(userID); err != nil {
			gw.auditLogger.LogError(userID, r.RequestURI, correlationID, fmt.Sprintf("token quota exceeded: %v", err), http.StatusTooManyRequests, clientIP)
			if gw.Metrics != nil {
				gw.Metrics.RecordTokenQuotaExceeded()
			}
			http.Error(w, fmt.Sprintf(`{"error":"token_quota_exceeded","detail":"%s"}`, err.Error()), http.StatusTooManyRequests)
			return
		}
	}

	// Check rate limit
	if gw.requestsPerMin > 0 && !gw.rateLimiter.Allow(userID, gw.requestsPerMin) {
		gw.auditLogger.LogError(userID, r.RequestURI, correlationID, "rate limit exceeded", http.StatusTooManyRequests, clientIP)
		if gw.Metrics != nil {
			gw.Metrics.RecordRateLimitHit()
		}
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// Host-aware admission check (kv-cache pressure + queue depth budget).
	// Sits AFTER per-user limits so a single user can't bypass user-level
	// caps via this circuit, and BEFORE the body read so we shed cheaply.
	if gw.admissionCtrl != nil {
		if d := gw.admissionCtrl.Allow(modelName); !d.Allow {
			gw.auditLogger.LogError(userID, r.RequestURI, correlationID,
				"admission shed: "+d.Reason, http.StatusServiceUnavailable, clientIP)
			if gw.Metrics != nil {
				gw.Metrics.RecordAdmissionShed(modelName)
			}
			if d.RetryAfter > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(d.RetryAfter.Seconds())))
			}
			http.Error(w,
				fmt.Sprintf(`{"error":"service_unavailable","detail":%q}`, d.Reason),
				http.StatusServiceUnavailable)
			return
		}
	}

	// Capability check — short-circuit with a clear error when the caller
	// hits an endpoint the model can't serve (e.g. /v1/chat/completions on
	// an encoder model). Without this, the request would forward and get
	// a misleading bare {"detail":"Not Found"} from the model's FastAPI.
	// Best-effort: fires only when the router has populated capability info
	// AND the request matches the routed-by-model URL shape; otherwise
	// passes through and preserves prior behavior.
	if gw.modelRouter != nil {
		routedName := extractModelNameFromPath(r.URL.Path)
		if routedName != "" {
			if backend, exists := gw.modelRouter.GetBackend(routedName); exists {
				stripped := stripModelPrefixFromPath(r.URL.Path, routedName)
				if !backend.IsPathSupported(stripped) {
					reason := fmt.Sprintf(
						"model %q (capability %q) does not serve %s; supported endpoints: %s",
						routedName, backend.Capability, stripped,
						strings.Join(backend.Endpoints, ", "))
					gw.auditLogger.LogError(userID, r.RequestURI, correlationID,
						reason, http.StatusNotFound, clientIP)
					w.Header().Set("X-Supported-Endpoints", strings.Join(backend.Endpoints, ","))
					http.Error(w,
						fmt.Sprintf(`{"error":"endpoint_not_supported_for_model","model":%q,"capability":%q,"requested":%q,"supported":%s}`,
							routedName, backend.Capability, stripped,
							jsonStringArray(backend.Endpoints)),
						http.StatusNotFound)
					return
				}
			} else {
				// Path has /models/{name}/... prefix but the named model isn't in
				// the registry. Reject here instead of forwarding the unstripped
				// path to the default backend, which would receive a path vLLM
				// doesn't understand and return a confusing bare 404.
				gw.auditLogger.LogError(userID, r.RequestURI, correlationID,
					fmt.Sprintf("model %q not deployed", routedName), http.StatusNotFound, clientIP)
				http.Error(w,
					fmt.Sprintf(`{"error":"model_not_deployed","model":%q}`, routedName),
					http.StatusNotFound)
				return
			}
		}
	}

	// Log incoming request
	var requestBody []byte
	if r.Body != nil {
		var err error
		requestBody, err = io.ReadAll(r.Body)
		if err != nil {
			gw.auditLogger.LogError(userID, r.RequestURI, correlationID, fmt.Sprintf("failed to read body: %v", err), http.StatusBadRequest, clientIP)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		// Restore body for backend to read
		r.Body = io.NopCloser(bytes.NewBuffer(requestBody))
	}

	// Log request
	gw.auditLogger.LogRequest(userID, modelName, r.RequestURI, r.Method, clientIP, userAgent, correlationID, int64(len(requestBody)))

	// Create response writer wrapper to capture response details
	wrappedWriter := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	// Forward to backend
	gw.proxy.ServeHTTP(wrappedWriter, r)

	// Calculate request duration
	duration := time.Since(startTime).Milliseconds()

	// Parse token usage from response body (works for both JSON and SSE streaming).
	inputTokens, outputTokens := parseTokenUsage(wrappedWriter.body, wrappedWriter.Header().Get("Content-Type"))

	// Record metrics (Phase 4)
	if gw.Metrics != nil {
		gw.Metrics.RecordRequestComplete(wrappedWriter.statusCode, r.Method, userID, modelName)
		gw.Metrics.RecordLatency(modelName, duration)
		if inputTokens > 0 || outputTokens > 0 {
			gw.Metrics.RecordTokens(userID, modelName, inputTokens, outputTokens)
		}
	}

	// Record quota usage against actual token counts.
	if gw.quotaManager != nil && (inputTokens > 0 || outputTokens > 0) {
		gw.quotaManager.Record(userID, inputTokens, outputTokens)
	}

	// Log response
	if wrappedWriter.statusCode < 400 {
		gw.auditLogger.LogResponse(userID, modelName, r.RequestURI, correlationID, wrappedWriter.statusCode, int64(len(wrappedWriter.body)), int(inputTokens), int(outputTokens), duration)
	} else {
		gw.auditLogger.LogError(userID, r.RequestURI, correlationID, fmt.Sprintf("backend returned %d", wrappedWriter.statusCode), wrappedWriter.statusCode, clientIP)
	}
}

// Allow checks if a user is within rate limits
func (rl *RateLimiter) Allow(userID string, requestsPerMin float64) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if requestsPerMin == 0 {
		return true // Unlimited
	}

	limit, exists := rl.limits[userID]
	if !exists {
		limit = &UserRateLimit{
			tokens:       requestsPerMin,
			lastRefillAt: time.Now(),
		}
		rl.limits[userID] = limit
	}

	limit.mu.Lock()
	defer limit.mu.Unlock()

	// Refill tokens based on time passed
	now := time.Now()
	timePassed := now.Sub(limit.lastRefillAt).Minutes()
	tokensToAdd := timePassed * requestsPerMin
	newTokens := limit.tokens + tokensToAdd
	if newTokens > requestsPerMin {
		limit.tokens = requestsPerMin
	} else {
		limit.tokens = newTokens
	}
	limit.lastRefillAt = now

	// Check if we have tokens
	if limit.tokens >= 1 {
		limit.tokens--
		return true
	}

	return false
}

// responseWriter wraps http.ResponseWriter to capture response details
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	body       []byte
}

// WriteHeader captures the status code
func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// Write captures response body
func (w *responseWriter) Write(b []byte) (int, error) {
	w.body = append(w.body, b...)
	return w.ResponseWriter.Write(b)
}

// Helper functions

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	parts := strings.Split(r.RemoteAddr, ":")
	if len(parts) > 0 {
		return parts[0]
	}

	return r.RemoteAddr
}

func extractModelName(r *http.Request) string {
	// URL path takes priority: /models/{name}/... or /v1/models/{name}/...
	if name := extractModelNameFromPath(r.URL.Path); name != "" {
		return name
	}

	// Fall back to the "model" field in the JSON request body. This supports
	// standard OpenAI-compatible clients that send POST /v1/chat/completions
	// with {"model": "mistral-7b", ...} rather than encoding the model in the URL.
	if (r.Method == http.MethodPost || r.Method == http.MethodPut) && r.Body != nil {
		// Peek at the first 512 bytes — the model field is always near the top.
		peek := make([]byte, 512)
		n, _ := r.Body.Read(peek)
		peek = peek[:n]
		// Restore the body so downstream reads (audit logging, proxy) work correctly.
		r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(peek), r.Body))
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(peek, &payload); err == nil && payload.Model != "" {
			return payload.Model
		}
	}

	return "unknown"
}

// serveModelList writes an OpenAI-compatible GET /v1/models response containing
// all currently deployed models the authenticated user is allowed to access.
// Capability and supported endpoints are included as extensions so clients like
// OpenCode can distinguish generative from encoder models.
func (gw *Gateway) serveModelList(w http.ResponseWriter, userID string) {
	type modelEntry struct {
		ID         string   `json:"id"`
		Object     string   `json:"object"`
		Created    int64    `json:"created"`
		OwnedBy    string   `json:"owned_by"`
		Capability string   `json:"capability,omitempty"`
		Endpoints  []string `json:"endpoints,omitempty"`
	}
	type modelList struct {
		Object string       `json:"object"`
		Data   []modelEntry `json:"data"`
	}

	backends := gw.modelRouter.ListModels()
	now := time.Now().Unix()
	data := make([]modelEntry, 0, len(backends))
	for _, b := range backends {
		if gw.accessController != nil && userID != "" && !gw.accessController.CanAccess(userID, b.Name) {
			continue
		}
		data = append(data, modelEntry{
			ID:         b.Name,
			Object:     "model",
			Created:    now,
			OwnedBy:    "sovereignstack",
			Capability: b.Capability,
			Endpoints:  b.Endpoints,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(modelList{Object: "list", Data: data})
}

// servePrometheusTargets writes a Prometheus HTTP SD response listing every
// running model backend as a scrape target. Prometheus polls this endpoint
// instead of using a static target list, so new models are scraped automatically
// without reloading prometheus.yml.
//
// Response shape (Prometheus HTTP SD v1):
//
//	[{"targets":["localhost:8000"],"labels":{"model":"mistral-7b","job":"model-metrics"}}]
func (gw *Gateway) servePrometheusTargets(w http.ResponseWriter) {
	type sdTarget struct {
		Targets []string          `json:"targets"`
		Labels  map[string]string `json:"labels"`
	}

	backends := gw.modelRouter.ListModels()
	targets := make([]sdTarget, 0, len(backends))
	for _, b := range backends {
		if b.Port == 0 {
			continue // port unknown; skip
		}
		targets = append(targets, sdTarget{
			Targets: []string{fmt.Sprintf("localhost:%d", b.Port)},
			Labels: map[string]string{
				"job":   "model-metrics",
				"model": b.Name,
			},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(targets)
}

// serveModelMetrics reverse-proxies GET /models/{name}/metrics to the model
// backend's own /metrics endpoint, allowing Prometheus to scrape vLLM metrics
// through the gateway without exposing model container ports externally.
func (gw *Gateway) serveModelMetrics(w http.ResponseWriter, r *http.Request, modelName string) {
	backend, exists := gw.modelRouter.GetBackend(modelName)
	if !exists {
		http.Error(w, `{"error":"model not deployed"}`, http.StatusNotFound)
		return
	}

	targetURL := backend.URL + "/metrics"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL, nil)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, `{"error":"backend unreachable"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Forward content-type so Prometheus receives valid exposition format.
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func generateCorrelationID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().Nanosecond())
}

// extractModelNameFromPath extracts the model name from paths that embed it
// for routing purposes. Handles both formats:
//
//	/models/{name}/v1/...      (SovereignStack native)
//	/v1/models/{name}/...      (OpenAI-style prefix)
//
// At least one path component must follow the model name so that the
// /v1/models listing endpoint (no name, no trailing path) is not mistaken
// for a routing directive.
func extractModelNameFromPath(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		if part == "models" && i+2 < len(parts) && parts[i+1] != "" {
			return parts[i+1]
		}
	}
	return ""
}

// stripModelPrefixFromPath removes the /models/{model-name} segment from
// wherever it appears in the path, joining the surrounding components.
//
//	/models/mistral-7b/v1/chat/completions  → /v1/chat/completions
//	/v1/models/mistral-7b/chat/completions  → /v1/chat/completions
func stripModelPrefixFromPath(path, modelName string) string {
	segment := "/models/" + modelName
	idx := strings.Index(path, segment)
	if idx < 0 {
		return path
	}
	end := idx + len(segment)
	// Verify the model name ends at a path boundary to avoid partial matches
	// (e.g., model "llama" must not strip from a path containing "llama-3-8b").
	if end < len(path) && path[end] != '/' {
		return path
	}
	result := path[:idx] + path[end:]
	if result == "" {
		return "/"
	}
	if !strings.HasPrefix(result, "/") {
		result = "/" + result
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// jsonStringArray emits a small JSON array literal from a Go []string.
// We only use it for the supported-endpoints list in the
// endpoint_not_supported_for_model error body, so the standard library
// (encoding/json + bytes.Buffer round-trip) would be overkill.
func jsonStringArray(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, s := range items {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('"')
		// Escape the two characters that can appear in HTTP path strings
		// and would break the JSON literal.
		s = strings.ReplaceAll(s, `\`, `\\`)
		s = strings.ReplaceAll(s, `"`, `\"`)
		b.WriteString(s)
		b.WriteByte('"')
	}
	b.WriteByte(']')
	return b.String()
}

// parseTokenUsage extracts prompt_tokens and completion_tokens from a backend
// response. It handles both plain JSON (non-streaming) and SSE streaming
// responses (text/event-stream), where usage appears in the last data chunk.
func parseTokenUsage(body []byte, contentType string) (inputTokens, outputTokens int64) {
	type usagePayload struct {
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}

	tryParse := func(b []byte) (int64, int64, bool) {
		var p usagePayload
		if err := json.Unmarshal(b, &p); err != nil {
			return 0, 0, false
		}
		if p.Usage.PromptTokens == 0 && p.Usage.CompletionTokens == 0 {
			return 0, 0, false
		}
		return p.Usage.PromptTokens, p.Usage.CompletionTokens, true
	}

	if strings.Contains(contentType, "text/event-stream") {
		// Scan SSE lines in reverse to find the last chunk with usage.
		lines := bytes.Split(body, []byte("\n"))
		for i := len(lines) - 1; i >= 0; i-- {
			line := bytes.TrimSpace(lines[i])
			if !bytes.HasPrefix(line, []byte("data:")) {
				continue
			}
			payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
			if bytes.Equal(payload, []byte("[DONE]")) {
				continue
			}
			if in, out, ok := tryParse(payload); ok {
				return in, out
			}
		}
		return 0, 0
	}

	in, out, _ := tryParse(body)
	return in, out
}
