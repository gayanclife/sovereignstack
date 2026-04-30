package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gayanclife/sovereignstack/core/audit"
	"github.com/gayanclife/sovereignstack/core/gateway"
	"github.com/gayanclife/sovereignstack/core/keys"
	"github.com/spf13/cobra"
)

var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Start the reverse proxy gateway with audit logging",
	Long: `Start the SovereignStack reverse proxy gateway that handles:
- Authentication (API key validation)
- Rate limiting per user
- Audit logging for compliance
- Request routing to backend vLLM service

Example:
  sovstack gateway --backend http://localhost:8000 --port 8001 --rate-limit 100`,
	RunE: runGateway,
}

func init() {
	gatewayCmd.Flags().String("backend", "http://localhost:8000", "Backend vLLM service URL")
	gatewayCmd.Flags().Int("port", 8001, "Port for gateway to listen on")
	gatewayCmd.Flags().Float64("rate-limit", 100, "Requests per minute per user (0 = unlimited)")
	gatewayCmd.Flags().String("api-key-header", "X-API-Key", "Header name for API key")
	gatewayCmd.Flags().Int("audit-buffer", 10000, "Number of audit logs to keep in memory (only for in-memory logger)")
	gatewayCmd.Flags().String("audit-db", "./sovstack-audit.db", "Path to SQLite audit database (empty = in-memory only)")
	gatewayCmd.Flags().String("audit-key", "", "Encryption key for audit logs (reads SOVSTACK_AUDIT_KEY env var if not set)")
	gatewayCmd.Flags().String("keys", "", "Path to keys.json file (empty = use hardcoded test keys)")
	gatewayCmd.Flags().String("management-url", "http://localhost:8888", "Management service URL for model discovery (Phase 3)")
	rootCmd.AddCommand(gatewayCmd)
}

func runGateway(cmd *cobra.Command, args []string) error {
	backend, _ := cmd.Flags().GetString("backend")
	port, _ := cmd.Flags().GetInt("port")
	rateLimit, _ := cmd.Flags().GetFloat64("rate-limit")
	apiKeyHeader, _ := cmd.Flags().GetString("api-key-header")
	auditBuffer, _ := cmd.Flags().GetInt("audit-buffer")
	auditDB, _ := cmd.Flags().GetString("audit-db")
	auditKey, _ := cmd.Flags().GetString("audit-key")
	keysPath, _ := cmd.Flags().GetString("keys")
	managementURL, _ := cmd.Flags().GetString("management-url")

	// Resolve encryption key
	if auditKey == "" {
		auditKey = os.Getenv("SOVSTACK_AUDIT_KEY")
	}
	if auditKey == "" && auditDB != "" {
		// Generate a new key and print it for the user
		keyBytes := make([]byte, 32)
		rand.Read(keyBytes)
		auditKey = hex.EncodeToString(keyBytes)
		fmt.Printf("\n⚠️  Generated audit encryption key (save this for future restarts):\n")
		fmt.Printf("   SOVSTACK_AUDIT_KEY=%s\n\n", auditKey)
	}

	// Create audit logger (SQLite or in-memory)
	var auditLogger audit.AuditLogger
	var err error

	if auditDB != "" {
		auditLogger, err = audit.NewSQLiteLogger(auditDB, auditKey)
		if err != nil {
			return fmt.Errorf("failed to create SQLite audit logger: %w", err)
		}
	} else {
		auditLogger = audit.NewLogger(auditBuffer)
	}

	// Load keys from file or use hardcoded test keys
	var authProvider gateway.AuthProvider
	var keyStore *keys.KeyStore
	usingKeyStore := false

	if keysPath != "" {
		ks, err := keys.LoadKeyStore(keysPath)
		if err != nil {
			return fmt.Errorf("failed to load keys from %s: %w", keysPath, err)
		}
		keyStore = ks
		authProvider = &gatewayAuthAdapter{ks}
		usingKeyStore = true
	} else {
		// Use hardcoded test keys for backward compatibility
		auth := gateway.NewAPIKeyAuthProvider()
		auth.AddKey("sk_test_123", "test-user")
		auth.AddKey("sk_demo_456", "demo-user")
		authProvider = auth
	}

	// Create gateway
	var accessController gateway.AccessController
	var quotaManager *gateway.TokenQuotaManager
	var modelRouter *gateway.ModelRouter
	if usingKeyStore {
		// Wire up access controller (Phase 2) when using KeyStore
		accessController = gateway.NewKeyStoreAccessController(keyStore)
		// Wire up quota manager (Phase 2b) when using KeyStore
		quotaManager = gateway.NewTokenQuotaManager(keyStore)
	}

	// Create model router (Phase 3) for multi-model routing
	modelRouter = gateway.NewModelRouter(managementURL)
	modelRouter.StartDiscovery()

	gw, err := gateway.NewGateway(gateway.GatewayConfig{
		TargetURL:        backend,
		AuthProvider:     authProvider,
		AccessController: accessController,
		QuotaManager:     quotaManager,
		ModelRouter:      modelRouter,
		AuditLogger:      auditLogger,
		RequestsPerMin:   rateLimit,
		APIKeyHeader:     apiKeyHeader,
	})
	if err != nil {
		return fmt.Errorf("failed to create gateway: %w", err)
	}

	listenAddr := fmt.Sprintf(":%d", port)
	fmt.Printf("🚀 Starting SovereignStack Gateway\n")
	fmt.Printf("  Backend: %s\n", backend)
	fmt.Printf("  Listening: %s\n", listenAddr)
	fmt.Printf("  Rate Limit: %.0f req/min per user\n", rateLimit)
	fmt.Printf("  API Key Header: %s\n", apiKeyHeader)
	if auditDB != "" {
		fmt.Printf("  Audit Log: SQLite (encrypted) at %s\n", auditDB)
	} else {
		fmt.Printf("  Audit Log: In-memory (%d logs max)\n", auditBuffer)
	}

	if usingKeyStore {
		fmt.Printf("  Keys: Loaded from %s\n", keysPath)
		users := keyStore.ListUsers()
		fmt.Printf("  Users: %d registered\n", len(users))
		fmt.Printf("  Access Control: Enabled (Phase 2)\n")
		fmt.Printf("  Token Quotas: Enabled (Phase 2b)\n")
	} else {
		fmt.Printf("\nExample test keys (hardcoded for development):\n")
		fmt.Printf("  - sk_test_123 (test-user)\n")
		fmt.Printf("  - sk_demo_456 (demo-user)\n")
		fmt.Printf("\n  To use keys.json, run: sovstack gateway --keys ~/.sovereignstack/keys.json\n")
	}

	// Show model routing status
	fmt.Printf("  Model Router: Enabled (Phase 3, polling %s every 30s)\n", managementURL)
	fmt.Printf("  Registered Models: %d\n", modelRouter.GetModelCount())
	fmt.Printf("  Metrics: Enabled (Phase 4, Prometheus format)\n")

	fmt.Printf("\nUsage:\n")
	fmt.Printf("  curl -H 'X-API-Key: sk_test_123' http://localhost:%d/v1/models\n", port)
	fmt.Printf("  curl -H 'X-API-Key: sk_test_123' http://localhost:%d/models/mistral-7b/v1/chat/completions\n", port)
	fmt.Printf("\nView metrics (Prometheus format):\n")
	fmt.Printf("  curl http://localhost:%d/metrics\n", port)
	fmt.Printf("\nView audit logs:\n")
	fmt.Printf("  curl http://localhost:%d/api/audit/logs\n", port)
	fmt.Printf("\nPress Ctrl+C to stop\n\n")

	// Create metrics tracker (Phase 4)
	metrics := gateway.NewGatewayMetrics()
	gw.Metrics = metrics

	// Add audit endpoints
	http.HandleFunc("/api/audit/logs", func(w http.ResponseWriter, r *http.Request) {
		handleAuditLogs(w, r, auditLogger)
	})

	http.HandleFunc("/api/audit/stats", func(w http.ResponseWriter, r *http.Request) {
		handleAuditStats(w, r, auditLogger)
	})

	// Add metrics endpoint (Phase 4)
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Write([]byte(metrics.WritePrometheusText()))
	})

	// Main gateway handler (all other requests)
	http.Handle("/", gw)

	// Setup graceful shutdown
	server := &http.Server{Addr: listenAddr}
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		fmt.Printf("\n\n✓ Shutting down gracefully...\n")
		modelRouter.Stop() // Stop model discovery
		server.Close()
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("gateway error: %w", err)
	}

	return nil
}

func handleAuditLogs(w http.ResponseWriter, r *http.Request, logger audit.AuditLogger) {
	w.Header().Set("Content-Type", "application/json")

	n := 100 // Default: last 100 logs
	if nStr := r.URL.Query().Get("n"); nStr != "" {
		fmt.Sscanf(nStr, "%d", &n)
	}

	logs := logger.GetLogs(n)

	// Simple JSON response
	fmt.Fprintf(w, `{"logs": [`)
	for i, log := range logs {
		fmt.Fprintf(w, `{
			"timestamp": "%s",
			"event_type": "%s",
			"user": "%s",
			"model": "%s",
			"endpoint": "%s",
			"status_code": %d,
			"tokens_used": %d,
			"duration_ms": %d
		}`,
			log.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
			log.EventType,
			log.User,
			log.Model,
			log.Endpoint,
			log.StatusCode,
			log.TokensUsed,
			log.DurationMS,
		)

		if i < len(logs)-1 {
			fmt.Fprintf(w, ",")
		}
	}
	fmt.Fprintf(w, `]}`)
}

func handleAuditStats(w http.ResponseWriter, r *http.Request, logger audit.AuditLogger) {
	w.Header().Set("Content-Type", "application/json")
	stats := logger.GetStats()

	fmt.Fprintf(w, `{`)
	fmt.Fprintf(w, `"total_logs": %v,`, stats["total_logs"])
	fmt.Fprintf(w, `"total_requests": %v,`, stats["total_requests"])
	fmt.Fprintf(w, `"total_errors": %v,`, stats["total_errors"])
	fmt.Fprintf(w, `"total_tokens_used": %v,`, stats["total_tokens_used"])
	fmt.Fprintf(w, `"unique_users": %v,`, stats["unique_users"])
	fmt.Fprintf(w, `"unique_models": %v`, stats["unique_models"])
	fmt.Fprintf(w, `}`)
}

// gatewayAuthAdapter adapts KeyStore to the gateway.AuthProvider interface
type gatewayAuthAdapter struct {
	store *keys.KeyStore
}

func (a *gatewayAuthAdapter) ValidateToken(token string) (string, error) {
	user, _ := a.store.GetByKey(token)
	if user == nil {
		return "", fmt.Errorf("invalid API key")
	}
	return user.ID, nil
}

func (a *gatewayAuthAdapter) AddKey(apiKey, userID string) {
	// Not used when loading from KeyStore
}

func (a *gatewayAuthAdapter) RemoveKey(apiKey string) {
	// Not used when loading from KeyStore
}
