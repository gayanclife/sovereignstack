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
	"math"
	"sync"
	"testing"
	"time"
)

// fakeClock is a controllable Clock for tests that exercise time-based
// behavior (EMA decay, calm-window, telemetry staleness).
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// 24 GB host with one running model gives a queue cap of 24 * 4 = 96.
// Two models share to 48 each.
func newTestController(cfg Config, clock *fakeClock) *Controller {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 5 * time.Second
	}
	cfg.Enabled = true
	budget := HostBudget{TotalVRAMBytes: 24 * (1 << 30), CPUCores: 16}
	return New(cfg, budget, nil, nil, nil, clock)
}

func TestAllow_DisabledAdmitsAll(t *testing.T) {
	c := New(Config{Enabled: false}, HostBudget{}, nil, nil, nil, newFakeClock())
	d := c.Allow("anything")
	if !d.Allow {
		t.Errorf("disabled controller should always admit, got %+v", d)
	}
}

func TestAllow_NoTelemetryAdmits(t *testing.T) {
	c := newTestController(Config{}, newFakeClock())
	d := c.Allow("never-observed")
	if !d.Allow {
		t.Errorf("expected admit on cold-start, got %+v", d)
	}
}

func TestAllow_BelowSoftAdmits(t *testing.T) {
	clock := newFakeClock()
	c := newTestController(Config{HardCachePct: 95, SoftCachePct: 80}, clock)
	c.observe("m", modelMetrics{CacheUsagePct: 30, HasCacheUsage: true}, 1)

	d := c.Allow("m")
	if !d.Allow {
		t.Errorf("expected admit at 30%%, got %+v", d)
	}
}

func TestAllow_AboveHardRejects(t *testing.T) {
	clock := newFakeClock()
	c := newTestController(Config{HardCachePct: 95, SoftCachePct: 80, EMAAlpha: 0}, clock)
	c.observe("m", modelMetrics{CacheUsagePct: 96, HasCacheUsage: true}, 1)

	d := c.Allow("m")
	if d.Allow {
		t.Errorf("expected reject at 96%%, got %+v", d)
	}
	if d.RetryAfter == 0 {
		t.Error("rejection should set RetryAfter")
	}
}

func TestAllow_QueueDepthCapEnforced(t *testing.T) {
	clock := newFakeClock()
	// With one model on 24 GB host and MaxQueuePerGB=4 → cap = 96.
	c := newTestController(Config{
		HardCachePct: 95, SoftCachePct: 80, MaxQueuePerGB: 4, EMAAlpha: 1.0,
	}, clock)

	c.observe("m", modelMetrics{
		CacheUsagePct: 50, HasCacheUsage: true, RequestsWaiting: 95,
	}, 1)
	if !c.Allow("m").Allow {
		t.Error("queue=95 should still admit (cap 96)")
	}

	c.observe("m", modelMetrics{
		CacheUsagePct: 50, HasCacheUsage: true, RequestsWaiting: 200,
	}, 1)
	if c.Allow("m").Allow {
		t.Error("queue=200 must be rejected")
	}
}

func TestAllow_StaleTelemetryAdmits(t *testing.T) {
	clock := newFakeClock()
	c := newTestController(Config{
		PollInterval: time.Second, HardCachePct: 95, EMAAlpha: 1.0,
	}, clock)

	// Past the hard cap, but the sample will go stale before we ask.
	c.observe("m", modelMetrics{CacheUsagePct: 99, HasCacheUsage: true}, 1)
	clock.advance(10 * time.Second) // > 3 * 1s poll interval

	d := c.Allow("m")
	if !d.Allow {
		t.Errorf("stale telemetry should fail-open, got %+v", d)
	}
}

func TestObserve_EMAFirstSampleSeedsDirectly(t *testing.T) {
	clock := newFakeClock()
	c := newTestController(Config{EMAAlpha: 0.2}, clock)

	c.observe("m", modelMetrics{CacheUsagePct: 70, HasCacheUsage: true}, 1)
	st := c.snapshot()["m"]
	if math.Abs(st.cacheUsageEMA-70) > 0.01 {
		t.Errorf("first sample should seed EMA exactly: got %.3f", st.cacheUsageEMA)
	}
}

func TestObserve_EMAConverges(t *testing.T) {
	clock := newFakeClock()
	c := newTestController(Config{EMAAlpha: 0.5}, clock)

	// Seed at 0 then push 100 repeatedly; EMA should approach 100.
	c.observe("m", modelMetrics{CacheUsagePct: 0, HasCacheUsage: true}, 1)
	for range 10 {
		c.observe("m", modelMetrics{CacheUsagePct: 100, HasCacheUsage: true}, 1)
	}
	st := c.snapshot()["m"]
	if st.cacheUsageEMA <= 95 {
		t.Errorf("EMA failed to converge toward 100, got %.3f", st.cacheUsageEMA)
	}
}

func TestThreshold_DropsUnderStress(t *testing.T) {
	clock := newFakeClock()
	c := newTestController(Config{
		HardCachePct:    95,
		SoftCachePct:    80,
		EMAAlpha:        1.0, // raw samples for predictable assertions
		StressMargin:    10,
		MinHardCachePct: 60,
	}, clock)

	// Sample above 0.9 * hard (= 85.5) should tighten by StressMargin.
	c.observe("m", modelMetrics{CacheUsagePct: 90, HasCacheUsage: true}, 1)
	st := c.snapshot()["m"]
	if st.hardThresholdPct >= 95 {
		t.Errorf("hard cap should have tightened, still %.1f", st.hardThresholdPct)
	}
	if st.hardThresholdPct < 60 {
		t.Errorf("hard cap fell below floor, got %.1f", st.hardThresholdPct)
	}
}

func TestThreshold_RelaxesAfterCalmWindow(t *testing.T) {
	clock := newFakeClock()
	c := newTestController(Config{
		HardCachePct:    95,
		SoftCachePct:    80,
		EMAAlpha:        1.0,
		StressMargin:    10,
		MinHardCachePct: 60,
		CalmDuration:    5 * time.Minute,
	}, clock)

	// First, drive into stressed state.
	c.observe("m", modelMetrics{CacheUsagePct: 92, HasCacheUsage: true}, 1)
	stressed := c.snapshot()["m"].hardThresholdPct
	if stressed >= 95 {
		t.Fatalf("expected tightened cap, got %.1f", stressed)
	}

	// One calm sample sets calmSince but does not yet relax (no time has
	// passed). Advance the clock past CalmDuration, then observe again —
	// this second calm sample sees the elapsed window and steps the cap up.
	c.observe("m", modelMetrics{CacheUsagePct: 30, HasCacheUsage: true}, 1)
	clock.advance(6 * time.Minute)
	c.observe("m", modelMetrics{CacheUsagePct: 30, HasCacheUsage: true}, 1)

	relaxed := c.snapshot()["m"].hardThresholdPct
	if relaxed <= stressed {
		t.Errorf("hard cap should have relaxed: stressed=%.1f relaxed=%.1f",
			stressed, relaxed)
	}
}

func TestThreshold_FlooredByMinHardCachePct(t *testing.T) {
	clock := newFakeClock()
	c := newTestController(Config{
		HardCachePct:    95,
		SoftCachePct:    80,
		EMAAlpha:        1.0,
		StressMargin:    50, // would drop the cap below the floor on one tick
		MinHardCachePct: 70,
	}, clock)

	c.observe("m", modelMetrics{CacheUsagePct: 92, HasCacheUsage: true}, 1)
	got := c.snapshot()["m"].hardThresholdPct
	if got < 70 {
		t.Errorf("hard cap fell below MinHardCachePct=70, got %.1f", got)
	}
}

func TestComputeQueueDepth_FairShareAcrossModels(t *testing.T) {
	c := New(Config{Enabled: true, MaxQueuePerGB: 4},
		HostBudget{TotalVRAMBytes: 24 * (1 << 30)},
		nil, nil, nil, newFakeClock())

	if got := c.computeQueueDepth(1); got != 96 {
		t.Errorf("1 model: got %d, want 96", got)
	}
	if got := c.computeQueueDepth(2); got != 48 {
		t.Errorf("2 models: got %d, want 48", got)
	}
	if got := c.computeQueueDepth(0); got != 0 {
		t.Errorf("0 models: got %d, want 0 (no cap)", got)
	}
}

func TestComputeQueueDepth_ZeroVRAMReturnsNoCap(t *testing.T) {
	c := New(Config{Enabled: true, MaxQueuePerGB: 4},
		HostBudget{TotalVRAMBytes: 0},
		nil, nil, nil, newFakeClock())
	if got := c.computeQueueDepth(1); got != 0 {
		t.Errorf("zero VRAM should return 0 (no cap), got %d", got)
	}
}

func TestWithDefaults_FillsZeros(t *testing.T) {
	got := withDefaults(Config{})
	if got.HardCachePct == 0 || got.SoftCachePct == 0 ||
		got.MaxQueuePerGB == 0 || got.PollInterval == 0 ||
		got.EMAAlpha == 0 || got.CalmDuration == 0 ||
		got.StressMargin == 0 || got.MinHardCachePct == 0 {
		t.Errorf("withDefaults left zeros: %+v", got)
	}
}

func TestWithDefaults_PreservesNonZeros(t *testing.T) {
	in := Config{
		HardCachePct: 90, SoftCachePct: 70, MaxQueuePerGB: 8,
		PollInterval: 3 * time.Second, EMAAlpha: 0.4,
		CalmDuration: time.Minute, StressMargin: 5, MinHardCachePct: 50,
	}
	got := withDefaults(in)
	if got != in {
		t.Errorf("non-zero values must be preserved: got %+v want %+v", got, in)
	}
}
