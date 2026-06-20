// Copyright 2026 The Actordock Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package platform

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseLogsV1Query(t *testing.T) {
	t.Parallel()

	cases := []struct {
		query string
		ok    bool
	}{
		{"", true},
		{"start=0&limit=100", true},
		{"start=-1", false},
		{"start=bad", false},
		{"limit=-1", false},
		{"limit=bad", false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1/logs?"+tc.query, nil)
		err := parseLogsV1Query(req)
		if tc.ok && err != nil {
			t.Fatalf("query %q: err = %v, want nil", tc.query, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("query %q: err = nil, want error", tc.query)
		}
	}
}

func TestParseLogsV2Query(t *testing.T) {
	t.Parallel()

	cases := []struct {
		query string
		ok    bool
	}{
		{"", true},
		{"cursor=0&limit=100&direction=forward&level=info&search=foo", true},
		{"direction=backward", true},
		{"cursor=-1", false},
		{"limit=1001", false},
		{"direction=up", false},
		{"level=trace", false},
		{"search=" + strings.Repeat("a", maxLogSearchLen+1), false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/v2/sandboxes/sb-1/logs?"+tc.query, nil)
		err := parseLogsV2Query(req)
		if tc.ok && err != nil {
			t.Fatalf("query %q: err = %v, want nil", tc.query, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("query %q: err = nil, want error", tc.query)
		}
	}
}

func TestBuildStubSandboxLogs(t *testing.T) {
	t.Parallel()

	v1 := buildStubSandboxLogs()
	if v1.Logs == nil || v1.LogEntries == nil {
		t.Fatalf("stub v1 has nil slices: %+v", v1)
	}
	if len(v1.Logs) != 0 || len(v1.LogEntries) != 0 {
		t.Fatalf("stub v1 not empty: %+v", v1)
	}

	v2 := buildStubSandboxLogsV2()
	if v2.Logs == nil {
		t.Fatalf("stub v2 logs is nil")
	}
	if len(v2.Logs) != 0 {
		t.Fatalf("stub v2 not empty: %+v", v2)
	}

	b, err := json.Marshal(v1)
	if err != nil {
		t.Fatalf("marshal v1: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal v1: %v", err)
	}
	for _, key := range []string{"logs", "logEntries"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("missing key %q in %s", key, string(b))
		}
	}
}

func TestSandboxLogEntryJSONFields(t *testing.T) {
	t.Parallel()

	entry := sandboxLogEntryResponse{
		Timestamp: "2026-06-20T12:00:00Z",
		Message:   "hello",
		Level:     "info",
		Fields:    map[string]string{"k": "v"},
	}
	b, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"timestamp", "message", "level", "fields"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("missing key %q in %s", key, string(b))
		}
	}
}

func TestSandboxLogJSONFields(t *testing.T) {
	t.Parallel()

	entry := sandboxLogResponse{
		Timestamp: "2026-06-20T12:00:00Z",
		Line:      "hello",
	}
	b, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"timestamp", "line"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("missing key %q in %s", key, string(b))
		}
	}
}
