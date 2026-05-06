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
	"strings"
	"syscall"

	sovcrypto "github.com/gayanclife/sovereignstack/core/crypto"
	"github.com/gayanclife/sovereignstack/core/health"
	"github.com/gayanclife/sovereignstack/core/keys"
	"github.com/gayanclife/sovereignstack/core/logging"
	"github.com/gayanclife/sovereignstack/core/management/policy"
	"github.com/gayanclife/sovereignstack/core/tracing"
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
	policyCmd.Flags().String("admin-key", "", "Admin Bearer token (single-actor; reads SOVSTACK_ADMIN_KEY env). For per-actor attribution use --admin name=token (repeatable).")
	policyCmd.Flags().StringSlice("admin", nil, "Named admin token, repeatable: --admin alice=sk_xxx --admin bob=sk_yyy")
	policyCmd.Flags().String("master-key-file", "", "Path to AES-256 master key (auto-generated if missing). Enables Phase C5 field encryption.")
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

	tracerShutdown, err := tracing.Init(cmd.Context(), "sovstack-policy")
	if err != nil {
		log.Warn("tracing init failed; continuing without traces", "error", err)
	}
	defer func() { _ = tracerShutdown(cmd.Context()) }()

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

	// Phase C5: optional field encryption. If --master-key-file is set,
	// load (or auto-generate) the master key, configure the keystore
	// with it, and decrypt any already-encrypted profiles in place so
	// runtime reads see plaintext.
	if mkPath, _ := cmd.Flags().GetString("master-key-file"); mkPath != "" {
		mk, err := sovcrypto.LoadOrCreateMasterKey(mkPath)
		if err != nil {
			return fmt.Errorf("master key: %w", err)
		}
		store.SetMasterKey(mk)
		if store.HasEncryptedFields() {
			if err := store.DecryptProfilesInPlace(mk); err != nil {
				return fmt.Errorf("decrypt profiles: %w", err)
			}
			log.Info("master key loaded; encrypted fields decrypted in memory", "master_key_file", mkPath)
		} else {
			log.Info("master key loaded; future writes will be encrypted", "master_key_file", mkPath)
		}
	}

	mux := http.NewServeMux()
	svc := policy.New(store, adminKey)

	// Phase C4: parse named admin tokens from --admin name=token (repeatable)
	// and config (management.admin_keys map). CLI takes precedence.
	named := policy.NamedAdmins{}
	for k, v := range cfg.Management.AdminKeys {
		named[k] = v
	}
	if pairs, _ := cmd.Flags().GetStringSlice("admin"); len(pairs) > 0 {
		for _, p := range pairs {
			eq := strings.IndexByte(p, '=')
			if eq <= 0 {
				return fmt.Errorf("--admin must be name=token, got %q", p)
			}
			named[p[:eq]] = p[eq+1:]
		}
	}
	if len(named) > 0 {
		svc.NamedAdmins = named
		actors := make([]string, 0, len(named))
		for n := range named {
			actors = append(actors, n)
		}
		log.Info("named admins configured", "actors", actors)
	}

	// Phase C4: emit a structured audit log line on each successful admin
	// mutation. This goes through slog (which an operator may already be
	// shipping to a log aggregator); a richer audit DB sink is the
	// natural extension.
	svc.AdminAudit = func(actor, action string, r *http.Request) {
		log.Info("admin action",
			"actor", actor,
			"action", action,
			"ip", r.RemoteAddr,
			"path", r.URL.Path,
		)
	}

	// Phase F1: enable OIDC sign-in if the issuer is configured.
	if cfg.Management.OIDC.IssuerURL != "" {
		oidcCfg := policy.OIDCConfig{
			IssuerURL:    cfg.Management.OIDC.IssuerURL,
			ClientID:     cfg.Management.OIDC.ClientID,
			ClientSecret: cfg.Management.OIDC.ClientSecret,
			RedirectURL:  cfg.Management.OIDC.RedirectURL,
			AdminClaim:   cfg.Management.OIDC.AdminClaim,
		}
		if err := svc.EnableOIDC(cmd.Context(), oidcCfg); err != nil {
			return fmt.Errorf("oidc: %w", err)
		}
		log.Info("OIDC enabled",
			"issuer", oidcCfg.IssuerURL,
			"client_id", oidcCfg.ClientID,
			"admin_claim", oidcCfg.AdminClaim)
	}

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
