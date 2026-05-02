// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package cmd

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gayanclife/sovereignstack/core/health"
	"github.com/gayanclife/sovereignstack/core/logging"
	"github.com/gayanclife/sovereignstack/core/management/metricsproxy"
	"github.com/spf13/cobra"
)

var metricsProxyCmd = &cobra.Command{
	Use:   "metrics-proxy",
	Short: "Run only the vLLM metrics-proxy service (no auth, port 8890)",
	Long: `Run only the vLLM metrics-proxy subset of the management API.

This is one of three split-out subservices from Phase E. It exposes:
  GET /api/v1/models/{name}/metrics   forward to vLLM's own /metrics

Metrics-proxy is unauthenticated and stateless — it shells out to Docker
to resolve {name} → host port, then proxies to localhost:{port}/metrics.
Run as a non-privileged user.`,
	RunE: runMetricsProxy,
}

func init() {
	metricsProxyCmd.Flags().Int("port", 8890, "Port for metrics-proxy to listen on")
	rootCmd.AddCommand(metricsProxyCmd)
}

func runMetricsProxy(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	if _, err := logging.Init(cfg.Log); err != nil {
		return err
	}
	log := logging.Service("metrics-proxy")

	port, _ := cmd.Flags().GetInt("port")
	listenAddr := fmt.Sprintf(":%d", port)

	mux := http.NewServeMux()
	svc := metricsproxy.New()
	svc.Register(mux)

	hc := health.New()
	mux.HandleFunc("/healthz", hc.LivenessHandler())
	mux.HandleFunc("/readyz", hc.ReadinessHandler())

	log.Info("metrics-proxy starting", "listen", listenAddr)

	server := &http.Server{Addr: listenAddr, Handler: mux}
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Info("shutting down gracefully")
		server.Close()
	}()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("metrics-proxy error: %w", err)
	}
	return nil
}
