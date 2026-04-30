package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gayanclife/sovereignstack/core/keys"
	"github.com/gayanclife/sovereignstack/internal/docker"
	"github.com/spf13/cobra"
)

var managementCmd = &cobra.Command{
	Use:   "management",
	Short: "Start the management API server for system monitoring",
	Long: `Start the SovereignStack management API that provides:
- /api/models/running - List all running models
- /api/health - Health check
- /api/users - User management (requires admin key)
- /api/users/{id} - User profile management
- /api/users/{id}/models/{model} - Model access control
- /api/access/check - Check model access
- /api/models/{name}/metrics - Proxy vLLM Prometheus metrics for a specific model

This server is designed to be queried by the Visibility Platform (commercial monitoring solution).

Example:
  sovstack management --port 8888 --keys ~/.sovereignstack/keys.json`,
	RunE: runManagement,
}

func init() {
	managementCmd.Flags().Int("port", 8888, "Port for management API to listen on")
	managementCmd.Flags().String("keys", "", "Path to keys.json file (default: ~/.sovereignstack/keys.json)")
	managementCmd.Flags().String("admin-key", "", "Admin API key for user management operations")
	rootCmd.AddCommand(managementCmd)
}

// Global KeyStore (set in runManagement)
var keyStore *keys.KeyStore
var adminKey string

// ModelResponse represents a running model in the API response
type ModelResponse struct {
	Name        string `json:"name"`
	ContainerID string `json:"container_id"`
	Type        string `json:"type"` // "gpu" or "cpu"
	Status      string `json:"status"`
	Port        int    `json:"port"`
	StartedAt   string `json:"started_at,omitempty"`
}

// RunningModelsResponse is the API response for /api/models/running
type RunningModelsResponse struct {
	Version string           `json:"version"`
	Models  []ModelResponse  `json:"models"`
	Count   int              `json:"count"`
}

// HealthResponse is the API response for /api/health
type HealthResponse struct {
	Status string `json:"status"`
	Ready  bool   `json:"ready"`
}

func runManagement(cmd *cobra.Command, args []string) error {
	port, _ := cmd.Flags().GetInt("port")
	keysPath, _ := cmd.Flags().GetString("keys")
	adminKey, _ = cmd.Flags().GetString("admin-key")

	// Load KeyStore
	if keysPath == "" {
		home, _ := os.UserHomeDir()
		keysPath = filepath.Join(home, ".sovereignstack", "keys.json")
	}

	var err error
	keyStore, err = keys.LoadKeyStore(keysPath)
	if err != nil {
		return fmt.Errorf("failed to load keys: %w", err)
	}

	listenAddr := fmt.Sprintf(":%d", port)
	fmt.Printf("🔧 Starting SovereignStack Management API\n")
	fmt.Printf("  Listening: %s\n", listenAddr)
	fmt.Printf("  Keys file: %s\n", keysPath)
	fmt.Printf("  Endpoints:\n")
	fmt.Printf("    - GET /api/models/running\n")
	fmt.Printf("    - GET /api/health\n")
	fmt.Printf("    - GET /api/users\n")
	fmt.Printf("    - GET /api/users/{id}\n")
	fmt.Printf("    - POST /api/users/{id}/models/{model}\n")
	fmt.Printf("    - DELETE /api/users/{id}/models/{model}\n")
	fmt.Printf("    - PATCH /api/users/{id}/quota\n")
	fmt.Printf("    - GET /api/access/check?user={id}&model={model}\n")
	fmt.Printf("    - GET /api/models/{name}/metrics\n")
	fmt.Printf("\nPress Ctrl+C to stop\n\n")

	// Setup HTTP handlers
	http.HandleFunc("/api/models/running", handleRunningModels)
	http.HandleFunc("/api/models/", handleModelEndpoints)
	http.HandleFunc("/api/health", handleHealth)
	http.HandleFunc("/api/users", handleUsers)
	http.HandleFunc("/api/access/check", handleAccessCheck)

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
		return fmt.Errorf("management server error: %w", err)
	}

	return nil
}

func handleRunningModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	ctx := context.Background()
	runningModels, err := docker.GetRunningModels(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
		return
	}

	// Convert to response format
	models := make([]ModelResponse, 0, len(runningModels))
	for _, m := range runningModels {
		models = append(models, ModelResponse{
			Name:        m.ModelName,
			ContainerID: m.ContainerID[:12], // Short ID for readability
			Type:        m.Type,
			Status:      m.Status,
			Port:        m.Port,
		})
	}

	response := RunningModelsResponse{
		Version: "1.0",
		Models:  models,
		Count:   len(models),
	}

	json.NewEncoder(w).Encode(response)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := HealthResponse{
		Status: "ok",
		Ready:  true,
	}

	json.NewEncoder(w).Encode(response)
}

// vllmMetricsClient is a shared HTTP client for vLLM metrics scraping.
var vllmMetricsClient = &http.Client{Timeout: 5 * time.Second}

// handleModelEndpoints routes /api/models/{name}/... requests.
// Currently supports: /api/models/{name}/metrics
func handleModelEndpoints(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	// Expect: api/models/{name}/metrics
	if len(parts) < 4 {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}

	modelName := parts[2]
	subPath := parts[3]

	switch subPath {
	case "metrics":
		handleModelMetrics(w, r, modelName)
	default:
		http.Error(w, `{"error":"unknown endpoint"}`, http.StatusNotFound)
	}
}

// handleModelMetrics proxies the vLLM /metrics endpoint for a running model.
// GET /api/models/{name}/metrics → fetches http://localhost:{port}/metrics
func handleModelMetrics(w http.ResponseWriter, r *http.Request, modelName string) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Look up the running model to find its port
	ctx := context.Background()
	runningModels, err := docker.GetRunningModels(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to query running models: %v"}`, err), http.StatusInternalServerError)
		return
	}

	var port int
	var found bool
	for _, m := range runningModels {
		if m.ModelName == modelName && m.Status == "running" {
			port = m.Port
			found = true
			break
		}
	}

	if !found {
		http.Error(w, fmt.Sprintf(`{"error":"model %q is not running"}`, modelName), http.StatusNotFound)
		return
	}

	if port == 0 {
		http.Error(w, fmt.Sprintf(`{"error":"model %q has no exposed port"}`, modelName), http.StatusServiceUnavailable)
		return
	}

	// Proxy the request to vLLM's /metrics endpoint
	metricsURL := fmt.Sprintf("http://localhost:%d/metrics", port)
	resp, err := vllmMetricsClient.Get(metricsURL)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to fetch metrics: %v"}`, err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Forward content-type (Prometheus text format)
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/plain; version=0.0.4"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// checkAdminAuth verifies the admin API key.
func checkAdminAuth(r *http.Request) bool {
	if adminKey == "" {
		return true
	}
	auth := r.Header.Get("Authorization")
	expected := "Bearer " + adminKey
	return auth == expected
}

// handleUsers handles GET /api/users and user-specific operations.
func handleUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract user ID from path: /api/users/{id} or /api/users/{id}/models/{model}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}

	userID := parts[3]
	if userID == "" {
		// GET /api/users — list all users
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		if !checkAdminAuth(r) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		users := keyStore.ListUsers()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"users": users,
			"count": len(users),
		})
		return
	}

	user, _ := keyStore.GetByID(userID)
	if user == nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}

	// Check for model operations: /api/users/{id}/models/{model}
	if len(parts) >= 6 && parts[4] == "models" {
		model := parts[5]

		if !checkAdminAuth(r) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodPost:
			// Grant model access
			if err := keyStore.GrantModelAccess(userID, model); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "ok",
				"action": "granted",
				"model":  model,
			})

		case http.MethodDelete:
			// Revoke model access
			if err := keyStore.RevokeModelAccess(userID, model); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "ok",
				"action": "revoked",
				"model":  model,
			})

		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
		return
	}

	// Check for quota operations: /api/users/{id}/quota
	if len(parts) >= 5 && parts[4] == "quota" {
		if r.Method != http.MethodPatch {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		if !checkAdminAuth(r) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var req struct {
			MaxTokensPerDay   int64 `json:"max_tokens_per_day"`
			MaxTokensPerMonth int64 `json:"max_tokens_per_month"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}

		if err := keyStore.SetQuota(userID, req.MaxTokensPerDay, req.MaxTokensPerMonth); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":                "ok",
			"max_tokens_per_day":    req.MaxTokensPerDay,
			"max_tokens_per_month":  req.MaxTokensPerMonth,
		})
		return
	}

	// GET /api/users/{id} — get user profile
	if r.Method == http.MethodGet {
		json.NewEncoder(w).Encode(user)
		return
	}

	http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
}

// handleAccessCheck handles GET /api/access/check?user={id}&model={model}
func handleAccessCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	userID := r.URL.Query().Get("user")
	model := r.URL.Query().Get("model")

	if userID == "" || model == "" {
		http.Error(w, `{"error":"missing user or model parameter"}`, http.StatusBadRequest)
		return
	}

	allowed := keyStore.CanAccess(userID, model)
	statusCode := http.StatusOK
	if !allowed {
		statusCode = http.StatusForbidden
	}

	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user":    userID,
		"model":   model,
		"allowed": allowed,
	})
}
