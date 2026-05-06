#!/bin/bash

# SovereignStack Installation Script
# This script installs dependencies required for SovereignStack on Ubuntu

set -e

echo "Installing SovereignStack dependencies..."

# Update package list
sudo apt update

# Install Docker
echo "Installing Docker..."
sudo apt install -y apt-transport-https ca-certificates curl gnupg lsb-release
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt update
sudo apt install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin

# Install NVIDIA drivers and CUDA (if GPU present)
if lspci | grep -i nvidia > /dev/null; then
    echo "NVIDIA GPU detected. Installing drivers..."
    sudo apt install -y nvidia-driver-470
    echo "Please reboot the system after installation to load NVIDIA drivers."
else
    echo "No NVIDIA GPU detected. Skipping GPU driver installation."
fi

# Install Go (if not present)
if ! command -v go &> /dev/null; then
    echo "Installing Go..."
    wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
    sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    source ~/.bashrc
fi

echo "Installation completed!"
echo "Please reboot if NVIDIA drivers were installed."
echo "Then run: sovstack init"