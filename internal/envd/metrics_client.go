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
	"fmt"
	"io"
	"net/http"

	"github.com/actordock/actordock/internal/metrics"
)

// FetchMetric calls envd GET /metrics.
func FetchMetric(ctx context.Context, baseURL string) (metrics.Sample, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normalizeBaseURL(baseURL)+MetricsPath, nil)
	if err != nil {
		return metrics.Sample{}, fmt.Errorf("build metrics request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return metrics.Sample{}, fmt.Errorf("fetch metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return metrics.Sample{}, fmt.Errorf("fetch metrics: status %d: %s", resp.StatusCode, string(body))
	}

	var payload metrics.LatestResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return metrics.Sample{}, fmt.Errorf("decode metrics: %w", err)
	}
	return payload.Metric, nil
}

// FetchMetricHistory calls envd GET /metrics/history with the given raw query string.
func FetchMetricHistory(ctx context.Context, baseURL, rawQuery string) ([]metrics.Sample, error) {
	url := normalizeBaseURL(baseURL) + MetricsHistoryPath
	if rawQuery != "" {
		url += "?" + rawQuery
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build metrics history request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch metrics history: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("fetch metrics history: status %d: %s", resp.StatusCode, string(body))
	}

	var payload metrics.HistoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode metrics history: %w", err)
	}
	return payload.Metrics, nil
}
