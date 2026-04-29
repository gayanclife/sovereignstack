package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gayanclife/sovereignstack/internal/docker"
	"github.com/spf13/cobra"
)

var managementCmd = &cobra.Command{
	Use:   "management",
	Short: "Start the management API server for system monitoring",
	Long: `Start the SovereignStack management API that provides:
- /api/models/running - List all running models
- /api/system/info - System hardware information
- /api/health - Health check

This server is designed to be queried by the Visibility Platform (commercial monitoring solution).

Example:
  sovstack management --port 8888`,
	RunE: runManagement,
}

func init() {
	managementCmd.Flags().Int("port", 8888, "Port for management API to listen on")
	rootCmd.AddCommand(managementCmd)
}

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

	listenAddr := fmt.Sprintf(":%d", port)
	fmt.Printf("🔧 Starting SovereignStack Management API\n")
	fmt.Printf("  Listening: %s\n", listenAddr)
	fmt.Printf("  Endpoints:\n")
	fmt.Printf("    - GET /api/models/running\n")
	fmt.Printf("    - GET /api/health\n")
	fmt.Printf("\nPress Ctrl+C to stop\n\n")

	// Setup HTTP handlers
	http.HandleFunc("/api/models/running", handleRunningModels)
	http.HandleFunc("/api/health", handleHealth)

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
