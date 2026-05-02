// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package metricsproxy serves /api/v1/models/{name}/metrics by resolving
// the model name to a running container's host port and proxying the
// vLLM /metrics endpoint. Phase E split this out so it can run as a
// separate binary on its own port (8890 by default) with no admin auth.
package metricsproxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gayanclife/sovereignstack/internal/docker"
)

// PortResolver returns the host port of a running model by name, plus
// whether the model is currently running. Implementations:
//
//	DockerResolver — production; calls docker.GetRunningModels
//	StaticResolver — test/dev; returns from an in-memory map
type PortResolver interface {
	ResolvePort(ctx context.Context, modelName string) (port int, running bool, err error)
}

// DockerResolver looks up running models via docker.GetRunningModels.
type DockerResolver struct{}

func (DockerResolver) ResolvePort(ctx context.Context, name string) (int, bool, error) {
	models, err := docker.GetRunningModels(ctx)
	if err != nil {
		return 0, false, err
	}
	for _, m := range models {
		if m.ModelName == name && m.Status == "running" {
			return m.Port, true, nil
		}
	}
	return 0, false, nil
}

// StaticResolver is a test-only resolver: a map from model name → port.
// All entries are considered "running"; an unknown name reports running=false.
type StaticResolver map[string]int

func (s StaticResolver) ResolvePort(_ context.Context, name string) (int, bool, error) {
	port, ok := s[name]
	return port, ok, nil
}

// Service wraps a PortResolver and an HTTP client for the upstream call.
type Service struct {
	Resolver PortResolver
	Client   *http.Client
}

// New returns a Service backed by DockerResolver and a 5-second-timeout client.
func New() *Service {
	return &Service{
		Resolver: DockerResolver{},
		Client:   &http.Client{Timeout: 5 * time.Second},
	}
}

// Register attaches the metrics-proxy handler to mux.
//
// We register against the prefix "/api/v1/models/" rather than an exact
// path so that {name} can vary. The handler short-circuits if the request
// isn't actually for /metrics on a model.
func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/models/", s.handle)
}

func (s *Service) handle(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	// /api/v1/models/{name}/metrics → ["api","v1","models","{name}","metrics"]
	if len(parts) < 5 || parts[4] != "metrics" {
		http.Error(w, `{"error":"unknown endpoint"}`, http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	name := parts[3]
	port, running, err := s.Resolver.ResolvePort(r.Context(), name)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	if !running {
		http.Error(w, fmt.Sprintf(`{"error":"model %q not running"}`, name), http.StatusNotFound)
		return
	}
	if port == 0 {
		http.Error(w, fmt.Sprintf(`{"error":"model %q has no exposed port"}`, name), http.StatusServiceUnavailable)
		return
	}

	upstream := fmt.Sprintf("http://localhost:%d/metrics", port)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upstream, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "text/plain; version=0.0.4"
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
