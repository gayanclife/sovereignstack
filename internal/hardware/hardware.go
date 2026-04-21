package hardware

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// GPU represents a detected GPU
type GPU struct {
	Name string
	VRAM int // in bytes
}

// DetectGPUs detects NVIDIA GPUs and their VRAM
func DetectGPUs() ([]GPU, error) {
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run nvidia-smi: %v", err)
	}

	var gpus []GPU
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		vramStr := strings.TrimSpace(parts[1])
		vramMB, err := strconv.Atoi(vramStr)
		if err != nil {
			continue
		}
		vramBytes := vramMB * 1024 * 1024
		gpus = append(gpus, GPU{Name: name, VRAM: vramBytes})
	}

	return gpus, nil
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