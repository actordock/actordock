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

package envd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/actordock/actordock/internal/metrics"
)

func TestCollectorSnapshot(t *testing.T) {
	t.Parallel()

	reader := &incrCPUReader{}
	collector := NewCollector(reader)
	first := collector.Snapshot(time.Unix(100, 0))
	if first.MemUsed != 100 || first.MemTotal != 200 {
		t.Fatalf("first = %+v", first)
	}
	second := collector.Snapshot(time.Unix(101, 0))
	if second.CPUUsedPct <= 0 {
		t.Fatalf("cpuUsedPct = %v, want > 0", second.CPUUsedPct)
	}
}

type incrCPUReader struct {
	cpuUsec uint64
}

func (r *incrCPUReader) Read() (CgroupStats, error) {
	r.cpuUsec += 500_000
	return CgroupStats{
		MemUsed:      100,
		MemTotal:     200,
		CPUUsageUsec: r.cpuUsec,
		CPUCount:     2,
	}, nil
}

func TestMetricsHandler(t *testing.T) {
	t.Parallel()

	collector := stubMetricsCollector()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", NewMetricsHandler(collector))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp metrics.LatestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Metric.MemUsed == 0 || resp.Metric.MemTotal == 0 {
		t.Fatalf("metric = %+v", resp.Metric)
	}
}

func TestMetricsHistoryHandler(t *testing.T) {
	t.Parallel()

	collector := stubMetricsCollector()
	collector.Snapshot(time.Unix(10, 0))
	collector.Snapshot(time.Unix(20, 0))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics/history", NewMetricsHistoryHandler(collector))

	req := httptest.NewRequest(http.MethodGet, "/metrics/history?start=0&end=15", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp metrics.HistoryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Metrics) != 1 || resp.Metrics[0].TimestampUnix != 10 {
		t.Fatalf("metrics = %+v", resp.Metrics)
	}
}

func TestFetchMetric(t *testing.T) {
	t.Parallel()

	collector := stubMetricsCollector()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", NewMetricsHandler(collector))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	sample, err := FetchMetric(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchMetric: %v", err)
	}
	if sample.MemUsed == 0 {
		t.Fatalf("sample = %+v", sample)
	}
}
