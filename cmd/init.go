package cmd

import (
	"fmt"

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
			return
		}

		if len(gpus) == 0 {
			fmt.Println("No NVIDIA GPUs detected. This tool requires NVIDIA GPUs for optimal performance.")
			return
		}

		fmt.Printf("Detected %d GPU(s):\n", len(gpus))
		for i, gpu := range gpus {
			fmt.Printf("  GPU %d: %s (%d MB VRAM)\n", i+1, gpu.Name, gpu.VRAM/1024/1024)
		}

		// Check CUDA
		cudaInstalled, version, err := hardware.CheckCUDA()
		if err != nil {
			fmt.Printf("Error checking CUDA: %v\n", err)
			return
		}

		if cudaInstalled {
			fmt.Printf("CUDA installed: %s\n", version)
		} else {
			fmt.Println("CUDA not installed. Please install NVIDIA CUDA drivers.")
			fmt.Println("Installation instructions: https://docs.nvidia.com/cuda/cuda-installation-guide-linux/")
		}

		fmt.Println("Pre-flight checks completed.")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
