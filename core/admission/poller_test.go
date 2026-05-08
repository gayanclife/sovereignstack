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
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// stubLister is a deterministic ModelLister for tests; returns whatever
// names it was constructed with, or an error if errOnce is set.
type stubLister struct {
	names    []string
	errOnce  atomic.Bool
	errValue error
}

func (s *stubLister) List(_ context.Context) ([]string, error) {
	if s.errOnce.CompareAndSwap(true, false) {
		return nil, s.errValue
	}
	return s.names, nil
}

// stubFetcher returns a per-model body from its map and counts calls so
// tests can confirm the poller actually scraped each model.
type stubFetcher struct {
	bodies map[string]string
	calls  atomic.Int64
}

func (s *stubFetcher) Fetch(_ context.Context, model string) (string, error) {
	s.calls.Add(1)
	body, ok := s.bodies[model]
	if !ok {
		return "", errors.New("not found")
	}
	return body, nil
}

func TestPoller_TickPopulatesPerModelState(t *testing.T) {
	clock := newFakeClock()
	lister := &stubLister{names: []string{"m1", "m2"}}
	fetcher := &stubFetcher{bodies: map[string]string{
		"m1": `vllm:gpu_cache_usage_perc 0.30
vllm:num_requests_waiting 2
`,
		"m2": `vllm:gpu_cache_usage_perc 0.70
vllm:num_requests_waiting 5
`,
	}}
	cfg := Config{
		Enabled: true, HardCachePct: 95, SoftCachePct: 80,
		MaxQueuePerGB: 4, PollInterval: time.Second, EMAAlpha: 0,
	}
	c := New(cfg, HostBudget{TotalVRAMBytes: 24 * (1 << 30)},
		nil, fetcher, lister, clock)

	c.tick(context.Background())

	snap := c.snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 models in snapshot, got %d", len(snap))
	}
	if snap["m1"].lastSample.CacheUsagePct != 30 {
		t.Errorf("m1 cache: got %.1f, want 30", snap["m1"].lastSample.CacheUsagePct)
	}
	if snap["m2"].lastSample.RequestsWaiting != 5 {
		t.Errorf("m2 waiting: got %d, want 5", snap["m2"].lastSample.RequestsWaiting)
	}
}

func TestPoller_FetchErrorDoesNotKillTick(t *testing.T) {
	clock := newFakeClock()
	lister := &stubLister{names: []string{"good", "bad"}}
	fetcher := &stubFetcher{bodies: map[string]string{
		"good": `vllm:gpu_cache_usage_perc 0.10`,
	}}
	c := New(Config{Enabled: true, PollInterval: time.Second},
		HostBudget{TotalVRAMBytes: 1 << 30},
		nil, fetcher, lister, clock)

	c.tick(context.Background())

	snap := c.snapshot()
	if _, ok := snap["good"]; !ok {
		t.Error("good model should have been observed")
	}
	if _, ok := snap["bad"]; ok {
		t.Error("bad model should not have been observed (fetch errored)")
	}
}

func TestPoller_ListErrorIsNonFatal(t *testing.T) {
	clock := newFakeClock()
	lister := &stubLister{errValue: errors.New("management down")}
	lister.errOnce.Store(true)
	c := New(Config{Enabled: true, PollInterval: time.Second},
		HostBudget{TotalVRAMBytes: 1 << 30},
		nil, &stubFetcher{}, lister, clock)

	// Must not panic; snapshot should remain empty.
	c.tick(context.Background())
	if len(c.snapshot()) != 0 {
		t.Error("snapshot should be empty after a list error")
	}
}

func TestPoller_StartStopRunsCleanly(t *testing.T) {
	clock := newFakeClock()
	lister := &stubLister{names: []string{"m"}}
	fetcher := &stubFetcher{bodies: map[string]string{
		"m": `vllm:gpu_cache_usage_perc 0.20`,
	}}
	c := New(Config{Enabled: true, PollInterval: 50 * time.Millisecond},
		HostBudget{TotalVRAMBytes: 1 << 30},
		nil, fetcher, lister, clock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Start(ctx)

	// Wait for at least one fetch to land.
	deadline := time.Now().Add(2 * time.Second)
	for fetcher.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if fetcher.calls.Load() == 0 {
		t.Fatal("poller did not run within 2s")
	}

	c.Stop()
}

func TestPoller_DisabledControllerSkipsBackground(t *testing.T) {
	c := New(Config{Enabled: false},
		HostBudget{TotalVRAMBytes: 1 << 30},
		nil, &stubFetcher{}, &stubLister{names: []string{"m"}}, newFakeClock())
	c.Start(context.Background())
	// Stop must not deadlock since Start short-circuited.
	c.Stop()
}

func TestHTTPModelLister_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models/running" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"name":"a","status":"running"},
			{"name":"b","status":"running"},
			{"name":"c","status":"stopped"}
		]`))
	}))
	defer srv.Close()

	lister := newHTTPModelLister(srv.URL, time.Second)
	got, err := lister.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("expected [a b], got %v", got)
	}
}

func TestHTTPModelLister_ObjectWrappedSchema(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"x","status":"running"}]}`))
	}))
	defer srv.Close()

	lister := newHTTPModelLister(srv.URL, time.Second)
	got, err := lister.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0] != "x" {
		t.Errorf("wrapped schema: got %v, want [x]", got)
	}
}

func TestHTTPMetricsFetcher_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/models/") || !strings.HasSuffix(r.URL.Path, "/metrics") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`vllm:gpu_cache_usage_perc 0.42`))
	}))
	defer srv.Close()

	fetcher := newHTTPMetricsFetcher(srv.URL, time.Second)
	body, err := fetcher.Fetch(context.Background(), "any-model")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(body, "0.42") {
		t.Errorf("unexpected body: %q", body)
	}
}

func TestHTTPMetricsFetcher_NonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	fetcher := newHTTPMetricsFetcher(srv.URL, time.Second)
	if _, err := fetcher.Fetch(context.Background(), "m"); err == nil {
		t.Error("expected error on 503, got nil")
	}
}
