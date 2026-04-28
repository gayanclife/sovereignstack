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
package installer

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// InstallStatus represents the current installation state
type InstallStatus struct {
	CUDAInstalled             bool
	CUDAVersion               string
	ContainerToolkitInstalled bool
	DockerInstalled           bool
	DockerVersion             string
	NVIDIADriverInstalled     bool
	NVIDIADriverVersion       string
	OS                        string
	HasSudo                   bool
	OSSupported               bool
}

// IssuesToFix identifies which prerequisites are missing
type IssuesToFix struct {
	MissingCUDA             bool
	MissingContainerToolkit bool
	MissingDocker           bool
}

// VerifyInstallation checks the current installation status
func VerifyInstallation() (*InstallStatus, error) {
	status := &InstallStatus{}

	// Detect OS
	os := DetectOS()
	status.OS = os
	status.OSSupported = os == "ubuntu" || os == "debian"

	// Check sudo access
	status.HasSudo = checkSudoAccess()

	// Check CUDA
	status.CUDAInstalled, status.CUDAVersion, _ = checkCUDA()

	// Check NVIDIA drivers
	status.NVIDIADriverInstalled, status.NVIDIADriverVersion, _ = checkNVIDIADriver()

	// Check Docker
	status.DockerInstalled, status.DockerVersion, _ = checkDocker()

	// Check NVIDIA Container Toolkit
	status.ContainerToolkitInstalled = checkContainerToolkit()

	return status, nil
}

// GetIssuesToFix identifies missing prerequisites
// Note: CUDA and Container Toolkit are only required if GPUs are present (detected via NVIDIA drivers)
// CPU-only deployments only need Docker
func GetIssuesToFix(status *InstallStatus) *IssuesToFix {
	return &IssuesToFix{
		// CUDA only required if NVIDIA drivers are detected (meaning GPUs present)
		MissingCUDA: !status.CUDAInstalled && status.NVIDIADriverInstalled,
		// Container Toolkit only required if we have Docker AND GPUs
		MissingContainerToolkit: !status.ContainerToolkitInstalled && status.DockerInstalled && status.NVIDIADriverInstalled,
		MissingDocker:           !status.DockerInstalled,
	}
}

// InstallPrerequisites installs missing components based on issues found
func InstallPrerequisites(issues *IssuesToFix, status *InstallStatus) error {
	if !status.OSSupported {
		fmt.Printf("⚠️  Auto-install only supported on Ubuntu/Debian. You are on: %s\n", status.OS)
		fmt.Println("Manual installation instructions: https://docs.nvidia.com/cuda/cuda-installation-guide-linux/")
		return fmt.Errorf("unsupported OS")
	}

	if !status.HasSudo {
		fmt.Println("✗ Installation requires sudo access. Please run with sudo or configure passwordless sudo.")
		return fmt.Errorf("no sudo access")
	}

	// Install Docker first (if needed)
	if issues.MissingDocker {
		fmt.Println("\n📦 Installing Docker...")
		if err := InstallDocker(); err != nil {
			fmt.Printf("✗ Failed to install Docker: %v\n", err)
			fmt.Println("  Manual instructions: https://docs.docker.com/engine/install/ubuntu/")
			return err
		}
		fmt.Println("✓ Docker installed successfully")
	}

	// Install CUDA (if needed)
	// Note: CUDA installation failures are non-blocking for CPU-only deployments
	if issues.MissingCUDA {
		fmt.Println("\n📦 Installing CUDA...")
		if err := InstallCUDA("12.1"); err != nil {
			fmt.Printf("⚠️  Failed to install CUDA: %v\n", err)
			fmt.Println("  CUDA is required for GPU acceleration but optional for CPU-only deployments.")
			fmt.Println("  Manual instructions: https://docs.nvidia.com/cuda/cuda-installation-guide-linux/")
			fmt.Println("  Continuing with CPU-only setup...")
		} else {
			fmt.Println("✓ CUDA installed successfully")
		}
	}

	// Install NVIDIA Container Toolkit (if Docker is present and toolkit is missing)
	if issues.MissingContainerToolkit {
		fmt.Println("\n📦 Installing NVIDIA Container Toolkit...")
		if err := InstallNvidiaContainerToolkit(); err != nil {
			fmt.Printf("✗ Failed to install NVIDIA Container Toolkit: %v\n", err)
			fmt.Println("  Manual instructions: https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html")
			return err
		}
		fmt.Println("✓ NVIDIA Container Toolkit installed successfully")
	}

	return nil
}

// InstallDocker installs Docker on Ubuntu/Debian
func InstallDocker() error {
	fmt.Println("  Running: apt-get update")
	if err := runCommand("sudo", "apt-get", "update"); err != nil {
		return err
	}

	fmt.Println("  Running: apt-get install -y docker.io")
	if err := runCommand("sudo", "apt-get", "install", "-y", "docker.io"); err != nil {
		return err
	}

	fmt.Println("  Running: systemctl start docker")
	if err := runCommand("sudo", "systemctl", "start", "docker"); err != nil {
		return err
	}

	fmt.Println("  Running: systemctl enable docker")
	if err := runCommand("sudo", "systemctl", "enable", "docker"); err != nil {
		return err
	}

	return nil
}

// InstallCUDA installs CUDA toolkit on Ubuntu/Debian
func InstallCUDA(version string) error {
	// For MVP, we'll use the CUDA repository setup
	// This is a simplified version; in production, you may need more sophisticated version handling

	fmt.Printf("  Running: wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2204/x86_64/cuda-keyring_1.1-1_all.deb\n")
	if err := runCommand("sudo", "wget", "-P", "/tmp", "https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2204/x86_64/cuda-keyring_1.1-1_all.deb"); err != nil {
		return err
	}

	fmt.Println("  Running: dpkg -i /tmp/cuda-keyring_1.1-1_all.deb")
	if err := runCommand("sudo", "dpkg", "-i", "/tmp/cuda-keyring_1.1-1_all.deb"); err != nil {
		return err
	}

	fmt.Println("  Running: apt-get update")
	if err := runCommand("sudo", "apt-get", "update"); err != nil {
		return err
	}

	fmt.Println("  Running: apt-get install -y cuda-toolkit-12-1")
	if err := runCommand("sudo", "apt-get", "install", "-y", "cuda-toolkit-12-1"); err != nil {
		return err
	}

	return nil
}

// InstallNvidiaContainerToolkit installs NVIDIA Container Toolkit on Ubuntu/Debian
func InstallNvidiaContainerToolkit() error {
	fmt.Println("  Running: apt-get update")
	if err := runCommand("sudo", "apt-get", "update"); err != nil {
		return err
	}

	fmt.Println("  Running: apt-get install -y nvidia-container-toolkit")
	if err := runCommand("sudo", "apt-get", "install", "-y", "nvidia-container-toolkit"); err != nil {
		return err
	}

	fmt.Println("  Running: nvidia-ctk runtime configure --runtime=docker")
	if err := runCommand("sudo", "nvidia-ctk", "runtime", "configure", "--runtime=docker"); err != nil {
		return err
	}

	fmt.Println("  Running: systemctl restart docker")
	if err := runCommand("sudo", "systemctl", "restart", "docker"); err != nil {
		return err
	}

	return nil
}

// DetectOS detects the operating system
func DetectOS() string {
	// Read /etc/os-release for OS detection
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return "unknown"
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID=") {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				return strings.Trim(parts[1], "\"")
			}
		}
	}

	return "unknown"
}

// Helper functions

func checkSudoAccess() bool {
	cmd := exec.Command("sudo", "-n", "true")
	return cmd.Run() == nil
}

func checkCUDA() (bool, string, error) {
	cmd := exec.Command("nvcc", "--version")
	output, err := cmd.Output()
	if err != nil {
		return false, "", nil
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "release") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				return true, parts[2], nil
			}
		}
	}

	return true, "unknown", nil
}

func checkNVIDIADriver() (bool, string, error) {
	cmd := exec.Command("nvidia-smi", "--query-gpu=driver_version", "--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err != nil {
		return false, "", nil
	}

	version := strings.TrimSpace(string(output))
	if version != "" {
		return true, version, nil
	}

	return false, "", nil
}

func checkDocker() (bool, string, error) {
	cmd := exec.Command("docker", "--version")
	output, err := cmd.Output()
	if err != nil {
		return false, "", nil
	}

	version := strings.TrimSpace(string(output))
	// Docker version output: "Docker version X.Y.Z, build ..."
	// Extract just the version
	parts := strings.Fields(version)
	if len(parts) >= 3 {
		return true, parts[2], nil
	}

	return true, "unknown", nil
}

func checkContainerToolkit() bool {
	cmd := exec.Command("which", "nvidia-ctk")
	return cmd.Run() == nil
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
