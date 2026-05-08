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

package admission

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// runPoller is the controller's single background goroutine. Each tick:
//
//  1. lists running models from the management service
//  2. fetches each model's vLLM /metrics in turn
//  3. parses the three signals we care about
//  4. records the sample (which updates EMA + dynamic threshold)
//
// Errors at any step are logged at debug/warn but never propagated —
// the controller is best-effort and admits when it has no telemetry.
func (c *Controller) runPoller(ctx context.Context) {
	defer close(c.done)

	ticker := time.NewTicker(c.cfg.PollInterval)
	defer ticker.Stop()

	c.tick(ctx) // run once immediately so cold-start has data fast

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.tick(ctx)
		}
	}
}

// tick runs a single poll cycle. Bounded by 0.5 * PollInterval so a slow
// model can't stall the next tick.
func (c *Controller) tick(ctx context.Context) {
	deadline := c.cfg.PollInterval / 2
	if deadline < time.Second {
		deadline = time.Second
	}
	cycleCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	models, err := c.lister.List(cycleCtx)
	if err != nil {
		c.log.Debug("admission: list running models", "error", err)
		return
	}
	if len(models) == 0 {
		return
	}
	for _, m := range models {
		body, err := c.fetcher.Fetch(cycleCtx, m)
		if err != nil {
			c.log.Debug("admission: fetch metrics", "model", m, "error", err)
			continue
		}
		sample := parsePrometheusMetrics(body)
		c.observe(m, sample, len(models))
	}
}

// httpModelLister is the default ModelLister: GET /api/v1/models/running
// on the management service and pull the model names out of the JSON body.
type httpModelLister struct {
	managementURL string
	client        *http.Client
}

// newHTTPModelLister builds a ModelLister backed by the management
// service's discovery endpoint.
func newHTTPModelLister(managementURL string, timeout time.Duration) *httpModelLister {
	return &httpModelLister{
		managementURL: managementURL,
		client:        &http.Client{Timeout: timeout},
	}
}

// runningModelEntry mirrors the subset of /api/v1/models/running we read.
// Match ModelName / Status to the discovery service's JSON tags so future
// fields (port, GPU, etc.) can be added without breaking us.
type runningModelEntry struct {
	ModelName string `json:"model_name"`
	Status    string `json:"status"`
}

func (l *httpModelLister) List(ctx context.Context) ([]string, error) {
	url := l.managementURL + "/api/v1/models/running"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := l.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("management list models: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// The discovery endpoint responds either with a bare array or an object
	// wrapping one — accept both so a future schema tweak doesn't break us.
	var arr []runningModelEntry
	if err := json.Unmarshal(body, &arr); err != nil {
		var wrapped struct {
			Models []runningModelEntry `json:"models"`
		}
		if err2 := json.Unmarshal(body, &wrapped); err2 != nil {
			return nil, fmt.Errorf("decode models: %w", err)
		}
		arr = wrapped.Models
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if e.ModelName == "" || e.Status != "running" {
			continue
		}
		out = append(out, e.ModelName)
	}
	return out, nil
}

// httpMetricsFetcher is the default MetricsFetcher: GET
// /api/v1/models/{name}/metrics on the management service, which proxies
// the vLLM container's own Prometheus endpoint.
type httpMetricsFetcher struct {
	managementURL string
	client        *http.Client
}

func newHTTPMetricsFetcher(managementURL string, timeout time.Duration) *httpMetricsFetcher {
	return &httpMetricsFetcher{
		managementURL: managementURL,
		client:        &http.Client{Timeout: timeout},
	}
}

func (f *httpMetricsFetcher) Fetch(ctx context.Context, model string) (string, error) {
	url := f.managementURL + "/api/v1/models/" + model + "/metrics"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("model metrics: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// NewHTTPClients is a convenience for callers (cmd/gateway.go) that want
// the default management-backed implementations of MetricsFetcher and
// ModelLister. Returned values are wired up against the same
// management URL + a shared per-call timeout.
func NewHTTPClients(managementURL string, timeout time.Duration) (MetricsFetcher, ModelLister) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return newHTTPMetricsFetcher(managementURL, timeout),
		newHTTPModelLister(managementURL, timeout)
}
