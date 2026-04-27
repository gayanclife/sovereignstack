package cmd

import (
	"context"
	"fmt"

	"github.com/gayanclife/sovereignstack/core"
	"github.com/gayanclife/sovereignstack/core/engine"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		modelName := args[0]
		port, _ := cmd.Flags().GetInt("port")
		quantStr, _ := cmd.Flags().GetString("quantization")
		rebuild, _ := cmd.Flags().GetBool("rebuild")

		fmt.Printf("Deploying model: %s\n", modelName)

		// Initialize engine to detect hardware
		engineConfig := engine.EngineConfig{
			ModelCacheDir: "./models",
			Port:          port,
			RebuildImage:  rebuild,
		}

		er, err := engine.NewEngineRoom(engineConfig)
		if err != nil {
			// Graceful degradation: show suitable models even if engine fails to fully initialize
			fmt.Printf("Warning: Could not fully initialize engine: %v\n", err)
			fmt.Println("\nChecking available models for your hardware...")

			// Try to show suitable models anyway
			suitable, unsuitable := getSuitableModelsInfo()
			if len(suitable) > 0 {
				fmt.Println("\n✓ Models compatible with your hardware:")
				for _, m := range suitable {
					hwTarget := "GPU"
					if m.HardwareTarget == core.HardwareCPUOptimized {
						hwTarget = "CPU"
					}
					fmt.Printf("  • %s (%s, %dB params) - %s\n", m.Name, hwTarget, m.ParameterCount, m.Description)
				}
			}
			if len(unsuitable) > 0 {
				fmt.Println("\n✗ Models NOT compatible with your hardware:")
				for _, m := range unsuitable {
					fmt.Printf("  • %s - %s\n", m.Name, m.Description)
				}
			}
			return nil
		}

		// Check if model is suitable for hardware
		suitable, _ := er.GetSuitableModels()

		// Check if requested model is available
		sysInfo := er.GetSystemInfo()
		isSuitable := false
		for _, m := range suitable {
			if m.Name == modelName {
				isSuitable = true
				break
			}
		}

		if !isSuitable {
			fmt.Printf("✗ Model '%s' is not compatible with detected hardware\n\n", modelName)

			// Check if it's because GPU is not available
			if len(sysInfo.GPUs) == 0 {
				fmt.Println("No NVIDIA GPUs detected. This system can run CPU-optimized models only:")
				for _, m := range suitable {
					fmt.Printf("  • %s (requires %.1f GB RAM)\n", m.Name, float64(m.MinimumSystemRAM)/(1024*1024*1024))
				}
			} else {
				fmt.Println("GPU detected but model is GPU-only. Available GPU models:")
				for _, m := range suitable {
					fmt.Printf("  • %s (requires %.1f GB VRAM)\n", m.Name, float64(m.RequiredVRAM["none"])/(1024*1024*1024))
				}
			}
			return nil
		}

		fmt.Printf("✓ Model %s is compatible with your hardware\n\n", modelName)

		// Plan the deployment
		ctx := context.Background()
		plan, err := er.PlanDeployment(ctx, modelName)
		if err != nil {
			fmt.Printf("✗ Cannot plan deployment: %v\n", err)
			return err
		}

		fmt.Println("📋 Deployment Plan:")
		fmt.Printf("  Model:           %s\n", plan.ModelName)
		fmt.Printf("  Quantization:    %s\n", plan.Quantization)
		fmt.Printf("  Required VRAM:   %.1f GB\n", float64(plan.RequiredVRAM)/(1024*1024*1024))
		fmt.Printf("  Available VRAM:  %.1f GB\n", float64(plan.AvailableVRAM)/(1024*1024*1024))
		if plan.ContextLength > 0 {
			fmt.Printf("  Context Length:  %d tokens\n", plan.ContextLength)
		}
		if plan.Notes != "" {
			fmt.Printf("  Notes:           %s\n", plan.Notes)
		}
		fmt.Println()

		fmt.Println("🚀 Starting deployment...")

		// Parse quantization override if provided
		var optionalQuantization *core.QuantizationType
		if quantStr != "auto" {
			quantType := core.QuantizationType(quantStr)
			optionalQuantization = &quantType
		}

		// Deploy the model
		if err := er.Deploy(ctx, modelName, optionalQuantization); err != nil {
			fmt.Printf("✗ Deployment failed: %v\n", err)
			return err
		}

		fmt.Println()
		fmt.Printf("✅ Model deployed successfully!\n")
		fmt.Printf("  API endpoint: http://localhost:%d/v1/chat/completions\n", port)
		fmt.Println("  Run 'sovstack gateway' to start the secure proxy")

		return nil
	},
}

// getSuitableModelsInfo returns suitable and unsuitable models
// This is used when engine initialization fails
func getSuitableModelsInfo() ([]*core.ModelMetadata, []*core.ModelMetadata) {
	// Default models list (matches getCommonModels in model manager)
	allModels := map[string]*core.ModelMetadata{
		"meta-llama/Llama-2-7b-hf": {
			Name: "meta-llama/Llama-2-7b-hf", HardwareTarget: core.HardwareGPUOnly, ParameterCount: 7e9,
			Description: "Meta's Llama 2 7B parameter model"},
		"distilbert-base-uncased": {
			Name: "distilbert-base-uncased", HardwareTarget: core.HardwareCPUOptimized, ParameterCount: 66e6,
			Description: "DistilBERT - lightweight BERT variant for CPU inference"},
		"TinyLlama/TinyLlama-1.1B": {
			Name: "TinyLlama/TinyLlama-1.1B", HardwareTarget: core.HardwareCPUOptimized, ParameterCount: 1.1e9,
			Description: "TinyLlama - small LLM optimized for CPU inference"},
	}

	var suitable, unsuitable []*core.ModelMetadata
	for _, m := range allModels {
		// Assume CPU-only (no GPUs) as default fallback
		if m.HardwareTarget == core.HardwareCPUOptimized {
			suitable = append(suitable, m)
		} else {
			unsuitable = append(unsuitable, m)
		}
	}
	return suitable, unsuitable
}

func init() {
	rootCmd.AddCommand(deployCmd)
	deployCmd.Flags().IntP("port", "p", 8000, "Port to expose vLLM on")
	deployCmd.Flags().StringP("quantization", "q", "auto", "Quantization type to use (auto/none/awq/gptq/int8)")
	deployCmd.Flags().BoolP("rebuild", "r", false, "Force rebuild of inference engine Docker image")
}
