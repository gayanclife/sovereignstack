package cmd

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gayanclife/sovereignstack/core/audit"
	"github.com/gayanclife/sovereignstack/core/gateway"
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
	gatewayCmd.Flags().Int("audit-buffer", 10000, "Number of audit logs to keep in memory")
	rootCmd.AddCommand(gatewayCmd)
}

func runGateway(cmd *cobra.Command, args []string) error {
	backend, _ := cmd.Flags().GetString("backend")
	port, _ := cmd.Flags().GetInt("port")
	rateLimit, _ := cmd.Flags().GetFloat64("rate-limit")
	apiKeyHeader, _ := cmd.Flags().GetString("api-key-header")
	auditBuffer, _ := cmd.Flags().GetInt("audit-buffer")

	// Create audit logger
	auditLogger := audit.NewLogger(auditBuffer)

	// Create auth provider with some example keys
	authProvider := gateway.NewAPIKeyAuthProvider()
	authProvider.AddKey("sk_test_123", "test-user")
	authProvider.AddKey("sk_demo_456", "demo-user")

	// Create gateway
	gw, err := gateway.NewGateway(gateway.GatewayConfig{
		TargetURL:      backend,
		AuthProvider:   authProvider,
		AuditLogger:    auditLogger,
		RequestsPerMin: rateLimit,
		APIKeyHeader:   apiKeyHeader,
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
	fmt.Printf("  Audit Buffer: %d logs\n", auditBuffer)
	fmt.Printf("\nExample test keys:\n")
	fmt.Printf("  - sk_test_123 (test-user)\n")
	fmt.Printf("  - sk_demo_456 (demo-user)\n")
	fmt.Printf("\nUsage:\n")
	fmt.Printf("  curl -H 'X-API-Key: sk_test_123' http://localhost:%d/v1/models\n", port)
	fmt.Printf("\nView audit logs:\n")
	fmt.Printf("  curl http://localhost:%d/api/audit/logs\n", port)
	fmt.Printf("\nPress Ctrl+C to stop\n\n")

	// Add audit endpoints
	http.HandleFunc("/api/audit/logs", func(w http.ResponseWriter, r *http.Request) {
		handleAuditLogs(w, r, auditLogger)
	})

	http.HandleFunc("/api/audit/stats", func(w http.ResponseWriter, r *http.Request) {
		handleAuditStats(w, r, auditLogger)
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
		server.Close()
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("gateway error: %w", err)
	}

	return nil
}

func handleAuditLogs(w http.ResponseWriter, r *http.Request, logger *audit.Logger) {
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

func handleAuditStats(w http.ResponseWriter, r *http.Request, logger *audit.Logger) {
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
