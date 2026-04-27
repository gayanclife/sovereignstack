/*
Copyright 2026 SovereignStack Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"

	"github.com/gayanclife/sovereignstack/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage SovereignStack configuration",
	Long:  `Configure cache directory, API tokens, and other settings`,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		value := args[1]

		configMgr, err := config.NewManager()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		auditor := config.NewAuditLogger(configMgr.GetConfigDir())

		switch key {
		case "cache-dir":
			if err := configMgr.SetCacheDir(value); err != nil {
				auditor.LogConfigChange(key, "failed")
				return fmt.Errorf("failed to set cache-dir: %w", err)
			}
			auditor.LogConfigChange(key, "success")
			fmt.Printf("✓ Cache directory set to: %s\n", value)

		case "log-dir":
			if err := configMgr.SetLogDir(value); err != nil {
				auditor.LogConfigChange(key, "failed")
				return fmt.Errorf("failed to set log-dir: %w", err)
			}
			auditor.LogConfigChange(key, "success")
			fmt.Printf("✓ Log directory set to: %s\n", value)

		case "hf-token":
			if err := configMgr.SetHFToken(value); err != nil {
				auditor.LogConfigChange(key, "failed")
				return fmt.Errorf("failed to set hf-token: %w", err)
			}
			auditor.LogConfigChange(key, "success")
			fmt.Printf("✓ HF token configured (encrypted)\n")

		default:
			return fmt.Errorf("unknown config key: %s", key)
		}

		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]

		configMgr, err := config.NewManager()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		switch key {
		case "cache-dir":
			fmt.Printf("%s\n", configMgr.GetCacheDir())

		case "log-dir":
			fmt.Printf("%s\n", configMgr.GetLogDir())

		case "hf-token":
			token := configMgr.GetHFToken()
			if token == "" {
				fmt.Println("(not set)")
			} else {
				fmt.Println("(set, encrypted)")
			}

		default:
			return fmt.Errorf("unknown config key: %s", key)
		}

		return nil
	},
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration values",
	RunE: func(cmd *cobra.Command, args []string) error {
		configMgr, err := config.NewManager()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		fmt.Println("SovereignStack Configuration:")
		fmt.Println()
		fmt.Printf("  cache-dir:      %s\n", configMgr.GetCacheDir())
		fmt.Printf("  log-dir:        %s\n", configMgr.GetLogDir())
		token := configMgr.GetHFToken()
		if token == "" {
			fmt.Printf("  hf-token:       (not set)\n")
		} else {
			fmt.Printf("  hf-token:       (set, encrypted)\n")
		}
		fmt.Println()
		fmt.Printf("  Config file:    %s/config.json\n", configMgr.GetConfigDir())
		fmt.Printf("  Audit log:      %s/audit.log\n", configMgr.GetLogDir())

		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configListCmd)
}
