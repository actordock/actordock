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

package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseHistoryQuery(t *testing.T) {
	t.Parallel()

	cases := []struct {
		query string
		ok    bool
	}{
		{"", true},
		{"start=0&end=100", true},
		{"start=-1", false},
		{"end=bad", false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/metrics/history?"+tc.query, nil)
		_, err := ParseHistoryQuery(req)
		if tc.ok && err != nil {
			t.Fatalf("query %q: err = %v, want nil", tc.query, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("query %q: err = nil, want error", tc.query)
		}
	}
}

func TestFallbackSample(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	m := FallbackSample(now)
	if m.Timestamp != "2026-06-20T12:00:00Z" {
		t.Fatalf("timestamp = %q", m.Timestamp)
	}
	if m.MemTotal != int64(DefaultMemoryMB)*1024*1024 {
		t.Fatalf("memTotal = %d", m.MemTotal)
	}
}

func TestFilterHistory(t *testing.T) {
	t.Parallel()

	samples := []Sample{
		{TimestampUnix: 10},
		{TimestampUnix: 20},
		{TimestampUnix: 30},
	}
	start := int64(15)
	end := int64(25)
	filtered := FilterHistory(samples, HistoryQuery{Start: &start, End: &end})
	if len(filtered) != 1 || filtered[0].TimestampUnix != 20 {
		t.Fatalf("filtered = %+v", filtered)
	}
}
