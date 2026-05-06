// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package health provides liveness (/healthz) and readiness (/readyz) probes
// shared across all SovereignStack services.
//
// Liveness ("am I running") returns 200 unconditionally as long as the
// HTTP server is responsive. Use this for container-runtime restart policies.
//
// Readiness ("can I serve traffic") returns 200 only when every registered
// dependency check passes. Use this for load-balancer membership and
// rolling-deploy gating.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Check is a single readiness probe. Return nil for healthy, error otherwise.
// The error message becomes the user-facing diagnostic.
type Check func(ctx context.Context) error

// Checker aggregates named readiness checks. Safe for concurrent use.
type Checker struct {
	mu       sync.RWMutex
	checks   map[string]Check
	timeout  time.Duration
	startTime time.Time
	version  string
}

// New returns a Checker with no registered checks. Liveness handler will
// always return 200; readiness handler returns 200 until checks are
// registered, after which all must pass.
func New() *Checker {
	return &Checker{
		checks:   make(map[string]Check),
		timeout:  3 * time.Second,
		startTime: time.Now(),
	}
}

// SetVersion records a version string included in every probe response.
func (c *Checker) SetVersion(v string) { c.version = v }

// SetTimeout sets the per-check timeout (default 3s).
func (c *Checker) SetTimeout(d time.Duration) { c.timeout = d }

// Register adds a named readiness check. If a check with the same name
// already exists it is replaced.
func (c *Checker) Register(name string, check Check) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks[name] = check
}

// LivenessHandler responds 200 + JSON body. The process is alive if it can
// answer this request; checks are not consulted.
func (c *Checker) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c.writeJSON(w, http.StatusOK, response{
			Status:    "ok",
			UptimeSec: int64(time.Since(c.startTime).Seconds()),
			Version:   c.version,
		})
	}
}

// ReadinessHandler runs all registered checks (in parallel, each with the
// per-check timeout) and responds 200 if all pass, 503 otherwise. The body
// reports each check's outcome.
func (c *Checker) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c.mu.RLock()
		// Snapshot so we don't hold the lock while running checks.
		snapshot := make(map[string]Check, len(c.checks))
		for k, v := range c.checks {
			snapshot[k] = v
		}
		c.mu.RUnlock()

		results := make(map[string]checkResult, len(snapshot))
		var wg sync.WaitGroup
		var resMu sync.Mutex

		for name, check := range snapshot {
			wg.Add(1)
			go func(name string, check Check) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(r.Context(), c.timeout)
				defer cancel()
				start := time.Now()
				err := check(ctx)
				latency := time.Since(start).Milliseconds()

				res := checkResult{LatencyMS: latency, Status: "ok"}
				if err != nil {
					res.Status = "fail"
					res.Error = err.Error()
				}
				resMu.Lock()
				results[name] = res
				resMu.Unlock()
			}(name, check)
		}
		wg.Wait()

		overallStatus := "ok"
		httpCode := http.StatusOK
		for _, res := range results {
			if res.Status != "ok" {
				overallStatus = "unhealthy"
				httpCode = http.StatusServiceUnavailable
				break
			}
		}

		c.writeJSON(w, httpCode, response{
			Status:    overallStatus,
			UptimeSec: int64(time.Since(c.startTime).Seconds()),
			Version:   c.version,
			Checks:    results,
		})
	}
}

func (c *Checker) writeJSON(w http.ResponseWriter, code int, body response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

// response is the shared shape returned by both probes.
type response struct {
	Status    string                 `json:"status"`            // ok | unhealthy
	UptimeSec int64                  `json:"uptime_seconds"`
	Version   string                 `json:"version,omitempty"`
	Checks    map[string]checkResult `json:"checks,omitempty"`
}

type checkResult struct {
	Status    string `json:"status"`               // ok | fail
	LatencyMS int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

// HTTPCheck returns a Check that issues a GET to url and considers it
// healthy if the response is 2xx. Useful for "is my upstream alive" checks.
func HTTPCheck(client *http.Client, url string) Check {
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	return func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return &httpError{Code: resp.StatusCode, URL: url}
		}
		return nil
	}
}

type httpError struct {
	Code int
	URL  string
}

func (e *httpError) Error() string {
	return http.StatusText(e.Code) + " from " + e.URL
}
