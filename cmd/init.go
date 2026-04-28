/*
Copyright 2026 SovereignStack Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/gayanclife/sovereignstack/core"
	"github.com/gayanclife/sovereignstack/core/engine"
	"github.com/gayanclife/sovereignstack/internal/hardware"
	"github.com/gayanclife/sovereignstack/internal/installer"
	"github.com/spf13/cobra"
)

var checkOnly bool

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Provision machine for SovereignStack",
	Long: `Run hardware pre-flight checks and optionally auto-install missing prerequisites.
This command should be run on a fresh server before deploying models.

SovereignStack supports both GPU-accelerated and CPU-only deployments.
GPU deployment requires NVIDIA drivers and CUDA. CPU-only deployments only need Docker.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Running pre-flight checks...")

		// Run initial hardware checks
		status, err := installer.VerifyInstallation()
		if err != nil {
			fmt.Printf("Error verifying installation: %v\n", err)
			return
		}

		// Display OS info
		fmt.Printf("System: %s\n", status.OS)
		if !status.OSSupported {
			fmt.Printf("⚠️  Warning: OS not supported for auto-install\n\n")
		}
		fmt.Printf("Sudo access: %s\n\n", boolToYesNo(status.HasSudo))

		// Display GPU detection
		gpus, _ := hardware.DetectGPUs()
		if len(gpus) == 0 {
			fmt.Println("✗ No NVIDIA GPUs detected")
		} else {
			fmt.Printf("✓ Detected %d GPU(s):\n", len(gpus))
			for i, gpu := range gpus {
				fmt.Printf("  GPU %d: %s (%d MB VRAM)\n", i+1, gpu.Name, gpu.VRAM/1024/1024)
			}
		}

		// Display NVIDIA Driver status
		if status.NVIDIADriverInstalled {
			fmt.Printf("✓ NVIDIA Driver: %s\n", status.NVIDIADriverVersion)
		} else {
			fmt.Println("✗ NVIDIA Driver not installed")
		}

		// Display CUDA status
		if status.CUDAInstalled {
			fmt.Printf("✓ CUDA: %s\n", status.CUDAVersion)
		} else {
			fmt.Println("✗ CUDA not installed")
		}

		// Display Docker status
		if status.DockerInstalled {
			fmt.Printf("✓ Docker: %s\n", status.DockerVersion)
		} else {
			fmt.Println("✗ Docker not installed")
		}

		// Display Container Toolkit status
		if status.ContainerToolkitInstalled {
			fmt.Println("✓ NVIDIA Container Toolkit installed")
		} else if status.DockerInstalled {
			fmt.Println("✗ NVIDIA Container Toolkit not installed")
		}

		// Get system hardware info
		hw, err := hardware.GetSystemHardware()
		if err != nil {
			fmt.Printf("Error getting system info: %v\n", err)
			return
		}

		fmt.Printf("✓ System: %d CPU cores, %.1f GB RAM\n\n",
			hw.CPUCores,
			float64(hw.SystemRAM)/(1024*1024*1024))

		// Identify issues
		issues := installer.GetIssuesToFix(status)
		issueCount := 0
		if issues.MissingCUDA {
			issueCount++
		}
		if issues.MissingDocker {
			issueCount++
		}
		if issues.MissingContainerToolkit {
			issueCount++
		}

		if issueCount > 0 {
			fmt.Printf("⚠️  %d issue(s) found:\n", issueCount)
			if issues.MissingCUDA {
				fmt.Println("  • CUDA not installed (required for GPU deployment)")
			}
			if issues.MissingDocker {
				fmt.Println("  • Docker not installed (required for all deployments)")
			}
			if issues.MissingContainerToolkit {
				fmt.Println("  • NVIDIA Container Toolkit not installed (required for GPU deployment)")
			}

			// Show CPU-only note if no GPUs but Docker is available
			if len(gpus) == 0 && !issues.MissingDocker {
				fmt.Println("\n💡 Tip: You can run CPU-only deployments with just Docker.")
				fmt.Println("   CUDA and GPU support are optional.")
			}
		}

		// Check-only mode
		if checkOnly {
			fmt.Println("\n(Use 'sovstack init' without --check to install missing prerequisites)")
			return
		}

		// If issues exist and user hasn't disabled install, prompt to fix
		if issueCount > 0 && status.OSSupported && status.HasSudo {
			fmt.Print("\nFix automatically? [y/N]: ")
			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.ToLower(strings.TrimSpace(response))

			if response == "y" || response == "yes" {
				if err := installer.InstallPrerequisites(issues, status); err != nil {
					fmt.Printf("\nInstallation failed: %v\n", err)
					return
				}

				// Re-verify after installation
				fmt.Println("\nRe-verifying installation...")
				newStatus, _ := installer.VerifyInstallation()
				newIssues := installer.GetIssuesToFix(newStatus)
				newIssueCount := 0
				if newIssues.MissingCUDA {
					newIssueCount++
				}
				if newIssues.MissingDocker {
					newIssueCount++
				}
				if newIssues.MissingContainerToolkit {
					newIssueCount++
				}

				if newIssueCount == 0 {
					fmt.Println("✓ All prerequisites installed successfully!")
				} else {
					fmt.Printf("⚠️  %d issue(s) still remain. Manual installation may be required.\n", newIssueCount)
				}
			}
		} else if issueCount > 0 {
			if !status.OSSupported {
				fmt.Println("\nAuto-install only supported on Ubuntu/Debian.")
				fmt.Println("Manual instructions: https://docs.nvidia.com/cuda/cuda-installation-guide-linux/")
			} else if !status.HasSudo {
				fmt.Println("\nAuto-install requires sudo access.")
				fmt.Println("Configure passwordless sudo or run with sudo privileges.")
			}
		}

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
	initCmd.Flags().BoolVar(&checkOnly, "check", false, "Run checks only, do not install")
}

func boolToYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
