package cmd

import (
	"fmt"

	"github.com/gayanclife/sovereignstack/core"
	"github.com/gayanclife/sovereignstack/core/engine"
	"github.com/gayanclife/sovereignstack/internal/hardware"
	"github.com/spf13/cobra"
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Run hardware pre-flight checks",
	Long: `Perform hardware detection including GPU detection, VRAM check,
and CUDA driver verification. This command should be run on a fresh server
before deploying models.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Running hardware pre-flight checks...")

		// Detect GPUs
		gpus, err := hardware.DetectGPUs()
		if err != nil {
			fmt.Printf("Error detecting GPUs: %v\n", err)
		} else if len(gpus) == 0 {
			fmt.Println("✗ No NVIDIA GPUs detected")
		} else {
			fmt.Printf("✓ Detected %d GPU(s):\n", len(gpus))
			for i, gpu := range gpus {
				fmt.Printf("  GPU %d: %s (%d MB VRAM)\n", i+1, gpu.Name, gpu.VRAM/1024/1024)
			}
		}

		// Check CUDA
		cudaInstalled, version, err := hardware.CheckCUDA()
		if err != nil {
			fmt.Printf("✗ Error checking CUDA: %v\n", err)
		} else if cudaInstalled {
			fmt.Printf("✓ CUDA installed: %s\n", version)
		} else {
			fmt.Println("✗ CUDA not installed")
			fmt.Println("  Installation: https://docs.nvidia.com/cuda/cuda-installation-guide-linux/")
		}

		// Get system hardware info
		hw, err := hardware.GetSystemHardware()
		if err != nil {
			fmt.Printf("Error getting system info: %v\n", err)
			return
		}

		fmt.Printf("✓ System: %d CPU cores, %.1f GB RAM\n",
			hw.CPUCores,
			float64(hw.SystemRAM)/(1024*1024*1024))

		// Show available models
		fmt.Println("\n--- Available Models for Your Hardware ---")

		engineConfig := engine.EngineConfig{
			ModelCacheDir: "./models",
			Port:          8000,
		}

		er, err := engine.NewEngineRoom(engineConfig)
		if err == nil {
			suitable, _ := er.GetSuitableModels()
			if len(suitable) > 0 {
				fmt.Printf("\n✓ %d model(s) compatible with your hardware:\n", len(suitable))
				for _, m := range suitable {
					hwTarget := "GPU"
					if m.HardwareTarget == core.HardwareCPUOptimized {
						hwTarget = "CPU"
					}
					fmt.Printf("  • %s (%s)\n", m.Name, hwTarget)
					fmt.Printf("    %s\n", m.Description)
					if hwTarget == "CPU" {
						fmt.Printf("    Min RAM: %.1f GB\n", float64(m.MinimumSystemRAM)/(1024*1024*1024))
					}
				}
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
