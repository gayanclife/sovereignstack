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
package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	cpuImageName = "sovereignstack-cpu-inference:latest"
)

const cpuServerDockerfile = `FROM python:3.11-slim
RUN pip install --no-cache-dir fastapi uvicorn transformers
RUN pip install --no-cache-dir torch --index-url https://download.pytorch.org/whl/cpu
COPY server.py /server.py
CMD ["python", "/server.py"]
`

const cpuServerPython = `from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from transformers import pipeline, AutoModel, AutoTokenizer
import uvicorn
import os
import torch
import logging
import time

logging.basicConfig(level=logging.INFO)
app = FastAPI()

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

model_path = os.environ.get("MODEL_PATH", "/model")
model_name = os.path.basename(model_path)
print(f"Loading model: {model_name}")
print(f"From path: {model_path}")

pipe = None
task_type = None

# Check if model files exist locally
if os.path.exists(model_path):
    print(f"Found local model at {model_path}")
    print(f"Files: {os.listdir(model_path)}")

    # Try to load as a classification model first (most common for local inference)
    try:
        print("Loading as text-classification...")
        tokenizer = AutoTokenizer.from_pretrained(model_path, local_files_only=True)
        model = AutoModel.from_pretrained(model_path, local_files_only=True)
        pipe = pipeline("text-classification", model=model, tokenizer=tokenizer, device=-1)
        task_type = "text-classification"
        print("✓ Loaded as text-classification")
    except Exception as e:
        print(f"Failed: {str(e)[:100]}")
        pipe = None

# Fallback: try downloading from HuggingFace
if pipe is None:
    pipeline_types = [
        "text-generation",
        "text2text-generation",
        "text-classification",
        "feature-extraction"
    ]

    for task in pipeline_types:
        try:
            print(f"Trying {task}...")
            pipe = pipeline(
                task,
                model=model_name,
                device=-1,
                torch_dtype=torch.float32,
                trust_remote_code=True
            )
            task_type = task
            print(f"✓ Loaded as {task}")
            break
        except Exception as e:
            print(f"  {task} failed: {str(e)[:100]}")
            continue

@app.get("/health")
def health():
    return {"status": "ok"}

@app.post("/v1/chat/completions")
def chat_completions(req: dict):
    if pipe is None:
        return {"error": f"Failed to load model from {model_path}"}, 500

    messages = req.get("messages", [])
    if not messages:
        return {"error": "no messages"}, 400

    prompt = messages[-1].get("content", "")
    max_tokens = req.get("max_tokens", 256)
    temperature = req.get("temperature", 0.7)

    try:
        # Handle different task types
        if task_type == "text-generation":
            result = pipe(
                prompt,
                max_new_tokens=max_tokens,
                do_sample=True,
                temperature=max(0.1, temperature),
                truncation=True
            )
            if isinstance(result, list) and len(result) > 0:
                content = result[0].get("generated_text", str(result[0]))
                if content.startswith(prompt):
                    content = content[len(prompt):].strip()
            else:
                content = str(result)

        elif task_type == "text-classification":
            result = pipe(prompt, truncation=True)
            if isinstance(result, list) and len(result) > 0:
                label = result[0].get("label", "unknown")
                score = result[0].get("score", 0)
                content = f"Label: {label} (confidence: {score:.2f})"
            else:
                content = str(result)

        elif task_type == "feature-extraction":
            result = pipe(prompt, truncation=True)
            content = f"Extracted features"

        else:  # text2text-generation
            result = pipe(prompt, max_length=max_tokens, do_sample=True, temperature=max(0.1, temperature))
            if isinstance(result, list) and len(result) > 0:
                content = result[0].get("generated_text", str(result[0]))
            else:
                content = str(result)

        return {
            "id": "chatcmpl-cpu",
            "object": "chat.completion",
            "created": int(time.time()),
            "model": req.get("model", "cpu-model"),
            "choices": [{
                "index": 0,
                "message": {"role": "assistant", "content": content},
                "finish_reason": "stop"
            }],
            "usage": {
                "prompt_tokens": len(prompt.split()),
                "completion_tokens": len(str(content).split()),
                "total_tokens": len(prompt.split()) + len(str(content).split())
            }
        }
    except Exception as e:
        import traceback
        traceback.print_exc()
        return {"error": str(e)}, 500

if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8000, log_level="info")
`

// CPUInferenceOrchestrator manages CPU-based inference via a lightweight FastAPI server
type CPUInferenceOrchestrator struct {
	containerID string
}

// NewCPUInferenceOrchestrator creates a new CPU inference orchestrator
func NewCPUInferenceOrchestrator() *CPUInferenceOrchestrator {
	return &CPUInferenceOrchestrator{}
}

// Start launches a CPU inference container
func (co *CPUInferenceOrchestrator) Start(ctx context.Context, config InferenceConfig) (containerID string, err error) {
	containerName := "cpu-" + config.ModelName

	// Check if image exists (skip if rebuild requested)
	if config.RebuildImage {
		fmt.Printf("Rebuilding CPU inference Docker image (this may take 2-3 minutes)...\n")
		if err := co.buildImage(ctx); err != nil {
			return "", err
		}
	} else {
		exists, _ := imageExists(ctx, cpuImageName)
		if !exists {
			fmt.Printf("Building CPU inference Docker image (this may take 2-3 minutes)...\n")
			if err := co.buildImage(ctx); err != nil {
				return "", err
			}
		}
	}

	// Remove stale container with same name if it exists
	checkCmd := exec.CommandContext(ctx, "docker", "ps", "-a", "-q", "-f", "name="+containerName)
	existingID, _ := checkCmd.Output()
	if len(existingID) > 0 {
		removeCmd := exec.CommandContext(ctx, "docker", "rm", "-f", containerName)
		removeCmd.Output()
		fmt.Printf("Cleaned up stale container: %s\n", containerName)
	}

	// Find a free port if the requested one is in use
	fmt.Printf("Checking for available port starting at %d...\n", config.Port)
	actualPort := findFreePort(config.Port)
	if actualPort != config.Port {
		fmt.Printf("Port %d is in use, using port %d instead\n", config.Port, actualPort)
	}

	// Build docker run command
	fmt.Printf("Starting container on port %d...\n", actualPort)
	args := []string{
		"run",
		"--shm-size", "2g",
		"-p", fmt.Sprintf("%d:8000", actualPort),
		"-v", fmt.Sprintf("%s:/model", filepath.Dir(config.ModelPath)),
		"-e", "MODEL_PATH=/model/" + filepath.Base(config.ModelPath),
		"-e", "HF_HOME=/model",
		"--name", containerName,
		"--restart", "unless-stopped",
		cpuImageName,
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	fmt.Printf("Docker run returned (err=%v)\n", err)
	if err != nil {
		return "", fmt.Errorf("failed to start CPU inference container: %v\nOutput: %s", err, string(output))
	}

	containerID = strings.TrimSpace(string(output))
	co.containerID = containerID

	// Health check with the actual port
	for i := 0; i < 30; i++ {
		if err := co.HealthCheck(ctx, actualPort); err == nil {
			fmt.Printf("✓ Inference engine ready on port %d\n", actualPort)
			break
		}
		if i%5 == 0 {
			fmt.Printf("  Waiting for inference engine... (attempt %d/30)\n", i+1)
		}
		if i == 29 {
			// Get container logs for debugging
			logs, _ := co.GetLogs(ctx, 20)
			return "", fmt.Errorf("inference engine failed health check after 30s\nContainer logs:\n%s", logs)
		}
		time.Sleep(time.Second)
	}

	return containerID, nil
}

// Stop halts the CPU inference container
func (co *CPUInferenceOrchestrator) Stop(ctx context.Context) error {
	if co.containerID == "" {
		return fmt.Errorf("no active container")
	}

	cmd := exec.CommandContext(ctx, "docker", "stop", co.containerID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	return nil
}

// Remove deletes the container
func (co *CPUInferenceOrchestrator) Remove(ctx context.Context) error {
	if co.containerID == "" {
		return fmt.Errorf("no active container")
	}

	cmd := exec.CommandContext(ctx, "docker", "rm", co.containerID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	co.containerID = ""
	return nil
}

// HealthCheck checks if the inference server is ready
func (co *CPUInferenceOrchestrator) HealthCheck(ctx context.Context, port int) error {
	if co.containerID == "" {
		return fmt.Errorf("no container ID")
	}

	inspectCmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", co.containerID)
	running, err := inspectCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to inspect container: %w", err)
	}

	if strings.TrimSpace(string(running)) == "true" {
		return nil
	}

	return fmt.Errorf("container not running")
}

// IsRunning checks if the container is running
func (co *CPUInferenceOrchestrator) IsRunning(ctx context.Context) (bool, error) {
	if co.containerID == "" {
		return false, nil
	}

	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", co.containerID)
	output, err := cmd.Output()
	if err != nil {
		return false, nil
	}

	return strings.TrimSpace(string(output)) == "true", nil
}

// GetLogs retrieves container logs
func (co *CPUInferenceOrchestrator) GetLogs(ctx context.Context, tailLines int) (string, error) {
	if co.containerID == "" {
		return "", fmt.Errorf("no active container")
	}

	cmd := exec.CommandContext(
		ctx,
		"docker", "logs",
		"--tail", strconv.Itoa(tailLines),
		co.containerID,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}

	return string(output), nil
}

// buildImage builds the CPU inference Docker image
func (co *CPUInferenceOrchestrator) buildImage(ctx context.Context) error {
	tmpDir, err := os.MkdirTemp("", "sovereignstack-cpu-build-")
	if err != nil {
		return fmt.Errorf("failed to create build directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write Dockerfile
	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(cpuServerDockerfile), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}

	// Write server script
	serverPath := filepath.Join(tmpDir, "server.py")
	if err := os.WriteFile(serverPath, []byte(cpuServerPython), 0644); err != nil {
		return fmt.Errorf("failed to write server script: %w", err)
	}

	// Build image (uses available Docker builder)
	cmd := exec.CommandContext(ctx, "docker", "build", "-t", cpuImageName, tmpDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to build CPU inference image: %w", err)
	}

	fmt.Println("✓ CPU inference image built successfully")
	return nil
}
