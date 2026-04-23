package cmd

import (
	"fmt"

	"github.com/gayanclife/sovereignstack/core/model"
	"github.com/spf13/cobra"
)

// removeCmd represents the remove command
var removeCmd = &cobra.Command{
	Use:   "remove [model-name]",
	Short: "Remove a cached model",
	Long: `Delete a cached model from disk and remove it from the cache registry.
This command safely removes the model directory and updates the metadata file.

Examples:
  sovstack remove distilbert-base-uncased
  sovstack remove microsoft/phi-2`,
	Args: cobra.ExactArgs(1),
	RunE: runRemove,
}

func init() {
	removeCmd.Flags().String("cache-dir", "./models", "Directory where models are cached")
	removeCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")
	rootCmd.AddCommand(removeCmd)
}

func runRemove(cmd *cobra.Command, args []string) error {
	modelName := args[0]
	cacheDir, _ := cmd.Flags().GetString("cache-dir")
	force, _ := cmd.Flags().GetBool("force")

	// Create cache manager
	cm, err := model.NewCacheManager(cacheDir)
	if err != nil {
		return fmt.Errorf("failed to create cache manager: %w", err)
	}

	// Check if model exists
	if !cm.IsCached(modelName) {
		return fmt.Errorf("model not found in cache: %s", modelName)
	}

	meta := cm.GetCachedModel(modelName)
	if meta == nil {
		return fmt.Errorf("failed to retrieve model metadata")
	}

	// Show what we're about to delete
	fmt.Printf("🗑️  Remove Cached Model\n")
	fmt.Printf("═══════════════════════════════════════════\n\n")
	fmt.Printf("Model: %s\n", meta.Name)
	fmt.Printf("Path: %s\n", meta.Path)
	fmt.Printf("Size: %.2f MB\n", float64(meta.Size)/(1024*1024))
	fmt.Printf("Cached: %s\n\n", meta.Downloaded.Format("2006-01-02 15:04:05"))

	// Ask for confirmation unless --force is set
	if !force {
		fmt.Printf("Are you sure you want to delete this model? (yes/no): ")
		var response string
		fmt.Scanln(&response)

		if response != "yes" && response != "y" {
			fmt.Printf("✗ Cancelled\n")
			return nil
		}
	}

	// Remove from cache
	fmt.Printf("🔄 Removing model...\n")
	if err := cm.RemoveFromCache(modelName); err != nil {
		return fmt.Errorf("failed to remove model: %w", err)
	}

	fmt.Printf("\n✅ Model removed successfully!\n\n")
	fmt.Printf("📊 Cache Statistics\n")
	fmt.Printf("─────────────────────────────────────────\n")

	// Show updated cache status
	remaining := cm.ListCached()
	totalSize := cm.GetTotalCacheSize()

	if len(remaining) == 0 {
		fmt.Printf("Cached Models: 0\n")
		fmt.Printf("Total Size: 0.00 GB\n")
		fmt.Printf("\nCache is now empty.\n")
	} else {
		fmt.Printf("Cached Models: %d\n", len(remaining))
		fmt.Printf("Total Size: %.2f GB\n", float64(totalSize)/(1024*1024*1024))
		fmt.Printf("\nRemaining models:\n")
		for i, m := range remaining {
			fmt.Printf("  %d. %s (%.2f MB)\n", i+1, m.Name, float64(m.Size)/(1024*1024))
		}
	}

	return nil
}
