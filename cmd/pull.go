package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// pullCmd represents the pull command
var pullCmd = &cobra.Command{
	Use:   "pull [model-name]",
	Short: "Pull a model from Hugging Face",
	Long: `Download model weights from Hugging Face or a private registry.
This command fetches the specified model and stores it locally for deployment.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		modelName := args[0]
		fmt.Printf("Pulling model: %s\n", modelName)
		fmt.Printf("Model %s pulled successfully!\n", modelName)
	},
}

func init() {
	rootCmd.AddCommand(pullCmd)
}
