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
	"fmt"
	"sort"

	"github.com/gayanclife/sovereignstack/core"
	"github.com/gayanclife/sovereignstack/core/model"
	"github.com/gayanclife/sovereignstack/internal/hardware"
	"github.com/spf13/cobra"
)

var (
	filterGPU bool
	filterCPU bool
	minVRAM   int
	showAll   bool
	remoteURL string
)

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Discover and manage available models",
	Long: `Browse and search available models compatible with your hardware.
Models are loaded from:
  1. Local bundled registry (always available, offline)
  2. Remote registry (fetched and cached, optional)
  3. User configuration (~/.sovereignstack/models.yaml)

Run 'sovstack models list' to see compatible models for your hardware.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default to list if no subcommand
		cmd.Help()
	},
}

var modelsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available models for your hardware",
	Long: `Show all models compatible with your hardware configuration.

Automatically detects your GPUs and filters models accordingly.
Use --all to show incompatible models (for reference).`,
	Run: func(cmd *cobra.Command, args []string) {
		// Load local models first (always available)
		localModels, err := model.LoadAllModels()
		if err != nil {
			fmt.Printf("Error loading local models: %v\n", err)
			return
		}

		// Try to load remote models if URL specified
		allModels := localModels
		remoteRegistry := model.NewRemoteRegistry(remoteURL)
		remoteModels, remoteErr := remoteRegistry.FetchAndCache()

		if remoteErr == nil && remoteModels != nil {
			allModels = model.MergeRegistries(localModels, remoteModels)
			fmt.Printf("✓ Loaded models from local registry + remote cache\n\n")
		} else if remoteErr != nil {
			fmt.Printf("⚠️  Remote models unavailable: %v\n", remoteErr)
			fmt.Printf("Using local registry only\n\n")
		} else {
			fmt.Printf("✓ Loaded models from local registry\n\n")
		}

		// Detect hardware
		hw, err := hardware.GetSystemHardware()
		if err != nil {
			fmt.Printf("Error detecting hardware: %v\n", err)
			return
		}

		hasGPU := len(hw.GPUs) > 0
		var compatible []*core.ModelMetadata

		if showAll {
			// Show all models
			compatible = make([]*core.ModelMetadata, 0, len(allModels))
			for _, m := range allModels {
				compatible = append(compatible, m)
			}
		} else {
			// Filter by hardware
			compatible = model.FilterByHardware(
				allModels,
				hasGPU,
				hw.TotalAvailable,
				hw.SystemRAM,
			)
		}

		if len(compatible) == 0 {
			fmt.Println("❌ No compatible models found for your hardware")
			return
		}

		// Sort by parameter count
		sort.Slice(compatible, func(i, j int) bool {
			return compatible[i].ParameterCount < compatible[j].ParameterCount
		})

		// Display results
		fmt.Printf("📚 Available Models (%d found):\n\n", len(compatible))

		if hasGPU {
			fmt.Printf("GPU: %d x %s (%.1f GB VRAM)\n",
				len(hw.GPUs),
				hw.GPUs[0].Name,
				float64(hw.TotalAvailable)/(1024*1024*1024))
		} else {
			fmt.Println("GPU: None (CPU-only mode)")
		}
		fmt.Printf("RAM: %.1f GB\n\n", float64(hw.SystemRAM)/(1024*1024*1024))

		for _, m := range compatible {
			target := "GPU"
			if m.HardwareTarget == "cpu" {
				target = "CPU"
			} else if m.HardwareTarget == "both" {
				target = "GPU/CPU"
			}

			fmt.Printf("📌 %s (%s)\n", m.Name, target)
			fmt.Printf("   %s\n", m.Description)

			// Show parameter count and context
			if m.ParameterCount > 0 {
				fmt.Printf("   Parameters: %.1fB | Context: %dk\n",
					float64(m.ParameterCount)/(1e9),
					m.Context/1000)
			}

			// Show VRAM/RAM requirements
			if m.HardwareTarget != "cpu" && len(m.RequiredVRAM) > 0 {
				minVRAM := int64(9223372036854775807) // Max int64
				for _, v := range m.RequiredVRAM {
					if v < minVRAM {
						minVRAM = v
					}
				}
				fmt.Printf("   Min VRAM: %.1f GB\n", float64(minVRAM)/(1024*1024*1024))
			}

			if m.MinimumSystemRAM > 0 {
				fmt.Printf("   Min RAM: %.1f GB\n", float64(m.MinimumSystemRAM)/(1024*1024*1024))
			}

			fmt.Printf("   Download: sovstack pull %s\n\n", m.Repo)
		}
	},
}

var modelsRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Refresh models from remote registry",
	Long: `Fetch latest models from remote registry and update local cache.

This requires internet connectivity. Falls back to cache if unavailable.`,
	Run: func(cmd *cobra.Command, args []string) {
		registry := model.NewRemoteRegistry(remoteURL)

		fmt.Println("Fetching models from remote registry...")

		models, err := registry.FetchOnly()
		if err != nil {
			fmt.Printf("❌ Failed to refresh models: %v\n", err)
			return
		}

		fmt.Printf("✅ Successfully fetched %d models from remote registry\n", len(models))
		fmt.Printf("   Cached at: ~/.sovereignstack/models-remote.json\n")
		fmt.Printf("   Cache expires in 24 hours\n")
		fmt.Println("\nRun 'sovstack models list' to see all available models")
	},
}

var modelsClearCacheCmd = &cobra.Command{
	Use:   "clear-cache",
	Short: "Clear remote models cache",
	Long:  `Remove the cached remote models. Fresh cache will be fetched on next 'refresh'.`,
	Run: func(cmd *cobra.Command, args []string) {
		registry := model.NewRemoteRegistry(remoteURL)

		if err := registry.ClearCache(); err != nil {
			fmt.Printf("❌ Failed to clear cache: %v\n", err)
			return
		}

		fmt.Println("✓ Remote models cache cleared")
	},
}

func init() {
	rootCmd.AddCommand(modelsCmd)
	modelsCmd.AddCommand(modelsListCmd)
	modelsCmd.AddCommand(modelsRefreshCmd)
	modelsCmd.AddCommand(modelsClearCacheCmd)

	// Global flag for remote registry URL
	modelsCmd.PersistentFlags().StringVar(&remoteURL,
		"registry",
		"https://models.sovereignstack.io/registry.yaml",
		"Remote registry URL")

	// Flags for list command
	modelsListCmd.Flags().BoolVar(&showAll,
		"all",
		false,
		"Show all models including incompatible ones")
	modelsListCmd.Flags().BoolVar(&filterGPU,
		"gpu",
		false,
		"Filter for GPU-only models")
	modelsListCmd.Flags().BoolVar(&filterCPU,
		"cpu",
		false,
		"Filter for CPU-friendly models")
	modelsListCmd.Flags().IntVar(&minVRAM,
		"min-vram",
		0,
		"Filter by minimum VRAM in GB (GPU only)")
}
