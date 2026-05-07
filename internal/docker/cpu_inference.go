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

// Generic, model-agnostic inference server. Detects the model's capability
// from config.architectures, loads it with the matching AutoModelFor* class
// on the first try, and only registers the endpoints the model can serve.
const cpuServerPython = `from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from transformers import AutoConfig, AutoTokenizer
import transformers
import uvicorn
import os
import sys
import time
import logging
import torch

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("sovstack")

model_path = os.environ.get("MODEL_PATH", "/model")
model_name = os.path.basename(model_path) or "model"

# Map architecture suffix -> (capability, transformers AutoModel class).
# First match wins; order matters because some suffixes are sub-strings of others.
ARCH_MAP = [
    ("ForCausalLM",                "generative",     "AutoModelForCausalLM"),
    ("LMHeadModel",                "generative",     "AutoModelForCausalLM"),
    ("ForConditionalGeneration",   "seq2seq",        "AutoModelForSeq2SeqLM"),
    ("ForSeq2SeqLM",               "seq2seq",        "AutoModelForSeq2SeqLM"),
    ("ForSequenceClassification",  "classification", "AutoModelForSequenceClassification"),
    ("ForTokenClassification",     "classification", "AutoModelForTokenClassification"),
    ("ForQuestionAnswering",       "classification", "AutoModelForQuestionAnswering"),
    ("ForMaskedLM",                "encoder",        "AutoModel"),
]

ENDPOINTS = {
    "generative":     ["/v1/chat/completions", "/v1/completions"],
    "seq2seq":        ["/v1/completions"],
    "encoder":        ["/v1/embeddings"],
    "classification": ["/v1/classify"],
}

def detect_capability(config):
    archs = getattr(config, "architectures", None) or []
    for arch in archs:
        for suffix, capability, klass in ARCH_MAP:
            if arch.endswith(suffix):
                return capability, klass, arch
    # Unknown architecture: fall back to plain encoder embeddings, the one
    # path that works for any transformer with a base hidden state.
    return "encoder", "AutoModel", (archs[0] if archs else "Unknown")

log.info("loading model '%s' from %s", model_name, model_path)
try:
    config = AutoConfig.from_pretrained(model_path, local_files_only=True, trust_remote_code=True)
except Exception as e:
    log.error("could not read model config: %s", e)
    sys.exit(1)

capability, model_class_name, arch_used = detect_capability(config)
log.info("architecture=%s capability=%s loader=%s", arch_used, capability, model_class_name)

try:
    tokenizer = AutoTokenizer.from_pretrained(model_path, local_files_only=True, trust_remote_code=True)
    ModelClass = getattr(transformers, model_class_name)
    # transformers >= 4.43 prefers 'dtype'; older versions only know 'torch_dtype'.
    try:
        model = ModelClass.from_pretrained(
            model_path, local_files_only=True, dtype=torch.float32, trust_remote_code=True,
        )
    except TypeError:
        model = ModelClass.from_pretrained(
            model_path, local_files_only=True, torch_dtype=torch.float32, trust_remote_code=True,
        )
    model.eval()
except Exception as e:
    log.error("model load failed: %s", e)
    sys.exit(1)

log.info("model ready: capability=%s endpoints=%s", capability, ENDPOINTS[capability])

app = FastAPI(title="sovstack-inference")
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

def count_tokens(text):
    if isinstance(text, list):
        return sum(len(tokenizer.encode(t, add_special_tokens=False)) for t in text)
    return len(tokenizer.encode(text, add_special_tokens=False))

def messages_to_prompt(messages):
    parts = []
    for m in messages:
        role = m.get("role", "user")
        content = m.get("content", "")
        parts.append(f"{role}: {content}")
    parts.append("assistant:")
    return "\n".join(parts)

def generate_text(prompt, max_tokens, temperature):
    inputs = tokenizer(prompt, return_tensors="pt")
    pad_id = tokenizer.eos_token_id if tokenizer.eos_token_id is not None else tokenizer.pad_token_id
    temp = float(temperature)
    with torch.no_grad():
        outputs = model.generate(
            inputs["input_ids"],
            max_new_tokens=int(max_tokens),
            temperature=max(0.1, temp),
            do_sample=temp > 0,
            top_p=0.95,
            pad_token_id=pad_id,
        )
    text = tokenizer.decode(outputs[0], skip_special_tokens=True)
    if text.startswith(prompt):
        text = text[len(prompt):]
    return text.strip()

def embed_texts(texts):
    if not isinstance(texts, list):
        texts = [texts]
    inputs = tokenizer(texts, return_tensors="pt", padding=True, truncation=True)
    with torch.no_grad():
        outputs = model(**inputs)
    last_hidden = outputs.last_hidden_state
    mask = inputs["attention_mask"].unsqueeze(-1).float()
    pooled = (last_hidden * mask).sum(dim=1) / mask.sum(dim=1).clamp(min=1e-9)
    pooled = torch.nn.functional.normalize(pooled, p=2, dim=1)
    return pooled.tolist()

@app.get("/health")
def health():
    return {"status": "ok", "model": model_name, "capability": capability}

@app.get("/v1/models")
def list_models():
    return {
        "object": "list",
        "data": [{
            "id": model_name,
            "object": "model",
            "created": int(time.time()),
            "owned_by": "sovereignstack",
            "capability": capability,
            "endpoints": ENDPOINTS[capability],
        }],
    }

if capability in ("generative", "seq2seq"):
    @app.post("/v1/chat/completions")
    def chat_completions(req: dict):
        messages = req.get("messages") or []
        if not messages:
            raise HTTPException(status_code=400, detail="messages required")
        prompt = messages_to_prompt(messages)
        text = generate_text(prompt, req.get("max_tokens", 256), req.get("temperature", 0.7))
        prompt_tokens = count_tokens(prompt)
        completion_tokens = count_tokens(text)
        return {
            "id": "chatcmpl-cpu",
            "object": "chat.completion",
            "created": int(time.time()),
            "model": req.get("model", model_name),
            "choices": [{
                "index": 0,
                "message": {"role": "assistant", "content": text},
                "finish_reason": "stop",
            }],
            "usage": {
                "prompt_tokens": prompt_tokens,
                "completion_tokens": completion_tokens,
                "total_tokens": prompt_tokens + completion_tokens,
            },
        }

    @app.post("/v1/completions")
    def completions(req: dict):
        prompt = req.get("prompt")
        if prompt is None:
            raise HTTPException(status_code=400, detail="prompt required")
        if isinstance(prompt, list):
            prompt = prompt[0]
        text = generate_text(prompt, req.get("max_tokens", 256), req.get("temperature", 0.7))
        prompt_tokens = count_tokens(prompt)
        completion_tokens = count_tokens(text)
        return {
            "id": "cmpl-cpu",
            "object": "text_completion",
            "created": int(time.time()),
            "model": req.get("model", model_name),
            "choices": [{"index": 0, "text": text, "finish_reason": "stop"}],
            "usage": {
                "prompt_tokens": prompt_tokens,
                "completion_tokens": completion_tokens,
                "total_tokens": prompt_tokens + completion_tokens,
            },
        }

if capability == "encoder":
    @app.post("/v1/embeddings")
    def embeddings(req: dict):
        inp = req.get("input")
        if inp is None:
            raise HTTPException(status_code=400, detail="input required")
        vectors = embed_texts(inp)
        data = [{"object": "embedding", "index": i, "embedding": v} for i, v in enumerate(vectors)]
        total_tokens = count_tokens(inp)
        return {
            "object": "list",
            "data": data,
            "model": req.get("model", model_name),
            "usage": {"prompt_tokens": total_tokens, "total_tokens": total_tokens},
        }

if capability == "classification":
    @app.post("/v1/classify")
    def classify(req: dict):
        text = req.get("input") or req.get("text") or req.get("prompt")
        if text is None:
            raise HTTPException(status_code=400, detail="input required")
        inputs = tokenizer(text, return_tensors="pt", truncation=True)
        with torch.no_grad():
            outputs = model(**inputs)
        scores = torch.softmax(outputs.logits[0], dim=-1).tolist()
        id2label = getattr(model.config, "id2label", None) or {}
        labels = sorted(
            [{"label": id2label.get(i, f"LABEL_{i}"), "score": s} for i, s in enumerate(scores)],
            key=lambda x: x["score"],
            reverse=True,
        )
        return {"model": req.get("model", model_name), "labels": labels}

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
	containerName := GetContainerName(config.ModelName, false)

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

	// Convert model path to absolute path for Docker volume mount
	modelDir := filepath.Dir(config.ModelPath)
	absModelDir, err := filepath.Abs(modelDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve model path: %w", err)
	}

	args := []string{
		"run",
		"-d", // Detached mode - container runs in background
		"--shm-size", "2g",
		"-p", fmt.Sprintf("%d:8000", actualPort),
		"-v", fmt.Sprintf("%s:/model", absModelDir),
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
