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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gayanclife/sovereignstack/internal/docker"
	"github.com/spf13/cobra"
)

// testCmd represents the test command
var testCmd = &cobra.Command{
	Use:   "test [model-name]",
	Short: "Test a running model with a sample request",
	Long: `Send a test request to a running model to verify it's working.

If no model name is specified, you'll be prompted to select from running models.

Examples:
  sovstack test distilbert-base-uncased
  sovstack test                          # Interactive selection
  sovstack test distilbert-base-uncased --prompt "Hello, how are you?"
  sovstack test distilbert-base-uncased --tokens 100`,
	Args: cobra.MaximumNArgs(1),
	RunE: runTest,
}

func init() {
	testCmd.Flags().StringP("prompt", "p", "Hello, what is artificial intelligence?", "Prompt to send to the model")
	testCmd.Flags().IntP("tokens", "t", 50, "Maximum tokens to generate")
	testCmd.Flags().Float32P("temperature", "T", 0.7, "Temperature for sampling (0.0-1.0)")
	rootCmd.AddCommand(testCmd)
}

func runTest(cmd *cobra.Command, args []string) error {
	prompt, _ := cmd.Flags().GetString("prompt")
	maxTokens, _ := cmd.Flags().GetInt("tokens")
	temperature, _ := cmd.Flags().GetFloat32("temperature")

	ctx := context.Background()

	// Get running models
	runningModels, err := docker.GetRunningModels(ctx)
	if err != nil {
		return fmt.Errorf("failed to query running models: %w", err)
	}

	if len(runningModels) == 0 {
		fmt.Println("❌ No running models found")
		fmt.Println("\nStart a model first:")
		fmt.Println("  sovstack deploy <model-name>")
		return nil
	}

	// Select model
	var targetModel *docker.RunningModel
	if len(args) == 0 {
		// Interactive selection if multiple models
		if len(runningModels) == 1 {
			targetModel = &runningModels[0]
			fmt.Printf("Testing: %s\n\n", targetModel.ModelName)
		} else {
			fmt.Println("🚀 Running Models:")
			fmt.Println()
			for i, m := range runningModels {
				if m.Port > 0 {
					fmt.Printf("%d. %s (port %d)\n", i+1, m.ModelName, m.Port)
				} else {
					fmt.Printf("%d. %s (not exposed)\n", i+1, m.ModelName)
				}
			}
			fmt.Println()

			choice := 0
			for {
				fmt.Print("Select model to test (1-" + fmt.Sprintf("%d", len(runningModels)) + "): ")
				_, _ = fmt.Scanln(&choice)
				if choice >= 1 && choice <= len(runningModels) {
					targetModel = &runningModels[choice-1]
					break
				}
				fmt.Printf("Invalid selection. Please enter a number between 1 and %d\n", len(runningModels))
			}
			fmt.Println()
		}
	} else {
		// Find by name
		modelName := args[0]
		for i, m := range runningModels {
			if strings.Contains(m.ModelName, modelName) {
				targetModel = &runningModels[i]
				break
			}
		}
		if targetModel == nil {
			fmt.Printf("❌ Model '%s' not found. Running models:\n", modelName)
			for _, m := range runningModels {
				fmt.Printf("  • %s\n", m.ModelName)
			}
			return nil
		}
	}

	// Verify port is available
	if targetModel.Port == 0 {
		return fmt.Errorf("model '%s' is not exposing a port", targetModel.ModelName)
	}

	if targetModel.Status != "running" {
		return fmt.Errorf("model '%s' is not running (status: %s)", targetModel.ModelName, targetModel.Status)
	}

	// Send test request
	fmt.Printf("📤 Testing model: %s\n", targetModel.ModelName)
	fmt.Printf("   Port: %d\n", targetModel.Port)
	fmt.Printf("   Prompt: \"%s\"\n", prompt)
	fmt.Printf("   Max tokens: %d\n", maxTokens)
	fmt.Printf("   Temperature: %.2f\n\n", temperature)

	response, err := sendTestRequest(targetModel.Port, prompt, maxTokens, temperature)
	if err != nil {
		return fmt.Errorf("failed to send test request: %w", err)
	}

	fmt.Printf("✅ Response received:\n")
	fmt.Printf("──────────────────────────────────────────\n")
	fmt.Printf("%s\n", response)
	fmt.Printf("──────────────────────────────────────────\n\n")

	return nil
}

// sendTestRequest sends a chat completion request to a running model
func sendTestRequest(port int, prompt string, maxTokens int, temperature float32) (string, error) {
	url := fmt.Sprintf("http://localhost:%d/v1/chat/completions", port)

	payload := map[string]interface{}{
		"model": "test",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"max_tokens":  maxTokens,
		"temperature": temperature,
		"stream":      false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Use a client with timeout
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response (read raw body first for debugging)
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Show raw response for debugging
	if len(respBody) < 1000 {
		fmt.Printf("📨 Raw response: %s\n\n", string(respBody))
	}

	// Try to parse as JSON object first
	var result map[string]interface{}
	err = json.Unmarshal(respBody, &result)
	if err != nil {
		// If it fails, try to provide helpful error message
		preview := string(respBody)
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		return "", fmt.Errorf("failed to parse response as JSON map: %v\n\nRaw response (first 500 chars):\n%s", err, preview)
	}

	// Extract the message content
	// Handle both vLLM and OpenAI-compatible formats

	// Try to find choices array
	var choices []interface{}
	if choicesVal, ok := result["choices"].([]interface{}); ok && len(choicesVal) > 0 {
		choices = choicesVal
	} else {
		return "", fmt.Errorf("no 'choices' field found in response. Response keys: %v", getKeys(result))
	}

	// First choice
	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("first choice is not a map: %T", choices[0])
	}

	// Try to extract content - might be in 'message' or 'text' field
	var content string

	// Try message.content (OpenAI format)
	if message, ok := choice["message"].(map[string]interface{}); ok {
		if c, ok := message["content"].(string); ok {
			content = c
		}
	}

	// Try text field (some models return this)
	if content == "" {
		if text, ok := choice["text"].(string); ok {
			content = text
		}
	}

	// Try generated_text (transformers format)
	if content == "" {
		if text, ok := choice["generated_text"].(string); ok {
			content = text
		}
	}

	if content == "" {
		return "", fmt.Errorf("could not extract response text from choice. Choice keys: %v", getKeys(choice))
	}

	return content, nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// getKeys returns the keys of a map for debugging
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
