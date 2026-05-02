// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package metricsproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// startUpstream fakes a vLLM /metrics endpoint and returns the port the
// proxy should resolve to for the given model name.
func startUpstream(t *testing.T, body string) (int, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(body))
	}))
	// Extract port
	parts := strings.Split(strings.TrimPrefix(srv.URL, "http://"), ":")
	port, _ := strconv.Atoi(parts[1])
	return port, func() { srv.Close() }
}

func TestProxy_HappyPath(t *testing.T) {
	port, stop := startUpstream(t, "vllm:metric 42\n")
	defer stop()

	svc := New()
	svc.Resolver = StaticResolver{"mistral-7b": port}

	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/models/mistral-7b/metrics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "vllm:metric 42") {
		t.Errorf("upstream body not forwarded: %s", w.Body.String())
	}
}

func TestProxy_ModelNotRunning(t *testing.T) {
	svc := New()
	svc.Resolver = StaticResolver{} // empty
	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/models/unknown/metrics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-running model, got %d", w.Code)
	}
}

func TestProxy_NoPort(t *testing.T) {
	svc := New()
	svc.Resolver = StaticResolver{"foo": 0}
	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/models/foo/metrics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for zero port, got %d", w.Code)
	}
}

func TestProxy_RejectsNonGET(t *testing.T) {
	svc := New()
	svc.Resolver = StaticResolver{"x": 8000}
	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/models/x/metrics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestProxy_UnknownEndpoint(t *testing.T) {
	svc := New()
	svc.Resolver = StaticResolver{"x": 8000}
	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/models/x/notmetrics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestProxy_UpstreamUnreachable(t *testing.T) {
	svc := New()
	// resolver hands out a port nothing is listening on
	svc.Resolver = StaticResolver{"foo": 1}

	mux := http.NewServeMux()
	svc.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/models/foo/metrics", nil).
		WithContext(context.Background())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502 when upstream is unreachable, got %d", w.Code)
	}
}
