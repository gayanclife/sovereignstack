package hardware

import (
	"bufio"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// GPU represents a detected GPU
type GPU struct {
	Index          int     `json:"index"`
	Name           string  `json:"name"`
	VRAM           int64   `json:"vram_bytes"`     // Total VRAM in bytes
	VRAMAvailable  int64   `json:"vram_available"` // Available VRAM in bytes
	Driver         string  `json:"driver_version"`
	CUDACapability string  `json:"cuda_capability"` // e.g., "8.0"
	Temperature    float32 `json:"temperature_celsius"`
}

// SystemHardware represents overall system hardware info
type SystemHardware struct {
	GPUs            []GPU  `json:"gpus"`
	TotalVRAM       int64  `json:"total_vram_bytes"`
	TotalAvailable  int64  `json:"total_available_bytes"`
	CPUCores        int    `json:"cpu_cores"`
	SystemRAM       int64  `json:"system_ram_bytes"`
	CUDAInstalled   bool   `json:"cuda_installed"`
	CUDAVersion     string `json:"cuda_version"`
	DockerInstalled bool   `json:"docker_installed"`
}

// DetectGPUs detects NVIDIA GPUs and their VRAM
func DetectGPUs() ([]GPU, error) {
	cmd := exec.Command(
		"nvidia-smi",
		"--query-gpu=index,name,memory.total,memory.free,driver_version,compute_cap,temperature.gpu",
		"--format=csv,noheader,nounits",
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("nvidia-smi not found or failed: %w", err)
	}

	var gpus []GPU
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 7 {
			continue
		}

		idx, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
		name := strings.TrimSpace(parts[1])
		totalVRAM, _ := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
		availableVRAM, _ := strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64)
		driver := strings.TrimSpace(parts[4])
		capability := strings.TrimSpace(parts[5])
		temp, _ := strconv.ParseFloat(strings.TrimSpace(parts[6]), 32)

		// Convert MB to bytes
		totalVRAM *= 1024 * 1024
		availableVRAM *= 1024 * 1024

		gpus = append(gpus, GPU{
			Index:          idx,
			Name:           name,
			VRAM:           totalVRAM,
			VRAMAvailable:  availableVRAM,
			Driver:         driver,
			CUDACapability: capability,
			Temperature:    float32(temp),
		})
	}

	return gpus, nil
}

// GetSystemHardware returns comprehensive hardware information
func GetSystemHardware() (*SystemHardware, error) {
	gpus, err := DetectGPUs()
	if err != nil {
		gpus = []GPU{} // Continue even if no GPUs found
	}

	hardware := &SystemHardware{
		GPUs:     gpus,
		CPUCores: runtime.NumCPU(),
	}

	for _, gpu := range gpus {
		hardware.TotalVRAM += gpu.VRAM
		hardware.TotalAvailable += gpu.VRAMAvailable
	}

	// Check CUDA
	hardware.CUDAInstalled, hardware.CUDAVersion, _ = CheckCUDA()

	// Check Docker
	hardware.DockerInstalled = CheckDocker()

	return hardware, nil
}

// CheckDocker verifies Docker installation
func CheckDocker() bool {
	cmd := exec.Command("docker", "--version")
	err := cmd.Run()
	return err == nil
}

// CheckCUDA checks if CUDA is installed and returns version
func CheckCUDA() (installed bool, version string, err error) {
	cmd := exec.Command("nvcc", "--version")
	output, err := cmd.Output()
	if err != nil {
		return false, "", nil
	}

	// Parse version from output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "release") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				version = parts[2]
				return true, version, nil
			}
		}
	}

	return true, "unknown", nil
}
