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

// Package admission implements a host-aware adaptive circuit breaker that
// the gateway invokes before each reverse-proxy call. It rejects requests
// that would push the inference engine past its safe operating envelope,
// using metrics polled from each running model's vLLM /metrics endpoint
// (proxied by the management service).
package admission

import (
	"bufio"
	"strconv"
	"strings"
)

// modelMetrics is the slice of vLLM exposition-format metrics the admission
// controller cares about. Anything else in the response is ignored.
type modelMetrics struct {
	// CacheUsagePct is gpu_cache_usage_perc reported as 0..100.
	// vLLM emits values in [0,1]; we normalize to percent for symmetry
	// with the user-facing thresholds.
	CacheUsagePct float64
	// RequestsRunning is num_requests_running — currently being decoded.
	RequestsRunning int
	// RequestsWaiting is num_requests_waiting — queued, not yet started.
	RequestsWaiting int
	// HasCacheUsage is true if the parser found a cache-usage sample.
	// Without it we can't make a safe admission decision; callers should
	// treat the model as "unknown state" and admit conservatively.
	HasCacheUsage bool
}

// parsePrometheusMetrics extracts the three signals the controller needs
// from a vLLM-style Prometheus text-exposition body. Tolerates either the
// `vllm:foo` or `vllm_foo` naming and any label set; samples are taken as
// the last matching sample wins (Prometheus convention for histograms /
// gauges with stale labels). Lines that don't parse are skipped.
func parsePrometheusMetrics(body string) modelMetrics {
	var m modelMetrics
	scanner := bufio.NewScanner(strings.NewReader(body))
	// Prometheus scrape responses can contain very long single lines for
	// histograms with many buckets; raise the scanner buffer to 1 MB.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := splitMetricLine(line)
		if !ok {
			continue
		}
		switch normalizeMetricName(name) {
		case "gpu_cache_usage_perc":
			// vLLM emits [0,1]; convert to percent. If a stack already
			// emits in [0,100] (some forks do) we still read it as-is —
			// values >1.0 are taken at face value.
			v, err := strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
			if v <= 1.0 {
				v *= 100
			}
			m.CacheUsagePct = v
			m.HasCacheUsage = true
		case "num_requests_running":
			n, err := strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
			m.RequestsRunning = int(n)
		case "num_requests_waiting":
			n, err := strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
			m.RequestsWaiting = int(n)
		}
	}
	return m
}

// splitMetricLine breaks a Prometheus exposition line into its metric name
// (without labels) and value. Returns ok=false for lines that don't match
// the expected `name{labels} value [timestamp]` shape, including comment
// lines (`# HELP …`, `# TYPE …`).
func splitMetricLine(line string) (name, value string, ok bool) {
	if strings.HasPrefix(line, "#") {
		return "", "", false
	}
	// A metric line is `name` or `name{labels...}` followed by whitespace
	// and the value; an optional timestamp may follow the value.
	i := strings.IndexAny(line, "{ \t")
	if i <= 0 {
		return "", "", false
	}
	name = line[:i]

	// Strip labels if present.
	rest := line[i:]
	if strings.HasPrefix(rest, "{") {
		end := strings.Index(rest, "}")
		if end < 0 {
			return "", "", false
		}
		rest = rest[end+1:]
	}

	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", "", false
	}
	// Take just the value, ignore optional trailing timestamp.
	if sp := strings.IndexAny(rest, " \t"); sp >= 0 {
		rest = rest[:sp]
	}
	return name, rest, true
}

// normalizeMetricName strips the vLLM namespace prefix so callers can
// match on a stable key regardless of whether the source uses the
// `vllm:foo` colon form, the `vllm_foo` underscore form, or no prefix.
func normalizeMetricName(name string) string {
	switch {
	case strings.HasPrefix(name, "vllm:"):
		return name[len("vllm:"):]
	case strings.HasPrefix(name, "vllm_"):
		return name[len("vllm_"):]
	default:
		return name
	}
}
