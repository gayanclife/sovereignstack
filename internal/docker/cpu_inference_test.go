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
package docker

import (
	"strings"
	"testing"
)

// The CPU inference container's behavior lives in an embedded Python literal
// that we can't run from a Go test without Docker. These tests are a
// lightweight safety net: they catch typos, dropped lines, or accidental
// removal of the architecture map / capability endpoints in cpu_inference.go.
// If the literal stops mentioning a capability the dispatcher in cmd/test.go
// expects, those integration paths break silently — these tests fail loudly.

func TestCPUServerPython_ContainsAllExpectedArchitectures(t *testing.T) {
	wantArchSuffixes := []string{
		"ForCausalLM",
		"LMHeadModel",
		"ForConditionalGeneration",
		"ForSeq2SeqLM",
		"ForSequenceClassification",
		"ForTokenClassification",
		"ForQuestionAnswering",
		"ForMaskedLM",
	}
	for _, s := range wantArchSuffixes {
		if !strings.Contains(cpuServerPython, s) {
			t.Errorf("ARCH_MAP missing architecture suffix %q", s)
		}
	}
}

func TestCPUServerPython_DeclaresAllCapabilities(t *testing.T) {
	// Mirrors the values the cmd/test.go dispatcher switches on.
	wantCapabilities := []string{"generative", "seq2seq", "encoder", "classification"}
	for _, c := range wantCapabilities {
		if !strings.Contains(cpuServerPython, `"`+c+`"`) {
			t.Errorf("ENDPOINTS map missing capability %q", c)
		}
	}
}

func TestCPUServerPython_RegistersExpectedEndpoints(t *testing.T) {
	wantEndpoints := []string{
		"/v1/models",
		"/v1/chat/completions",
		"/v1/completions",
		"/v1/embeddings",
		"/v1/classify",
	}
	for _, ep := range wantEndpoints {
		if !strings.Contains(cpuServerPython, ep) {
			t.Errorf("server script does not register %q", ep)
		}
	}
}

func TestCPUServerPython_LoadsLocallyOnly(t *testing.T) {
	// Containers ship without the network reaching out to HF at runtime;
	// regressions here would silently re-enable network downloads.
	if !strings.Contains(cpuServerPython, "local_files_only=True") {
		t.Error("model loader must use local_files_only=True (no runtime HF fetch)")
	}
}

func TestCPUServerPython_DtypeFallback(t *testing.T) {
	// transformers >= 4.43 wants 'dtype'; older versions only know 'torch_dtype'.
	// The script must try the new name and fall back to the old one.
	if !strings.Contains(cpuServerPython, "dtype=torch.float32") {
		t.Error("missing primary dtype= kwarg path for transformers >= 4.43")
	}
	if !strings.Contains(cpuServerPython, "torch_dtype=torch.float32") {
		t.Error("missing torch_dtype= fallback path for older transformers")
	}
	if !strings.Contains(cpuServerPython, "except TypeError") {
		t.Error("missing TypeError fallback between dtype and torch_dtype")
	}
}
