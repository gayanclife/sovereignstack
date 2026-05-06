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
	"os/exec"
	"strconv"
	"strings"

	"github.com/gayanclife/sovereignstack/internal/docker"
	"github.com/spf13/cobra"
)

// logsCmd represents the logs command
var logsCmd = &cobra.Command{
	Use:   "logs [model-name]",
	Short: "View logs from a running model container",
	Long: `Display logs from a deployed model's Docker container.

If no model name is specified, shows logs from all running models.

Examples:
  sovstack logs distilbert-base-uncased    # Logs from specific model
  sovstack logs                             # Show all running models
  sovstack logs -f distilbert-base-uncased # Follow logs in real-time
  sovstack logs -n 50 distilbert-base-uncased # Show last 50 lines`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLogs,
}

func init() {
	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output (like tail -f)")
	logsCmd.Flags().IntP("lines", "n", 100, "Number of recent log lines to show (default 100)")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	follow, _ := cmd.Flags().GetBool("follow")
	lines, _ := cmd.Flags().GetInt("lines")

	ctx := context.Background()

	// Get running models
	runningModels, err := docker.GetRunningModels(ctx)
	if err != nil {
		return fmt.Errorf("failed to query Docker: %w", err)
	}

	if len(runningModels) == 0 {
		fmt.Println("No running models found")
		return nil
	}

	// If no model specified, show which models are running and ask user to pick one
	var targetModel string
	if len(args) == 0 {
		if len(runningModels) == 0 {
			fmt.Println("No running models found")
			return nil
		}

		if len(runningModels) == 1 {
			// Only one model running, use it
			targetModel = runningModels[0].ContainerID[:12]
			fmt.Printf("Showing logs for: %s\n\n", runningModels[0].ModelName)
		} else {
			// Multiple models, let user pick
			fmt.Println("🚀 Running Models:")
			fmt.Println()
			for i, m := range runningModels {
				fmt.Printf("%d. %s\n", i+1, m.ModelName)
				fmt.Printf("   ID: %s | Status: %s\n", m.ContainerID[:12], m.Status)
				fmt.Println()
			}

			reader := bufio.NewReader(os.Stdin)
			for {
				fmt.Print("Select model number (1-" + fmt.Sprintf("%d", len(runningModels)) + "): ")
				input, _ := reader.ReadString('\n')
				input = strings.TrimSpace(input)

				num, err := strconv.Atoi(input)
				if err != nil || num < 1 || num > len(runningModels) {
					fmt.Printf("Invalid selection. Please enter a number between 1 and %d\n", len(runningModels))
					continue
				}

				targetModel = runningModels[num-1].ContainerID[:12]
				fmt.Printf("\nShowing logs for: %s\n\n", runningModels[num-1].ModelName)
				break
			}
		}
	} else {
		// Find model by name
		modelName := args[0]
		found := false
		for _, m := range runningModels {
			if strings.Contains(m.ModelName, modelName) || strings.Contains(m.ContainerID, modelName) {
				targetModel = m.ContainerID[:12]
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("Model '%s' not found. Running models:\n", modelName)
			for _, m := range runningModels {
				fmt.Printf("  - %s (ID: %s)\n", m.ModelName, m.ContainerID[:12])
			}
			return fmt.Errorf("model not found")
		}
	}

	// Build docker logs command
	dockerArgs := []string{"logs"}
	if follow {
		dockerArgs = append(dockerArgs, "-f")
	}
	dockerArgs = append(dockerArgs, "--tail", fmt.Sprintf("%d", lines))
	dockerArgs = append(dockerArgs, targetModel)

	// Execute docker logs
	dockerCmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr
	dockerCmd.Stdin = os.Stdin

	if err := dockerCmd.Run(); err != nil {
		// Don't return error for docker logs failures (e.g., user interrupted with Ctrl+C)
		if ctx.Err() == nil {
			return fmt.Errorf("failed to get logs: %w", err)
		}
	}

	return nil
}
