package cmd

import (
	"fmt"
	"os"

	"github.com/gayanclife/sovereignstack/core/model"
	"github.com/spf13/cobra"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show system status and cached models",
	Long: `Display the current status of SovereignStack, including:
- Hardware configuration
- Cached models and their size
- Total cache usage
- Model verification status`,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().String("cache-dir", "./models", "Directory where models are cached")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	cacheDir, _ := cmd.Flags().GetString("cache-dir")

	fmt.Printf("📊 SovereignStack Status\n")
	fmt.Printf("═══════════════════════════════════════════\n\n")

	// Show hardware info
	fmt.Printf("🖥️  Hardware Information\n")
	fmt.Printf("──────────────────────────────────────────\n")

	hw, err := os.Stat(cacheDir)
	if err != nil && os.IsNotExist(err) {
		fmt.Printf("Cache directory not found: %s\n", cacheDir)
		return nil
	}

	// Create cache manager
	cm, err := model.NewCacheManager(cacheDir)
	if err != nil {
		return fmt.Errorf("failed to create cache manager: %w", err)
	}

	// List cached models
	cached := cm.ListCached()

	if len(cached) == 0 {
		fmt.Printf("No models cached yet.\n\n")
		fmt.Printf("Usage:\n")
		fmt.Printf("  sovstack pull [model-name]     - Download a model\n")
		fmt.Printf("  sovstack deploy [model-name]   - Deploy a cached model\n")
		return nil
	}

	fmt.Printf("\n💾 Cached Models (%d)\n", len(cached))
	fmt.Printf("──────────────────────────────────────────\n")

	for i, meta := range cached {
		fmt.Printf("%d. %s\n", i+1, meta.Name)
		fmt.Printf("   Size: %.2f MB\n", float64(meta.Size)/(1024*1024))
		fmt.Printf("   Path: %s\n", meta.Path)
		fmt.Printf("   Cached: %s\n", meta.Downloaded.Format("2006-01-02 15:04:05"))

		// Verify file exists
		if _, err := os.Stat(meta.Path); err == nil {
			fmt.Printf("   Status: ✓ Present on disk\n")
		} else {
			fmt.Printf("   Status: ✗ Missing (needs re-download)\n")
		}
		fmt.Printf("\n")
	}

	// Show total cache size
	totalSize := cm.GetTotalCacheSize()
	fmt.Printf("📈 Cache Statistics\n")
	fmt.Printf("──────────────────────────────────────────\n")
	fmt.Printf("Total Models: %d\n", len(cached))
	fmt.Printf("Total Size: %.2f GB\n", float64(totalSize)/(1024*1024*1024))

	if hw != nil && !hw.IsDir() {
		fmt.Printf("Cache Location: %s\n", cacheDir)
	}

	fmt.Printf("\n✅ Status: Ready\n")

	return nil
}
