package cmd

import (
	"fmt"
	"os"

	"github.com/gayanclife/sovereignstack/core/config"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sovstack",
	Short: "SovereignStack CLI for deploying private LLM inference servers",
	Long: `SovereignStack is a CLI tool that automates the deployment of private,
production-grade LLM inference servers on bare metal or VPS. It provides a
one-command experience for setting up secure, local AI inference.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().String("config", "", "Path to YAML config file (e.g. ~/.sovereignstack/sovstack.yaml)")
}

// loadConfig is a helper for subcommands. It reads --config from the cobra
// command (a persistent flag on rootCmd), invokes config.Load, and returns
// the result. Subcommands should call this once at the top of their RunE
// and then apply per-flag overrides on top of the returned struct.
func loadConfig(cmd *cobra.Command) (*config.Config, error) {
	path, _ := cmd.Flags().GetString("config")
	cfg, err := config.Load(path)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}
