package gateway

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

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
	Metrics          *GatewayMetrics
	auditLogger      audit.AuditLogger
	rateLimiter      *RateLimiter
	requestsPerMin   float64 // Tokens per minute per user
	APIKeyHeader     string  // Header name for API key (default: "X-API-Key")
}

// GatewayConfig holds gateway configuration
type GatewayConfig struct {
	TargetURL        string            // Backend vLLM service URL (e.g., http://localhost:8000)
	AuthProvider     AuthProvider      // Custom auth provider
	AccessController AccessController  // Optional access control (Phase 2)
	QuotaManager     *TokenQuotaManager // Optional token quota manager (Phase 2b)
	ModelRouter      *ModelRouter      // Optional model router for multi-model backends (Phase 3)
	AuditLogger      audit.AuditLogger  // Audit logger
	RequestsPerMin   float64           // Rate limit: requests per minute (0 = unlimited)
	APIKeyHeader     string            // Header for API key (default: X-API-Key)
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
			if backend, exists := gw.modelRouter.GetBackend(modelName); exists {
				// Route to model-specific backend
				u, _ := url.Parse(backend.URL)
				targetURL = u
				// Strip the /models/{model-name} prefix from the path
				req.URL.Path = stripModelPrefixFromPath(req.URL.Path, modelName)
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

	// Record metrics (Phase 4)
	if gw.Metrics != nil {
		gw.Metrics.RecordRequestComplete(wrappedWriter.statusCode, r.Method, userID, modelName)
		gw.Metrics.RecordLatency(modelName, duration)
		// Note: Token recording would require parsing response body, handled separately if needed
	}

	// Log response
	if wrappedWriter.statusCode < 400 {
		gw.auditLogger.LogResponse(userID, modelName, r.RequestURI, correlationID, wrappedWriter.statusCode, int64(len(wrappedWriter.body)), 0, 0, duration)
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
	// Try to extract from URL path (e.g., /v1/models/llama-2-7b/completions)
	parts := strings.Split(r.URL.Path, "/")
	for i, part := range parts {
		if part == "models" && i+1 < len(parts) {
			return parts[i+1]
		}
	}

	// Try to extract from request body JSON (common in OpenAI-compatible APIs)
	if r.Method == "POST" && r.Body != nil {
		// Would need JSON parsing here, skipping for now
	}

	return "unknown"
}

func generateCorrelationID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().Nanosecond())
}

// extractModelNameFromPath extracts model name from paths like /models/{model-name}/v1/...
// Used for Phase 3 multi-model routing
func extractModelNameFromPath(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) > 1 && parts[0] == "models" {
		// Return the model name (next part after "models")
		return parts[1]
	}
	return ""
}

// stripModelPrefixFromPath removes the /models/{model-name} prefix from a path
// e.g., /models/mistral-7b/v1/chat/completions → /v1/chat/completions
func stripModelPrefixFromPath(path, modelName string) string {
	prefix := "/models/" + modelName
	if strings.HasPrefix(path, prefix) {
		// Return path without the prefix, ensuring it starts with /
		remainder := strings.TrimPrefix(path, prefix)
		if !strings.HasPrefix(remainder, "/") {
			remainder = "/" + remainder
		}
		return remainder
	}
	return path
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
