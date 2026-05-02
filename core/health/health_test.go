// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLiveness_AlwaysOK(t *testing.T) {
	c := New()
	// Register a check that always fails — liveness should ignore it.
	c.Register("downstream", func(ctx context.Context) error { return errors.New("nope") })

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	c.LivenessHandler()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("liveness should be 200 even when checks fail, got %d", w.Code)
	}

	var body response
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("liveness status: %s", body.Status)
	}
}

func TestReadiness_NoChecks(t *testing.T) {
	c := New()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	c.ReadinessHandler()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("readiness with no checks should be 200, got %d", w.Code)
	}
}

func TestReadiness_AllChecksPass(t *testing.T) {
	c := New()
	c.Register("a", func(ctx context.Context) error { return nil })
	c.Register("b", func(ctx context.Context) error { return nil })

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	c.ReadinessHandler()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body response
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(body.Checks) != 2 {
		t.Errorf("expected 2 checks reported, got %d", len(body.Checks))
	}
}

func TestReadiness_OneCheckFails(t *testing.T) {
	c := New()
	c.Register("good", func(ctx context.Context) error { return nil })
	c.Register("bad", func(ctx context.Context) error { return errors.New("bad downstream") })

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	c.ReadinessHandler()(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when one check fails, got %d", w.Code)
	}

	var body response
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v", err)
	}
	if body.Status != "unhealthy" {
		t.Errorf("status: %s", body.Status)
	}
	if body.Checks["bad"].Status != "fail" {
		t.Errorf("bad check should report fail status: %+v", body.Checks["bad"])
	}
	if body.Checks["bad"].Error == "" {
		t.Error("bad check should include error message")
	}
}

func TestReadiness_TimeoutCountsAsFailure(t *testing.T) {
	c := New()
	c.SetTimeout(50 * time.Millisecond)
	c.Register("slow", func(ctx context.Context) error {
		select {
		case <-time.After(500 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	c.ReadinessHandler()(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 on check timeout, got %d", w.Code)
	}
}

func TestHTTPCheck_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	check := HTTPCheck(nil, srv.URL)
	if err := check(context.Background()); err != nil {
		t.Errorf("HTTPCheck on healthy server should not fail: %v", err)
	}
}

func TestHTTPCheck_FailsOn5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	check := HTTPCheck(nil, srv.URL)
	if err := check(context.Background()); err == nil {
		t.Error("HTTPCheck should fail on 500")
	}
}

func TestHTTPCheck_FailsOnUnreachable(t *testing.T) {
	check := HTTPCheck(&http.Client{Timeout: 100 * time.Millisecond}, "http://127.0.0.1:1") // nothing listening
	if err := check(context.Background()); err == nil {
		t.Error("HTTPCheck should fail when server is unreachable")
	}
}

func TestVersion_IncludedInResponse(t *testing.T) {
	c := New()
	c.SetVersion("1.2.3")

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	c.LivenessHandler()(w, req)

	var body response
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Version != "1.2.3" {
		t.Errorf("version: %s", body.Version)
	}
}
