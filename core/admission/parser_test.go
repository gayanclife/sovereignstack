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
	"testing"
)

func TestParsePrometheusMetrics_VLLMColonForm(t *testing.T) {
	body := `# HELP vllm:gpu_cache_usage_perc kv cache usage
# TYPE vllm:gpu_cache_usage_perc gauge
vllm:gpu_cache_usage_perc{model_name="m"} 0.42
vllm:num_requests_running{model_name="m"} 7
vllm:num_requests_waiting{model_name="m"} 3
`
	got := parsePrometheusMetrics(body)
	if !got.HasCacheUsage {
		t.Fatal("expected HasCacheUsage=true")
	}
	if math.Abs(got.CacheUsagePct-42.0) > 0.001 {
		t.Errorf("CacheUsagePct: got %.3f, want 42.000", got.CacheUsagePct)
	}
	if got.RequestsRunning != 7 {
		t.Errorf("RequestsRunning: got %d, want 7", got.RequestsRunning)
	}
	if got.RequestsWaiting != 3 {
		t.Errorf("RequestsWaiting: got %d, want 3", got.RequestsWaiting)
	}
}

func TestParsePrometheusMetrics_UnderscoreForm(t *testing.T) {
	body := `vllm_gpu_cache_usage_perc 0.18
vllm_num_requests_running 1
vllm_num_requests_waiting 0
`
	got := parsePrometheusMetrics(body)
	if math.Abs(got.CacheUsagePct-18.0) > 0.001 {
		t.Errorf("CacheUsagePct: got %.3f, want 18.000", got.CacheUsagePct)
	}
	if got.RequestsRunning != 1 {
		t.Errorf("RequestsRunning: got %d, want 1", got.RequestsRunning)
	}
}

func TestParsePrometheusMetrics_AlreadyPercent(t *testing.T) {
	// Some forks expose [0,100] directly. Treat values >1.0 as already in percent.
	body := `vllm:gpu_cache_usage_perc 73.5`
	got := parsePrometheusMetrics(body)
	if math.Abs(got.CacheUsagePct-73.5) > 0.001 {
		t.Errorf("CacheUsagePct: got %.3f, want 73.500", got.CacheUsagePct)
	}
}

func TestParsePrometheusMetrics_LastSampleWins(t *testing.T) {
	body := `vllm:gpu_cache_usage_perc{a="1"} 0.10
vllm:gpu_cache_usage_perc{a="2"} 0.50
vllm:num_requests_waiting 5
vllm:num_requests_waiting 9
`
	got := parsePrometheusMetrics(body)
	if math.Abs(got.CacheUsagePct-50.0) > 0.001 {
		t.Errorf("CacheUsagePct: got %.3f, want 50.000 (last sample)", got.CacheUsagePct)
	}
	if got.RequestsWaiting != 9 {
		t.Errorf("RequestsWaiting: got %d, want 9 (last sample)", got.RequestsWaiting)
	}
}

func TestParsePrometheusMetrics_IgnoresUnknownAndComments(t *testing.T) {
	body := `# HELP something unrelated
# TYPE other_metric counter
other_metric{x="y"} 42
vllm:gpu_cache_usage_perc 0.05
junk line that does not parse
`
	got := parsePrometheusMetrics(body)
	if !got.HasCacheUsage {
		t.Fatal("expected HasCacheUsage=true despite noisy body")
	}
	if math.Abs(got.CacheUsagePct-5.0) > 0.001 {
		t.Errorf("CacheUsagePct: got %.3f, want 5.000", got.CacheUsagePct)
	}
}

func TestParsePrometheusMetrics_EmptyBodyHasNoCacheSignal(t *testing.T) {
	got := parsePrometheusMetrics("")
	if got.HasCacheUsage {
		t.Error("HasCacheUsage should be false on empty body")
	}
}

func TestParsePrometheusMetrics_TimestampedSampleIsRead(t *testing.T) {
	// Prometheus permits a trailing millisecond timestamp on each sample.
	body := `vllm:gpu_cache_usage_perc 0.42 1735689600000`
	got := parsePrometheusMetrics(body)
	if math.Abs(got.CacheUsagePct-42.0) > 0.001 {
		t.Errorf("CacheUsagePct: got %.3f, want 42.000 (timestamp must be ignored)",
			got.CacheUsagePct)
	}
}

func TestSplitMetricLine(t *testing.T) {
	cases := []struct {
		in        string
		wantName  string
		wantValue string
		wantOK    bool
	}{
		{`metric_a 1.5`, "metric_a", "1.5", true},
		{`metric_b{l="v"} 0.42`, "metric_b", "0.42", true},
		{`metric_c{a="1",b="2"} 9 1735689600000`, "metric_c", "9", true},
		{`# comment`, "", "", false},
		{``, "", "", false},
		{`name_only`, "", "", false},
		{`malformed{no_close 1`, "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			n, v, ok := splitMetricLine(tc.in)
			if ok != tc.wantOK {
				t.Errorf("ok: got %v, want %v", ok, tc.wantOK)
			}
			if n != tc.wantName {
				t.Errorf("name: got %q, want %q", n, tc.wantName)
			}
			if v != tc.wantValue {
				t.Errorf("value: got %q, want %q", v, tc.wantValue)
			}
		})
	}
}

func TestNormalizeMetricName(t *testing.T) {
	cases := map[string]string{
		"vllm:gpu_cache_usage_perc": "gpu_cache_usage_perc",
		"vllm_gpu_cache_usage_perc": "gpu_cache_usage_perc",
		"gpu_cache_usage_perc":      "gpu_cache_usage_perc",
		"other_metric":              "other_metric",
	}
	for in, want := range cases {
		if got := normalizeMetricName(in); got != want {
			t.Errorf("normalizeMetricName(%q) = %q, want %q", in, got, want)
		}
	}
}
