package cmd

import (
	"fmt"

	"github.com/gayanclife/sovereignstack/core/model"
	"github.com/gayanclife/sovereignstack/internal/config"
	"github.com/spf13/cobra"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show system status and cached models",
	Long: `Display the current status of SovereignStack, including:
- Cached models and their verification status
- Total cache usage and location
- Which models are ready to deploy
- File integrity and completeness`,
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
	statusCmd.Flags().BoolP("detailed", "d", false, "Show detailed file listing")
	statusCmd.Flags().BoolP("json", "j", false, "Output as JSON")
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(verifyCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	detailed, _ := cmd.Flags().GetBool("detailed")

	// Load config
	configMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Printf("📊 SovereignStack Status\n")
	fmt.Printf("═══════════════════════════════════════════\n\n")

	// Create cache manager
	cm, err := model.NewCacheManager("")
	if err != nil {
		return fmt.Errorf("failed to create cache manager: %w", err)
	}

	// List and verify all models
	cached := cm.ListCached()

	if len(cached) == 0 {
		fmt.Printf("💾 Cached Models: None\n\n")
		fmt.Printf("Get started:\n")
		fmt.Printf("  sovstack pull gpt2           - Download GPT-2 (small model)\n")
		fmt.Printf("  sovstack status              - Show this status\n")
		fmt.Printf("  sovstack deploy gpt2             - Deploy the model\n")
		return nil
	}

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
