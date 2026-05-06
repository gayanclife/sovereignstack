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
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gayanclife/sovereignstack/core/engine"
	"github.com/spf13/cobra"
)

// stopCmd represents the stop command
var stopCmd = &cobra.Command{
	Use:   "stop [model-name]",
	Short: "Stop a running model",
	Long: `Stop a specific running model and clean up its Docker container.

Usage:
  sovstack stop <model-name>              # Stop a specific model
  sovstack stop --all                     # Stop all running models (requires confirmation)

Examples:
  sovstack stop mistralai/Mistral-7B-v0.3
  sovstack stop distilbert-base-uncased
  sovstack stop --all`,
	RunE: runStop,
	Args: cobra.MaximumNArgs(1),
}

func runStop(cmd *cobra.Command, args []string) error {
	stopAll, _ := cmd.Flags().GetBool("all")

	er, err := engine.NewEngineRoom(engine.EngineConfig{
		ModelCacheDir: "./models",
		Port:          8000,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize engine: %w", err)
	}

	ctx := context.Background()

	// Handle --all flag
	if stopAll {
		running := er.GetRunningModels()
		if len(running) == 0 {
			fmt.Println("No running models to stop")
			return nil
		}

		fmt.Printf("Running models:\n")
		for modelName := range running {
			fmt.Printf("  • %s\n", modelName)
		}

		fmt.Printf("\n⚠️  This will stop %d model(s). Continue? [y/N]: ", len(running))
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			fmt.Println("Cancelled")
			return nil
		}

		fmt.Printf("\nStopping all %d running model(s)...\n\n", len(running))
		failCount := 0
		for modelName := range running {
			if err := er.StopModel(ctx, modelName); err != nil {
				fmt.Printf("✗ Failed to stop %s: %v\n", modelName, err)
				failCount++
			} else {
				fmt.Printf("✓ Stopped %s\n", modelName)
			}
		}

		if failCount > 0 {
			return fmt.Errorf("failed to stop %d model(s)", failCount)
		}
		return nil
	}

	// Require model name if --all not specified
	if len(args) == 0 {
		return fmt.Errorf("must specify a model name or use --all flag\n\nUsage:\n  sovstack stop <model-name>\n  sovstack stop --all")
	}

	// Stop specific model
	modelName := args[0]
	if err := er.StopModel(ctx, modelName); err != nil {
		fmt.Printf("✗ Failed to stop %s: %v\n", modelName, err)
		return err
	}

	fmt.Printf("✓ Model %s stopped\n", modelName)
	return nil
}

func init() {
	stopCmd.Flags().BoolP("all", "a", false, "Stop all running models (requires confirmation)")
	rootCmd.AddCommand(stopCmd)
}
