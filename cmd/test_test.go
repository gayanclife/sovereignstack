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
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

// startCapabilityServer spins up a localhost httptest server that responds
// to GET /v1/models with the supplied body and status. Returns the port the
// server is listening on so it can be passed to fetchCapability.
func startCapabilityServer(t *testing.T, status int, body string) (port int, cleanup func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))

	u, err := url.Parse(srv.URL)
	if err != nil {
		srv.Close()
		t.Fatalf("parse server url: %v", err)
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil {
		srv.Close()
		t.Fatalf("parse port: %v", err)
	}
	return p, srv.Close
}

func TestFetchCapability_Success(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		wantCapability string
		wantEndpoints  []string
	}{
		{
			name: "generative",
			body: `{"data":[{"id":"mistral-7b","capability":"generative",
				"endpoints":["/v1/chat/completions","/v1/completions"]}]}`,
			wantCapability: "generative",
			wantEndpoints:  []string{"/v1/chat/completions", "/v1/completions"},
		},
		{
			name: "encoder",
			body: `{"data":[{"id":"bert-base","capability":"encoder",
				"endpoints":["/v1/embeddings"]}]}`,
			wantCapability: "encoder",
			wantEndpoints:  []string{"/v1/embeddings"},
		},
		{
			name: "classification",
			body: `{"data":[{"id":"distilbert-sst2","capability":"classification",
				"endpoints":["/v1/classify"]}]}`,
			wantCapability: "classification",
			wantEndpoints:  []string{"/v1/classify"},
		},
		{
			name: "seq2seq",
			body: `{"data":[{"id":"t5-small","capability":"seq2seq",
				"endpoints":["/v1/completions"]}]}`,
			wantCapability: "seq2seq",
			wantEndpoints:  []string{"/v1/completions"},
		},
		{
			name: "first entry wins when multiple are present",
			body: `{"data":[
				{"id":"primary","capability":"generative","endpoints":["/v1/chat/completions"]},
				{"id":"secondary","capability":"encoder","endpoints":["/v1/embeddings"]}
			]}`,
			wantCapability: "generative",
			wantEndpoints:  []string{"/v1/chat/completions"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port, cleanup := startCapabilityServer(t, http.StatusOK, tt.body)
			defer cleanup()

			cap, err := fetchCapability(port)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cap.Capability != tt.wantCapability {
				t.Errorf("capability: got %q, want %q", cap.Capability, tt.wantCapability)
			}
			if len(cap.Endpoints) != len(tt.wantEndpoints) {
				t.Fatalf("endpoint count: got %d (%v), want %d (%v)",
					len(cap.Endpoints), cap.Endpoints,
					len(tt.wantEndpoints), tt.wantEndpoints)
			}
			for i, ep := range tt.wantEndpoints {
				if cap.Endpoints[i] != ep {
					t.Errorf("endpoint[%d]: got %q, want %q", i, cap.Endpoints[i], ep)
				}
			}
		})
	}
}

func TestFetchCapability_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"non-200 status", http.StatusInternalServerError, `{"error":"boom"}`},
		{"404 looks like an old server", http.StatusNotFound, `not found`},
		{"empty data array", http.StatusOK, `{"data":[]}`},
		{"missing data field", http.StatusOK, `{}`},
		{"malformed JSON", http.StatusOK, `{not json`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port, cleanup := startCapabilityServer(t, tt.status, tt.body)
			defer cleanup()

			cap, err := fetchCapability(port)
			if err == nil {
				t.Errorf("expected error, got nil (cap=%+v)", cap)
			}
			if cap != nil {
				t.Errorf("expected nil capability on error, got %+v", cap)
			}
		})
	}
}

func TestFetchCapability_ConnectionRefused(t *testing.T) {
	// Port 1 is privileged + nothing listens; fetchCapability should error.
	cap, err := fetchCapability(1)
	if err == nil {
		t.Errorf("expected connection error, got cap=%+v", cap)
	}
}
