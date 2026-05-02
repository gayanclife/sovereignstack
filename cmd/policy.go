// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gayanclife/sovereignstack/core/health"
	"github.com/gayanclife/sovereignstack/core/keys"
	"github.com/gayanclife/sovereignstack/core/logging"
	"github.com/gayanclife/sovereignstack/core/management/policy"
	"github.com/spf13/cobra"
)

var policyCmd = &cobra.Command{
	Use:   "policy",
	Short: "Run only the user-policy service (admin auth, port 8888)",
	Long: `Run only the user-policy subset of the management API.

This is one of three split-out subservices from Phase E. It owns the
keys.json file and exposes:
  GET    /api/v1/users                     (admin)
  GET    /api/v1/users/{id}                (no auth)
  POST   /api/v1/users/{id}/models/{model} (admin)
  DELETE /api/v1/users/{id}/models/{model} (admin)
  PATCH  /api/v1/users/{id}/quota          (admin)
  GET    /api/v1/access/check              (no auth)

Policy is the only subservice that needs filesystem access to keys.json.
In production it should run as a separate user from discovery and
metrics-proxy so a compromise of those services cannot mutate auth state.`,
	RunE: runPolicy,
}

func init() {
	policyCmd.Flags().Int("port", 8888, "Port for policy API to listen on")
	policyCmd.Flags().String("keys", "", "Path to keys.json (default: ~/.sovereignstack/keys.json)")
	policyCmd.Flags().String("admin-key", "", "Admin Bearer token (also reads SOVSTACK_ADMIN_KEY env)")
	rootCmd.AddCommand(policyCmd)
}

func runPolicy(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	if _, err := logging.Init(cfg.Log); err != nil {
		return err
	}
	log := logging.Service("policy")

	port, _ := cmd.Flags().GetInt("port")
	keysPath, _ := cmd.Flags().GetString("keys")
	if keysPath == "" {
		keysPath = cfg.Management.KeysFile
	}
	if keysPath == "" {
		home, _ := os.UserHomeDir()
		keysPath = filepath.Join(home, ".sovereignstack", "keys.json")
	}

	adminKey, _ := cmd.Flags().GetString("admin-key")
	if adminKey == "" {
		adminKey = cfg.Management.AdminKey
	}
	if adminKey == "" {
		adminKey = os.Getenv("SOVSTACK_ADMIN_KEY")
	}

	store, err := keys.LoadKeyStore(keysPath)
	if err != nil {
		return fmt.Errorf("load keys: %w", err)
	}

	mux := http.NewServeMux()
	svc := policy.New(store, adminKey)
	svc.Register(mux)

	hc := health.New()
	hc.Register("keystore", func(ctx context.Context) error {
		if store == nil {
			return errors.New("keystore not loaded")
		}
		return nil
	})
	mux.HandleFunc("/healthz", hc.LivenessHandler())
	mux.HandleFunc("/readyz", hc.ReadinessHandler())

	listenAddr := fmt.Sprintf(":%d", port)
	log.Info("policy starting",
		"listen", listenAddr,
		"keys_file", keysPath,
		"admin_auth_enabled", adminKey != "",
	)

	server := &http.Server{Addr: listenAddr, Handler: mux}
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Info("shutting down gracefully")
		server.Close()
	}()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("policy error: %w", err)
	}
	return nil
}
