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
	"github.com/gayanclife/sovereignstack/core/management/discovery"
	"github.com/gayanclife/sovereignstack/core/tracing"
	"github.com/spf13/cobra"
)

var discoveryCmd = &cobra.Command{
	Use:   "discovery",
	Short: "Run only the docker-discovery service (no auth, port 8889)",
	Long: `Run only the docker-discovery subset of the management API.

This is one of three split-out subservices from Phase E. It exposes:
  GET /api/v1/models/running   list Docker-discovered model containers
  GET /api/v1/health           legacy health (prefer /healthz)
  GET /healthz                 liveness
  GET /readyz                  readiness (Docker reachable)

Discovery is unauthenticated and stateless — it shells out to Docker
on demand. Run as a non-privileged user with access to the Docker socket.`,
	RunE: runDiscovery,
}

func init() {
	discoveryCmd.Flags().Int("port", 8889, "Port for discovery API to listen on")
	rootCmd.AddCommand(discoveryCmd)
}

func runDiscovery(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	if _, err := logging.Init(cfg.Log); err != nil {
		return err
	}
	log := logging.Service("discovery")

	tracerShutdown, err := tracing.Init(cmd.Context(), "sovstack-discovery")
	if err != nil {
		log.Warn("tracing init failed; continuing without traces", "error", err)
	}
	defer func() { _ = tracerShutdown(cmd.Context()) }()

	port, _ := cmd.Flags().GetInt("port")
	listenAddr := fmt.Sprintf(":%d", port)

	mux := http.NewServeMux()
	svc := discovery.New()
	svc.Register(mux)

	hc := health.New()
	mux.HandleFunc("/healthz", hc.LivenessHandler())
	mux.HandleFunc("/readyz", hc.ReadinessHandler())

	log.Info("discovery starting", "listen", listenAddr)

	server := &http.Server{Addr: listenAddr, Handler: mux}
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Info("shutting down gracefully")
		server.Close()
	}()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("discovery error: %w", err)
	}
	return nil
}
