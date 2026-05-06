// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gayanclife/sovereignstack/internal/docker"
)

type stubLister struct {
	models []docker.RunningModel
	err    error
}

func (s stubLister) GetRunningModels(_ context.Context) ([]docker.RunningModel, error) {
	return s.models, s.err
}

func TestDiscovery_RunningModels_OK(t *testing.T) {
	svc := &Service{Lister: stubLister{models: []docker.RunningModel{
		{ModelName: "mistral-7b", ContainerID: "abc123def456789012345", Type: "gpu", Status: "running", Port: 8000},
		{ModelName: "phi-3", ContainerID: "ffeeddccbbaa", Type: "cpu", Status: "running", Port: 8002},
	}}}

	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/models/running", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d", w.Code)
	}
	var resp RunningModelsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp.Count != 2 {
		t.Errorf("count: %d", resp.Count)
	}
	if len(resp.Models[0].ContainerID) != 12 {
		t.Errorf("container_id should be truncated to 12 chars, got %q", resp.Models[0].ContainerID)
	}
}

func TestDiscovery_RunningModels_DockerError(t *testing.T) {
	svc := &Service{Lister: stubLister{err: errors.New("docker unreachable")}}

	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/models/running", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on docker err, got %d", w.Code)
	}
}

func TestDiscovery_LegacyHealth(t *testing.T) {
	svc := New() // default lister; we don't call /running so docker isn't invoked
	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("legacy health: %d", w.Code)
	}
}
