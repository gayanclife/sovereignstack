package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/gayanclife/sovereignstack/core"
	"github.com/gayanclife/sovereignstack/core/engine"
	"github.com/gayanclife/sovereignstack/core/model"
	"github.com/gayanclife/sovereignstack/internal/docker"
	"github.com/spf13/cobra"
)

// deployCmd represents the deploy command
var deployCmd = &cobra.Command{
	Use:   "deploy [model-name]",
	Short: "Deploy a model to the inference server",
	Long: `Deploy a specified model to the inference server.

The model will be automatically pulled from Hugging Face if:
- It's not cached locally, OR
- The cached directory exists but contains no model files

If no model is specified, you'll be prompted to choose from available models.

Use --force to stop and restart an already running model.

This command will start the Docker container with optimized GPU parameters.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var modelName string
		if len(args) == 0 {
			// No model specified, prompt user to choose
			selectedModel, err := promptDeployModelSelection()
			if err != nil {
				return err
			}
			modelName = selectedModel
		} else {
			modelName = args[0]
		}
		port, _ := cmd.Flags().GetInt("port")
		quantStr, _ := cmd.Flags().GetString("quantization")
		rebuild, _ := cmd.Flags().GetBool("rebuild")
		force, _ := cmd.Flags().GetBool("force")

		// If --force, stop any running instance first
		if force {
			ctx := context.Background()
			runningModels, err := docker.GetRunningModels(ctx)
			if err == nil {
				for _, m := range runningModels {
					if strings.Contains(m.ModelName, modelName) || strings.Contains(m.ContainerID, modelName) {
						fmt.Printf("Stopping running instance: %s\n", m.ModelName)
						stopCmd := exec.CommandContext(ctx, "docker", "stop", m.ContainerID[:12])
						if err := stopCmd.Run(); err != nil {
							fmt.Printf("⚠ Warning: Failed to stop container: %v\n", err)
						} else {
							fmt.Println("✓ Stopped")
						}
						break
					}
				}
			}
			fmt.Println()
		}

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

		// Auto-pull model if not cached or incomplete
		fmt.Printf("🔍 Checking model cache status for: %s\n", modelName)

		cm, err := model.NewCacheManager("./models")
		if err != nil {
			fmt.Printf("⚠ Failed to create CacheManager: %v\n", err)
			fmt.Printf("   Will attempt deployment without pre-check\n\n")
		} else {
			fmt.Printf("   Cache directory: ./models\n")

			mm, err2 := model.NewManager("./models")
			if err2 != nil {
				fmt.Printf("⚠ Failed to create Manager: %v\n", err2)
			} else {
				needsPull := false
				var pullReason string

				// Check if model directory exists and has valid files
				validateErr := mm.ValidateModel(modelName)

				if validateErr != nil {
					errMsg := validateErr.Error()
					fmt.Printf("   Validation result: %s\n", errMsg)

					// Model is missing or invalid, need to pull
					if strings.Contains(errMsg, "not cached") || strings.Contains(errMsg, "no model files") || strings.Contains(errMsg, "unknown model") {
						needsPull = true
						if strings.Contains(errMsg, "not cached") {
							pullReason = "not cached"
						} else if strings.Contains(errMsg, "no model files") {
							pullReason = "incomplete (no model files)"
						} else {
							pullReason = "not in registry"
						}
					}
				} else {
					fmt.Printf("   ✅ Model is cached and valid\n")
				}

				if needsPull {
					fmt.Printf("\n📥 Model %s. Auto-pulling %s...\n", pullReason, modelName)
					pullErr := cm.DownloadModel(modelName)
					if pullErr != nil {
						fmt.Printf("⚠ Auto-pull failed: %v\n", pullErr)
						fmt.Printf("   Try manually: sovstack pull %s\n\n", modelName)
						return pullErr
					}
					fmt.Printf("✅ Model pulled successfully\n\n")
				} else {
					fmt.Printf("\n")
				}
			}
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
		fmt.Printf("  Check status: sovstack status\n")
		fmt.Printf("  Stop model:   sovstack stop %s\n", modelName)
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

func promptDeployModelSelection() (string, error) {
	// Load all available models
	allModels, err := model.LoadAllModels()
	if err != nil {
		return "", fmt.Errorf("failed to load model registry: %w", err)
	}

	// Get models for the current hardware
	engineConfig := engine.EngineConfig{
		ModelCacheDir: "./models",
		Port:          8000,
	}

	er, err := engine.NewEngineRoom(engineConfig)
	if err != nil {
		return "", fmt.Errorf("failed to initialize engine: %w", err)
	}

	suitable, _ := er.GetSuitableModels()
	suitableMap := make(map[string]bool)
	for _, m := range suitable {
		suitableMap[m.Name] = true
	}

	// Filter available models to show only suitable ones
	displayModels := make([]*core.ModelMetadata, 0)
	displayNames := make([]string, 0)

	// Preferred order
	preferredOrder := []string{
		"distilbert-base-uncased",
		"TinyLlama/TinyLlama-1.1B-Chat-v1.0",
		"microsoft/Phi-2",
		"google/gemma-2-2b-it",
		"meta-llama/Llama-2-7b-hf",
		"mistralai/Mistral-7B-v0.1",
	}

	for _, name := range preferredOrder {
		if m, ok := allModels[name]; ok && suitableMap[m.Name] {
			displayModels = append(displayModels, m)
			displayNames = append(displayNames, name)
		}
	}

	if len(displayModels) == 0 {
		return "", fmt.Errorf("no suitable models available for your hardware")
	}

	fmt.Println("\n📋 Available Models for your hardware:")
	fmt.Println()
	for i, m := range displayModels {
		paramStr := formatParameterCount(m.ParameterCount)
		fmt.Printf("%d. %s\n", i+1, m.Name)
		fmt.Printf("   %s\n", m.Description)
		fmt.Printf("   Parameters: %s\n", paramStr)
		fmt.Println()
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Select model number (1-" + fmt.Sprintf("%d", len(displayModels)) + "): ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		num, err := strconv.Atoi(input)
		if err != nil || num < 1 || num > len(displayModels) {
			fmt.Printf("Invalid selection. Please enter a number between 1 and %d\n", len(displayModels))
			continue
		}

		return displayNames[num-1], nil
	}
}

func init() {
	rootCmd.AddCommand(deployCmd)
	deployCmd.Flags().IntP("port", "p", 8000, "Port to expose vLLM on")
	deployCmd.Flags().StringP("quantization", "q", "auto", "Quantization type to use (auto/none/awq/gptq/int8)")
	deployCmd.Flags().BoolP("rebuild", "r", false, "Force rebuild of inference engine Docker image")
	deployCmd.Flags().BoolP("force", "f", false, "Stop and restart if model is already running")
}
