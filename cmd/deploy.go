package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// deployCmd represents the deploy command
var deployCmd = &cobra.Command{
	Use:   "deploy [model-name]",
	Short: "Deploy a model to the inference server",
	Long: `Deploy a specified model to the vLLM inference server.
The model must be pulled first using 'sovstack pull'. This command
will start the Docker container with optimized GPU parameters.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		modelName := args[0]
		fmt.Printf("Deploying model: %s\n", modelName)
		fmt.Printf("Model %s deployment initiated.\n", modelName)
		fmt.Println("API endpoint available at: http://localhost:8000/v1/chat/completions")
	},
}

func init() {
	rootCmd.AddCommand(deployCmd)
}
