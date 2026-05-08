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
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Config tunes the admission controller. All thresholds are in percent
// (0..100) on the gpu_cache_usage_perc gauge. Defaults are applied by
// the caller (see core/config); zero values here are NOT auto-filled.
type Config struct {
	// Enabled is the master switch. When false, Allow always admits.
	Enabled bool

	// HardCachePct is the static ceiling for cache usage, the hard cap
	// for the dynamic threshold to drift up to. Above this the controller
	// rejects with 503.
	HardCachePct float64

	// SoftCachePct is the warning threshold. Above this the controller
	// admits but logs at WARN. Used as the "calm" boundary for the EMA.
	SoftCachePct float64

	// MaxQueuePerGB caps queued requests per GB of total host VRAM,
	// fair-shared across running models. A request is rejected when
	// num_requests_waiting on its target model exceeds the share.
	MaxQueuePerGB int

	// PollInterval is how often the metrics poller refreshes its view.
	PollInterval time.Duration

	// EMAAlpha is the smoothing factor in (0,1]. Higher = more responsive
	// (1.0 effectively uses each raw sample). 0 falls back to the default.
	EMAAlpha float64

	// CalmDuration is how long the EMA must stay below SoftCachePct
	// before the dynamic hard cap relaxes back up to HardCachePct.
	CalmDuration time.Duration

	// StressMargin is how far below HardCachePct the dynamic cap drops
	// when the EMA enters the stressed band (>= 0.9 * HardCachePct).
	StressMargin float64

	// MinHardCachePct floors the dynamic hard cap so it can't drift to
	// pathological values under sustained stress.
	MinHardCachePct float64
}

// HostBudget describes the host's static capacity envelope. The poller
// uses TotalVRAMBytes to compute fair-share queue depth; RAM and CPU are
// recorded for future expansion (and to surface in /healthz observability).
type HostBudget struct {
	TotalVRAMBytes int64
	TotalRAMBytes  int64
	CPUCores       int
}

// Decision is the result of an Allow check. RetryAfter is non-zero only
// when Allow is false; clients should honor it as a hint, not a contract.
type Decision struct {
	Allow      bool
	Reason     string
	RetryAfter time.Duration
}

// MetricsFetcher fetches the vLLM /metrics body for a single model name.
// Implementations are expected to be cheap and short-timeout; the default
// uses the management service's metrics-proxy. Injected for testing.
type MetricsFetcher interface {
	Fetch(ctx context.Context, model string) (string, error)
}

// ModelLister returns the names of currently running models. Used by the
// poller to know which models to scrape and to recompute queue shares.
type ModelLister interface {
	List(ctx context.Context) ([]string, error)
}

// Clock is injected so EMA / threshold-relaxation tests don't depend on
// wall-clock timing. Default is realClock (time.Now).
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// modelState is the per-model live observation, protected by Controller.mu.
type modelState struct {
	lastSample        modelMetrics
	cacheUsageEMA     float64
	hardThresholdPct  float64
	calmSince         time.Time // earliest moment EMA fell below soft
	updatedAt         time.Time
	maxQueueDepth     int
}

// Controller is the admission decision-maker. Construct with New, kick the
// background poller with Start, query per-request with Allow, and tear down
// with Stop. Safe for concurrent use after Start.
type Controller struct {
	cfg     Config
	budget  HostBudget
	log     *slog.Logger
	clock   Clock
	fetcher MetricsFetcher
	lister  ModelLister

	mu     sync.RWMutex
	states map[string]*modelState

	cancel context.CancelFunc
	done   chan struct{}
}

// New constructs a Controller with sensible defaults applied to any
// unset Config fields. budget should be derived from internal/hardware
// at startup. fetcher and lister default to the HTTP implementations
// pointed at managementURL when nil.
func New(cfg Config, budget HostBudget, log *slog.Logger,
	fetcher MetricsFetcher, lister ModelLister, clock Clock) *Controller {

	cfg = withDefaults(cfg)
	if log == nil {
		log = slog.Default()
	}
	if clock == nil {
		clock = realClock{}
	}
	return &Controller{
		cfg:     cfg,
		budget:  budget,
		log:     log,
		clock:   clock,
		fetcher: fetcher,
		lister:  lister,
		states:  make(map[string]*modelState),
		done:    make(chan struct{}),
	}
}

// withDefaults fills in any zero-valued Config fields with sensible defaults.
// Centralized so the package can be used standalone in tests without
// duplicating the constants in core/config.
func withDefaults(c Config) Config {
	if c.HardCachePct == 0 {
		c.HardCachePct = 95
	}
	if c.SoftCachePct == 0 {
		c.SoftCachePct = 80
	}
	if c.MaxQueuePerGB == 0 {
		c.MaxQueuePerGB = 4
	}
	if c.PollInterval == 0 {
		c.PollInterval = 5 * time.Second
	}
	if c.EMAAlpha == 0 {
		c.EMAAlpha = 0.2
	}
	if c.CalmDuration == 0 {
		c.CalmDuration = 5 * time.Minute
	}
	if c.StressMargin == 0 {
		c.StressMargin = 10
	}
	if c.MinHardCachePct == 0 {
		c.MinHardCachePct = 60
	}
	return c
}

// Start launches the polling goroutine. Returns immediately. Stop cancels
// it. Calling Start twice is a programming error and will panic.
func (c *Controller) Start(ctx context.Context) {
	if c.cancel != nil {
		panic("admission.Controller.Start called twice")
	}
	if !c.cfg.Enabled {
		c.log.Info("admission controller disabled")
		close(c.done)
		return
	}
	if c.fetcher == nil || c.lister == nil {
		c.log.Warn("admission controller starting without fetcher/lister; will admit all requests")
		close(c.done)
		return
	}
	pollCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	go c.runPoller(pollCtx)
}

// Stop cancels the poller and waits for the goroutine to drain.
// Idempotent.
func (c *Controller) Stop() {
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	<-c.done
}

// Allow is the per-request decision point. Call BEFORE the reverse-proxy
// forwards. When Allow returns false, the gateway should reply 503 with
// the supplied Retry-After.
//
// Conservative behavior: if the controller is disabled, has no telemetry
// for the requested model yet, or has stale telemetry, Allow admits. This
// keeps the gateway from blackholing requests during cold-start.
func (c *Controller) Allow(model string) Decision {
	if !c.cfg.Enabled {
		return Decision{Allow: true, Reason: "admission disabled"}
	}

	c.mu.RLock()
	st, ok := c.states[model]
	c.mu.RUnlock()
	if !ok || !st.lastSample.HasCacheUsage {
		return Decision{Allow: true, Reason: "no telemetry yet"}
	}

	// Stale telemetry — last update older than 3x poll interval. Admit
	// rather than risk blackholing the gateway from a poller hiccup.
	maxAge := 3 * c.cfg.PollInterval
	if c.clock.Now().Sub(st.updatedAt) > maxAge {
		return Decision{Allow: true, Reason: "telemetry stale, admitting"}
	}

	cachePct := st.cacheUsageEMA

	if cachePct >= st.hardThresholdPct {
		return Decision{
			Allow:      false,
			Reason:     fmt.Sprintf("kv-cache %.1f%% >= hard cap %.1f%%", cachePct, st.hardThresholdPct),
			RetryAfter: 5 * time.Second,
		}
	}

	if st.maxQueueDepth > 0 && st.lastSample.RequestsWaiting >= st.maxQueueDepth {
		return Decision{
			Allow:      false,
			Reason:     fmt.Sprintf("queue depth %d >= cap %d", st.lastSample.RequestsWaiting, st.maxQueueDepth),
			RetryAfter: 10 * time.Second,
		}
	}

	if cachePct >= c.cfg.SoftCachePct {
		c.log.Warn("admission soft threshold exceeded",
			"model", model,
			"cache_pct", cachePct,
			"soft", c.cfg.SoftCachePct,
			"hard", st.hardThresholdPct)
	}

	return Decision{Allow: true, Reason: "ok"}
}

// observe records a fresh metrics sample for a model and runs the EMA +
// dynamic-threshold update. Called from the poller; exported on the
// receiver only for tests in the same package.
func (c *Controller) observe(model string, sample modelMetrics, runningCount int) {
	now := c.clock.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	st, ok := c.states[model]
	if !ok {
		st = &modelState{
			hardThresholdPct: c.cfg.HardCachePct,
		}
		c.states[model] = st
	}
	st.lastSample = sample
	st.updatedAt = now
	st.maxQueueDepth = c.computeQueueDepth(runningCount)

	if !sample.HasCacheUsage {
		return
	}

	// EMA update. First sample seeds directly so we don't bias toward 0.
	if st.cacheUsageEMA == 0 {
		st.cacheUsageEMA = sample.CacheUsagePct
	} else {
		st.cacheUsageEMA = c.cfg.EMAAlpha*sample.CacheUsagePct +
			(1-c.cfg.EMAAlpha)*st.cacheUsageEMA
	}

	c.adjustThreshold(st, now)
}

// adjustThreshold is the dynamic-cap state machine. Two transitions:
//
//	stressed (EMA crosses 0.9 * hard cap) → drop hard cap by StressMargin
//	calm     (EMA below soft for CalmDuration) → restore hard cap to configured value
func (c *Controller) adjustThreshold(st *modelState, now time.Time) {
	stressBand := 0.9 * st.hardThresholdPct

	if st.cacheUsageEMA >= stressBand {
		// Tighten and reset calm timer.
		newCap := st.hardThresholdPct - c.cfg.StressMargin
		if newCap < c.cfg.MinHardCachePct {
			newCap = c.cfg.MinHardCachePct
		}
		if newCap < st.hardThresholdPct {
			st.hardThresholdPct = newCap
		}
		st.calmSince = time.Time{}
		return
	}

	if st.cacheUsageEMA < c.cfg.SoftCachePct {
		if st.calmSince.IsZero() {
			st.calmSince = now
		}
		if now.Sub(st.calmSince) >= c.cfg.CalmDuration &&
			st.hardThresholdPct < c.cfg.HardCachePct {
			// Step toward the configured ceiling, not jump to it, to
			// avoid a single relaxation followed by an immediate snap-back.
			step := c.cfg.StressMargin / 2
			st.hardThresholdPct += step
			if st.hardThresholdPct > c.cfg.HardCachePct {
				st.hardThresholdPct = c.cfg.HardCachePct
			}
			st.calmSince = now
		}
		return
	}

	// Between soft and stress band: hold steady, reset calm timer.
	st.calmSince = time.Time{}
}

// computeQueueDepth derives a per-model queue depth budget from the host
// VRAM and running-model count. Fair-share approximation: each model gets
// an equal slice of the configured budget. Returns 0 (no queue cap) when
// VRAM or model count is unknown.
func (c *Controller) computeQueueDepth(runningCount int) int {
	if c.budget.TotalVRAMBytes <= 0 || runningCount <= 0 || c.cfg.MaxQueuePerGB <= 0 {
		return 0
	}
	totalGB := float64(c.budget.TotalVRAMBytes) / (1 << 30)
	perModelGB := totalGB / float64(runningCount)
	q := int(perModelGB * float64(c.cfg.MaxQueuePerGB))
	if q < 1 {
		q = 1
	}
	return q
}

// snapshot exposes the controller's per-model state for tests + future
// /api/v1/admission/state debug endpoint. Returned slice is a copy.
func (c *Controller) snapshot() map[string]modelState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]modelState, len(c.states))
	for k, v := range c.states {
		out[k] = *v
	}
	return out
}
