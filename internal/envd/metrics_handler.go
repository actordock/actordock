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
	"encoding/json"
	"net/http"
	"time"

	"github.com/actordock/actordock/internal/metrics"
)

const (
	MetricsPath        = "/metrics"
	MetricsHistoryPath = "/metrics/history"
)

// NewMetricsHandler serves envd internal GET /metrics.
func NewMetricsHandler(collector *Collector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sample := collector.Snapshot(time.Now().UTC())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metrics.LatestResponse{Metric: sample})
	}
}

// NewMetricsHistoryHandler serves envd internal GET /metrics/history.
func NewMetricsHistoryHandler(collector *Collector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, err := metrics.ParseHistoryQuery(r)
		if err != nil {
			http.Error(w, "invalid metrics query", http.StatusBadRequest)
			return
		}
		samples := collector.History(q)
		if samples == nil {
			samples = []metrics.Sample{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metrics.HistoryResponse{Metrics: samples})
	}
}
