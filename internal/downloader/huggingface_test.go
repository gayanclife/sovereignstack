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
package downloader

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestParseShardManifest(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected []string
	}{
		{
			name: "two unique shards",
			body: `{"weight_map":{
				"layer.0.weight":"model-00001-of-00002.safetensors",
				"layer.1.weight":"model-00002-of-00002.safetensors"
			}}`,
			expected: []string{
				"model-00001-of-00002.safetensors",
				"model-00002-of-00002.safetensors",
			},
		},
		{
			name: "duplicate shard names dedup to one",
			body: `{"weight_map":{
				"layer.0.weight":"model-00001-of-00001.safetensors",
				"layer.1.weight":"model-00001-of-00001.safetensors",
				"layer.2.weight":"model-00001-of-00001.safetensors"
			}}`,
			expected: []string{"model-00001-of-00001.safetensors"},
		},
		{
			name:     "empty weight_map yields nil",
			body:     `{"weight_map":{}}`,
			expected: nil,
		},
		{
			name:     "missing weight_map yields nil",
			body:     `{}`,
			expected: nil,
		},
		{
			name:     "malformed JSON yields nil",
			body:     `{not json`,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseShardManifest([]byte(tt.body))
			// Map iteration order is non-deterministic; sort both sides.
			sort.Strings(got)
			expected := append([]string(nil), tt.expected...)
			sort.Strings(expected)

			if len(got) != len(expected) {
				t.Fatalf("len mismatch: got %d (%v), want %d (%v)",
					len(got), got, len(expected), expected)
			}
			for i := range got {
				if got[i] != expected[i] {
					t.Errorf("[%d] got %q, want %q", i, got[i], expected[i])
				}
			}
		})
	}
}

func TestValidateSafetensorsFile(t *testing.T) {
	dir := t.TempDir()

	t.Run("valid minimal file", func(t *testing.T) {
		// 8-byte LE header size + JSON header + zero tensor bytes is the minimal
		// well-formed safetensors layout. transformers will refuse to load it,
		// but our validator only checks the framing.
		path := filepath.Join(dir, "valid.safetensors")
		header := []byte(`{"__metadata__":{}}`)
		buf := make([]byte, 8+len(header))
		binary.LittleEndian.PutUint64(buf[0:8], uint64(len(header)))
		copy(buf[8:], header)
		if err := os.WriteFile(path, buf, 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		if err := validateSafetensorsFile(path); err != nil {
			t.Errorf("expected valid, got error: %v", err)
		}
	})

	t.Run("file shorter than 8-byte header size", func(t *testing.T) {
		path := filepath.Join(dir, "tiny.safetensors")
		if err := os.WriteFile(path, []byte("hi"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		err := validateSafetensorsFile(path)
		if err == nil || !strings.Contains(err.Error(), "too small") {
			t.Errorf("expected 'too small' error, got %v", err)
		}
	})

	t.Run("zero header size is invalid", func(t *testing.T) {
		path := filepath.Join(dir, "zero-header.safetensors")
		buf := make([]byte, 8) // header size = 0
		if err := os.WriteFile(path, buf, 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		err := validateSafetensorsFile(path)
		if err == nil || !strings.Contains(err.Error(), "invalid safetensors header size") {
			t.Errorf("expected invalid-header error, got %v", err)
		}
	})

	t.Run("oversize header is rejected", func(t *testing.T) {
		path := filepath.Join(dir, "huge-header.safetensors")
		buf := make([]byte, 8)
		// 200 MB — over the 100 MB cap.
		binary.LittleEndian.PutUint64(buf[0:8], 200*1024*1024)
		if err := os.WriteFile(path, buf, 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		err := validateSafetensorsFile(path)
		if err == nil || !strings.Contains(err.Error(), "invalid safetensors header size") {
			t.Errorf("expected invalid-header error, got %v", err)
		}
	})

	t.Run("file truncated mid-header is incomplete", func(t *testing.T) {
		path := filepath.Join(dir, "truncated.safetensors")
		// Declares a 64-byte header but only provides 16 of those bytes.
		buf := make([]byte, 8+16)
		binary.LittleEndian.PutUint64(buf[0:8], 64)
		if err := os.WriteFile(path, buf, 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		err := validateSafetensorsFile(path)
		if err == nil || !strings.Contains(err.Error(), "incomplete file") {
			t.Errorf("expected incomplete-file error, got %v", err)
		}
	})

	t.Run("missing file errors", func(t *testing.T) {
		err := validateSafetensorsFile(filepath.Join(dir, "nope.safetensors"))
		if err == nil {
			t.Error("expected error for missing file, got nil")
		}
	})
}
