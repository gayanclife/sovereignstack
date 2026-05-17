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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var gatewayTestCmd = &cobra.Command{
	Use:   "test [model-name]",
	Short: "Send a test request through the gateway",
	Long: `Send a test request to a running model via the gateway, exercising the full
auth → access → quota → routing → proxy pipeline.

Unlike 'sovstack test', which hits the model port directly, this command
routes every request through the gateway so the API key, access policy,
and path stripping are all exercised.

Examples:
  sovstack gateway test mistral-7b
  sovstack gateway test                        # interactive selection
  sovstack gateway test mistral-7b --prompt "Explain Go generics"
  sovstack gateway test mistral-7b --key sk_live_abc123`,
	Args: cobra.MaximumNArgs(1),
	RunE: runGatewayTest,
}

func init() {
	gatewayTestCmd.Flags().String("gateway", "http://localhost:8001", "Gateway URL")
	gatewayTestCmd.Flags().String("discovery", "http://localhost:8889", "Discovery service URL")
	gatewayTestCmd.Flags().StringP("key", "k", "", "API key (falls back to SOVSTACK_API_KEY env, then first key in keys.json)")
	gatewayTestCmd.Flags().StringP("prompt", "p", "Hello, what is artificial intelligence?", "Prompt to send")
	gatewayTestCmd.Flags().IntP("tokens", "t", 50, "Maximum tokens to generate")
	gatewayTestCmd.Flags().Float32P("temperature", "T", 0.7, "Sampling temperature (0.0–1.0)")
	gatewayCmd.AddCommand(gatewayTestCmd)
}

// discoveredModel is a model entry from the discovery service.
type discoveredModel struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Port   int    `json:"port"`
	Type   string `json:"type"`
}

func runGatewayTest(cmd *cobra.Command, args []string) error {
	gatewayURL, _ := cmd.Flags().GetString("gateway")
	discoveryURL, _ := cmd.Flags().GetString("discovery")
	apiKey, _ := cmd.Flags().GetString("key")
	prompt, _ := cmd.Flags().GetString("prompt")
	maxTokens, _ := cmd.Flags().GetInt("tokens")
	temperature, _ := cmd.Flags().GetFloat32("temperature")

	gatewayURL = strings.TrimSuffix(gatewayURL, "/")
	discoveryURL = strings.TrimSuffix(discoveryURL, "/")

	// Resolve API key: flag → env → keys.json first key.
	if apiKey == "" {
		apiKey = os.Getenv("SOVSTACK_API_KEY")
	}
	if apiKey == "" {
		var err error
		apiKey, err = firstKeyFromStore()
		if err != nil {
			return fmt.Errorf("no API key found: pass --key, set SOVSTACK_API_KEY, or run 'sovstack keys add <user>'")
		}
	}

	// Discover running models.
	models, err := discoverRunningModels(discoveryURL)
	if err != nil {
		return fmt.Errorf("discovery service unreachable at %s: %w\n  Is the stack running? Try: ./scripts/start-stack-docker.sh", discoveryURL, err)
	}
	if len(models) == 0 {
		fmt.Println("No running models found.")
		fmt.Println("\nDeploy a model first:")
		fmt.Println("  sovstack deploy <model-name>")
		return nil
	}

	// Select model.
	var target *discoveredModel
	if len(args) == 0 {
		if len(models) == 1 {
			target = &models[0]
			fmt.Printf("Testing: %s\n\n", target.Name)
		} else {
			fmt.Println("Running models:")
			for i, m := range models {
				fmt.Printf("  %d. %s (%s, port %d)\n", i+1, m.Name, m.Type, m.Port)
			}
			fmt.Println()
			choice := 0
			for {
				fmt.Printf("Select model (1-%d): ", len(models))
				_, _ = fmt.Scanln(&choice)
				if choice >= 1 && choice <= len(models) {
					target = &models[choice-1]
					break
				}
				fmt.Printf("Enter a number between 1 and %d\n", len(models))
			}
			fmt.Println()
		}
	} else {
		name := args[0]
		for i := range models {
			if strings.Contains(models[i].Name, name) {
				target = &models[i]
				break
			}
		}
		if target == nil {
			fmt.Printf("Model '%s' not found in running models:\n", name)
			for _, m := range models {
				fmt.Printf("  • %s\n", m.Name)
			}
			return nil
		}
	}

	if target.Status != "running" {
		return fmt.Errorf("model '%s' is not running (status: %s)", target.Name, target.Status)
	}

	fmt.Printf("Testing model : %s\n", target.Name)
	fmt.Printf("Gateway       : %s\n", gatewayURL)
	fmt.Printf("Key           : %s...\n", apiKey[:min(len(apiKey), 12)])

	// Discover capability through the gateway (proves routing works end-to-end).
	cap, err := fetchCapabilityViaGateway(gatewayURL, target.Name, apiKey)
	if err != nil {
		fmt.Printf("Capability    : unknown (gateway returned error: %v) — defaulting to chat\n\n", err)
		return runGatewayChatTest(gatewayURL, target.Name, apiKey, prompt, maxTokens, temperature)
	}
	fmt.Printf("Capability    : %s (endpoints: %s)\n\n", cap.Capability, strings.Join(cap.Endpoints, ", "))

	switch cap.Capability {
	case "generative", "seq2seq":
		fmt.Printf("Prompt        : %q\n", prompt)
		fmt.Printf("Max tokens    : %d\n", maxTokens)
		fmt.Printf("Temperature   : %.2f\n\n", temperature)
		return runGatewayChatTest(gatewayURL, target.Name, apiKey, prompt, maxTokens, temperature)
	case "encoder":
		fmt.Printf("Input         : %q (encoder — testing /v1/embeddings)\n\n", prompt)
		return runGatewayEmbeddingTest(gatewayURL, target.Name, apiKey, prompt)
	case "classification":
		fmt.Printf("Input         : %q (classifier — testing /v1/classify)\n\n", prompt)
		return runGatewayClassifyTest(gatewayURL, target.Name, apiKey, prompt)
	default:
		return fmt.Errorf("unsupported capability %q", cap.Capability)
	}
}

// discoverRunningModels queries the discovery service for running model containers.
func discoverRunningModels(discoveryURL string) ([]discoveredModel, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(discoveryURL + "/api/v1/models/running")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var body struct {
		Models []discoveredModel `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	running := body.Models[:0]
	for _, m := range body.Models {
		if m.Status == "running" {
			running = append(running, m)
		}
	}
	return running, nil
}

// fetchCapabilityViaGateway hits /models/{name}/v1/models through the gateway
// to confirm routing works and get the model's capability.
func fetchCapabilityViaGateway(gatewayURL, modelName, apiKey string) (*modelCapability, error) {
	url := fmt.Sprintf("%s/models/%s/v1/models", gatewayURL, modelName)
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var body struct {
		Data []modelCapability `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	if len(body.Data) == 0 {
		return nil, fmt.Errorf("empty data array")
	}
	return &body.Data[0], nil
}

func runGatewayChatTest(gatewayURL, modelName, apiKey, prompt string, maxTokens int, temperature float32) error {
	url := fmt.Sprintf("%s/models/%s/v1/chat/completions", gatewayURL, modelName)
	payload := map[string]interface{}{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens":  maxTokens,
		"temperature": temperature,
		"stream":      false,
	}
	body, _ := json.Marshal(payload)

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("could not parse response: %w\nraw: %s", err, string(respBody))
	}

	content, err := extractChatContent(result)
	if err != nil {
		return err
	}
	fmt.Printf("Response:\n──────────────────────────────────────────\n%s\n──────────────────────────────────────────\n\n", content)
	return nil
}

func runGatewayEmbeddingTest(gatewayURL, modelName, apiKey, input string) error {
	url := fmt.Sprintf("%s/models/%s/v1/embeddings", gatewayURL, modelName)
	body, _ := json.Marshal(map[string]interface{}{"input": input})

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Usage struct{ TotalTokens int `json:"total_tokens"` } `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("could not parse response: %w", err)
	}
	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return fmt.Errorf("empty embedding in response")
	}
	vec := result.Data[0].Embedding
	preview := vec
	if len(preview) > 5 {
		preview = preview[:5]
	}
	fmt.Printf("Embedding generated:\n")
	fmt.Printf("  Dimensions : %d\n", len(vec))
	fmt.Printf("  First 5    : %v\n", preview)
	fmt.Printf("  Tokens used: %d\n\n", result.Usage.TotalTokens)
	return nil
}

func runGatewayClassifyTest(gatewayURL, modelName, apiKey, input string) error {
	url := fmt.Sprintf("%s/models/%s/v1/classify", gatewayURL, modelName)
	body, _ := json.Marshal(map[string]interface{}{"input": input})

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Labels []struct {
			Label string  `json:"label"`
			Score float64 `json:"score"`
		} `json:"labels"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("could not parse response: %w", err)
	}
	fmt.Printf("Classification results:\n")
	limit := len(result.Labels)
	if limit > 5 {
		limit = 5
	}
	for i := 0; i < limit; i++ {
		fmt.Printf("  %d. %-30s %.4f\n", i+1, result.Labels[i].Label, result.Labels[i].Score)
	}
	fmt.Println()
	return nil
}

// extractChatContent pulls the assistant message text out of a chat completion response.
func extractChatContent(result map[string]interface{}) (string, error) {
	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("no 'choices' in response (keys: %v)", getKeys(result))
	}
	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected choice type %T", choices[0])
	}
	if msg, ok := choice["message"].(map[string]interface{}); ok {
		if c, ok := msg["content"].(string); ok {
			return c, nil
		}
	}
	if text, ok := choice["text"].(string); ok {
		return text, nil
	}
	return "", fmt.Errorf("could not extract content from choice (keys: %v)", getKeys(choice))
}

// firstKeyFromStore reads the first API key from ~/.sovereignstack/keys.json.
func firstKeyFromStore() (string, error) {
	path := filepath.Join(os.Getenv("HOME"), ".sovereignstack", "keys.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var store struct {
		Users []struct {
			APIKey string `json:"api_key"`
		} `json:"users"`
	}
	if err := json.Unmarshal(data, &store); err != nil {
		return "", err
	}
	for _, u := range store.Users {
		if u.APIKey != "" {
			return u.APIKey, nil
		}
	}
	return "", fmt.Errorf("no keys found in %s", path)
}
