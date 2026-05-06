// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package discovery hosts the HTTP handlers that list currently-running
// model containers. Phase E split this out of the monolithic management
// service so it can run as its own binary on a separate port (8889 by
// default) with no admin-auth surface and no KeyStore dependency.
//
// Endpoints owned by this package:
//
//	GET /api/v1/models/running
//	GET /api/v1/health        (legacy compat; prefer /healthz)
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gayanclife/sovereignstack/internal/docker"
)

// ModelLister is the dependency the discovery service needs from the
// runtime: a way to list running model containers. The default impl
// (DefaultLister) shells out to Docker. Tests can supply their own.
type ModelLister interface {
	GetRunningModels(ctx context.Context) ([]docker.RunningModel, error)
}

// DefaultLister is the production impl backed by package
// internal/docker. It implements ModelLister.
type DefaultLister struct{}

func (DefaultLister) GetRunningModels(ctx context.Context) ([]docker.RunningModel, error) {
	return docker.GetRunningModels(ctx)
}

// Service is the dependency-injected handler bundle.
type Service struct {
	Lister ModelLister
}

// New returns a Service backed by the default Docker-shelling lister.
func New() *Service {
	return &Service{Lister: DefaultLister{}}
}

// ModelResponse is one entry in the running-models JSON array.
type ModelResponse struct {
	Name        string `json:"name"`
	ContainerID string `json:"container_id"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Port        int    `json:"port"`
	StartedAt   string `json:"started_at,omitempty"`
}

// RunningModelsResponse is the top-level shape returned by /api/v1/models/running.
type RunningModelsResponse struct {
	Version string          `json:"version"`
	Models  []ModelResponse `json:"models"`
	Count   int             `json:"count"`
}

// Register attaches the discovery handlers to mux.
func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/models/running", s.handleRunningModels)
	mux.HandleFunc("/api/v1/health", s.handleHealth) // legacy compat
}

func (s *Service) handleRunningModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	models, err := s.Lister.GetRunningModels(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	out := make([]ModelResponse, 0, len(models))
	for _, m := range models {
		id := m.ContainerID
		if len(id) > 12 {
			id = id[:12]
		}
		out = append(out, ModelResponse{
			Name:        m.ModelName,
			ContainerID: id,
			Type:        m.Type,
			Status:      m.Status,
			Port:        m.Port,
		})
	}

	_ = json.NewEncoder(w).Encode(RunningModelsResponse{
		Version: "1.0",
		Models:  out,
		Count:   len(out),
	})
}

func (s *Service) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "ready": true})
}
