package cmd

import (
	"context"
	"fmt"

	"github.com/gayanclife/sovereignstack/core/model"
	"github.com/gayanclife/sovereignstack/internal/config"
	"github.com/gayanclife/sovereignstack/internal/docker"
	"github.com/gayanclife/sovereignstack/internal/hardware"
	"github.com/spf13/cobra"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show system status and cached models",
	Long: `Display the current status of SovereignStack.

By default shows: hardware, running models, and cached models.

Use flags to show only specific sections for cleaner output when you have many models.

Examples:
  sovstack status                 # Full status (hardware + running + cached)
  sovstack status --running       # Only running models
  sovstack status --cached        # Only cached models
  sovstack status --hardware      # Only hardware info
  sovstack status --running --cached  # Running and cached (skip hardware)`,
	RunE: runStatus,
}

var verifyCmd = &cobra.Command{
	Use:   "verify [model-name]",
	Short: "Verify a specific model is ready to deploy",
	Long:  `Verify that a model has been completely downloaded and is ready for deployment`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runVerify,
}

func init() {
	statusCmd.Flags().BoolP("detailed", "d", false, "Show detailed file listing for cached models")
	statusCmd.Flags().BoolP("json", "j", false, "Output as JSON")
	statusCmd.Flags().Bool("running", false, "Show only running models")
	statusCmd.Flags().Bool("cached", false, "Show only cached models")
	statusCmd.Flags().Bool("hardware", false, "Show only hardware info")
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(verifyCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	detailed, _ := cmd.Flags().GetBool("detailed")
	showRunning, _ := cmd.Flags().GetBool("running")
	showCached, _ := cmd.Flags().GetBool("cached")
	showHardware, _ := cmd.Flags().GetBool("hardware")

	// If no specific sections requested, show all
	showAll := !showRunning && !showCached && !showHardware
	if showAll {
		showRunning = true
		showCached = true
		showHardware = true
	}

	// Load config
	configMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Printf("📊 SovereignStack Status\n")
	fmt.Printf("═══════════════════════════════════════════\n\n")

	// Show running models
	if showRunning {
		ctx := context.Background()
		runningModels, err := docker.GetRunningModels(ctx)
		if err == nil && len(runningModels) > 0 {
			fmt.Printf("🚀 Running Models (%d)\n", len(runningModels))
			fmt.Printf("──────────────────────────────────────────\n")
			for _, model := range runningModels {
				fmt.Printf("  • %s\n", model.ModelName)
				fmt.Printf("    Type: %s | Status: %s\n", model.Type, model.Status)
				fmt.Printf("    Container: %s\n", model.ContainerID[:12])
				if model.Port > 0 {
					fmt.Printf("    Port: %d (http://localhost:%d)\n", model.Port, model.Port)
					fmt.Printf("    API: http://localhost:%d/v1/chat/completions\n", model.Port)
				} else {
					fmt.Printf("    Port: (not exposed)\n")
				}
				fmt.Printf("\n")
			}
		} else {
			fmt.Printf("🚀 Running Models: None\n\n")
		}
	}

	// Show hardware information
	if showHardware {
		fmt.Printf("🖥️  Hardware\n")
		fmt.Printf("──────────────────────────────────────────\n")
		hw, err := hardware.GetSystemHardware()
		if err == nil && hw != nil {
			if len(hw.GPUs) > 0 {
				totalVRAM := int64(0)
				for _, gpu := range hw.GPUs {
					totalVRAM += gpu.VRAM
				}
				fmt.Printf("  GPUs: %d x %s (%.1f GB total)\n", len(hw.GPUs), hw.GPUs[0].Name, float64(totalVRAM)/(1024*1024*1024))
			} else {
				fmt.Printf("  GPUs: None (CPU-only system)\n")
			}
			fmt.Printf("  CPU: %d cores\n", hw.CPUCores)
			fmt.Printf("  RAM: %.1f GB\n", float64(hw.SystemRAM)/(1024*1024*1024))
			if hw.CUDAInstalled {
				fmt.Printf("  CUDA: %s\n", hw.CUDAVersion)
			} else {
				fmt.Printf("  CUDA: Not installed\n")
			}
			if hw.DockerInstalled {
				fmt.Printf("  Docker: ✓ installed\n")
			} else {
				fmt.Printf("  Docker: ✗ not installed\n")
			}
		} else {
			fmt.Printf("  Unable to detect hardware\n")
		}
		fmt.Printf("\n")
	}

	// Show cached models
	if showCached {
		// Create cache manager
		cm, err := model.NewCacheManager("")
		if err != nil {
			return fmt.Errorf("failed to create cache manager: %w", err)
		}

		// List and verify all models
		cached := cm.ListCached()

		if len(cached) == 0 {
			fmt.Printf("💾 Cached Models: None\n\n")
			if showAll {
				fmt.Printf("Get started:\n")
				fmt.Printf("  sovstack pull distilbert-base-uncased   - Download a small model\n")
				fmt.Printf("  sovstack deploy distilbert-base-uncased - Deploy the model\n")
			}
		} else {
			fmt.Printf("💾 Cached Models (%d)\n", len(cached))
			fmt.Printf("──────────────────────────────────────────\n")

			readyCount := 0
			totalSize := int64(0)

			for i, meta := range cached {
				// Verify the model
				result := cm.VerifyModel(meta.Name)
				totalSize += result.TotalSize

				statusIcon := "✓"
				statusStr := "Ready to deploy"
				if !result.ReadyToDeploy {
					statusIcon = "✗"
					statusStr = result.Status
				} else {
					readyCount++
				}

				fmt.Printf("%d. %s [%s %s]\n", i+1, meta.Name, statusIcon, statusStr)
				fmt.Printf("   Size: %.2f MB (%d files)\n", float64(result.TotalSize)/(1024*1024), result.FileCount)
				fmt.Printf("   Location: %s\n", meta.Path)
				fmt.Printf("   Cached: %s\n", meta.Downloaded.Format("2006-01-02 15:04:05"))

				if detailed {
					fmt.Printf("   Files:\n")
					for _, f := range result.Files {
						fmt.Printf("     - %s (%.2f MB)\n", f.Name, float64(f.Size)/(1024*1024))
					}
				}

				if len(result.Warnings) > 0 {
					for _, w := range result.Warnings {
						fmt.Printf("   ⚠ Warning: %s\n", w)
					}
				}

				fmt.Printf("\n")
			}

			// Show summary
			fmt.Printf("📈 Cache Statistics\n")
			fmt.Printf("──────────────────────────────────────────\n")
			fmt.Printf("Total Models: %d\n", len(cached))
			fmt.Printf("Ready to Deploy: %d/%d\n", readyCount, len(cached))
			fmt.Printf("Total Size: %.2f GB\n", float64(totalSize)/(1024*1024*1024))
			fmt.Printf("Location: %s\n", configMgr.GetCacheDir())

			if readyCount > 0 {
				fmt.Printf("\n✅ Status: Ready\n")
			} else {
				fmt.Printf("\n⚠️  Status: Models need attention\n")
			}
		}
	}

	return nil
}

func runVerify(cmd *cobra.Command, args []string) error {
	// Create cache manager
	cm, err := model.NewCacheManager("")
	if err != nil {
		return fmt.Errorf("failed to create cache manager: %w", err)
	}

	// If no model specified, verify all
	if len(args) == 0 {
		results := cm.VerifyAllModels()
		if len(results) == 0 {
			fmt.Println("No models cached.")
			return nil
		}

		allReady := true
		for _, result := range results {
			icon := "✓"
			if !result.ReadyToDeploy {
				icon = "✗"
				allReady = false
			}
			fmt.Printf("%s %s: %s\n", icon, result.ModelName, result.Status)

			if len(result.Warnings) > 0 {
				for _, w := range result.Warnings {
					fmt.Printf("  ⚠ %s\n", w)
				}
			}
		}

		if allReady {
			fmt.Printf("\n✅ All models verified and ready!\n")
		} else {
			fmt.Printf("\n⚠️  Some models need attention.\n")
		}
		return nil
	}

	// Verify specific model
	modelName := args[0]
	result := cm.VerifyModel(modelName)

	fmt.Printf("📋 Verification Report: %s\n", modelName)
	fmt.Printf("═══════════════════════════════════════════\n\n")

	fmt.Printf("Status: %s\n", result.Status)
	fmt.Printf("Ready to Deploy: %v\n", result.ReadyToDeploy)
	fmt.Printf("File Count: %d\n", result.FileCount)
	fmt.Printf("Total Size: %.2f GB (%.0f MB)\n", float64(result.TotalSize)/(1024*1024*1024), float64(result.TotalSize)/(1024*1024))

	if len(result.Files) > 0 {
		fmt.Printf("\n📁 Model Files:\n")
		for _, f := range result.Files {
			fmt.Printf("  ✓ %s (%.2f MB)\n", f.Name, float64(f.Size)/(1024*1024))
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("\n⚠️  Warnings:\n")
		for _, w := range result.Warnings {
			fmt.Printf("  • %s\n", w)
		}
	}

	fmt.Printf("\n")
	if result.ReadyToDeploy {
		fmt.Printf("✅ READY: Model is complete and ready to deploy\n")
		fmt.Printf("\nNext step: sovstack deploy %s\n", modelName)
	} else {
		fmt.Printf("❌ NOT READY: Model verification failed\n")
		if result.Status == "not_cached" {
			fmt.Printf("\nFix: Download the model first\n")
			fmt.Printf("  sovstack pull %s\n", modelName)
		}
	}

	return nil
}
