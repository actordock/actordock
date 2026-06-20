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
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxSandboxMetricsIDs = 100

var (
	errMissingSandboxIDs   = errors.New("sandbox_ids is required")
	errInvalidSandboxIDs   = errors.New("invalid sandbox_ids")
	errInvalidMetricsQuery = errors.New("invalid metrics query")
)

type sandboxMetricResponse struct {
	Timestamp     string  `json:"timestamp"`
	TimestampUnix int64   `json:"timestampUnix"`
	CPUCount      int     `json:"cpuCount"`
	CPUUsedPct    float64 `json:"cpuUsedPct"`
	MemUsed       int64   `json:"memUsed"`
	MemTotal      int64   `json:"memTotal"`
	MemCache      int64   `json:"memCache"`
	DiskUsed      int64   `json:"diskUsed"`
	DiskTotal     int64   `json:"diskTotal"`
}

type sandboxesWithMetricsResponse struct {
	Sandboxes map[string]sandboxMetricResponse `json:"sandboxes"`
}

func buildStubSandboxMetric(now time.Time) sandboxMetricResponse {
	return sandboxMetricResponse{
		Timestamp:     now.UTC().Format(time.RFC3339),
		TimestampUnix: now.Unix(),
		CPUCount:      defaultCPUCount,
		CPUUsedPct:    0,
		MemUsed:       0,
		MemTotal:      int64(defaultMemoryMB) * 1024 * 1024,
		MemCache:      0,
		DiskUsed:      0,
		DiskTotal:     int64(defaultDiskSizeMB) * 1024 * 1024,
	}
}

func parseSandboxIDs(r *http.Request) ([]string, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("sandbox_ids"))
	if raw == "" {
		return nil, errMissingSandboxIDs
	}

	parts := strings.Split(raw, ",")
	ids := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" {
			return nil, errInvalidSandboxIDs
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, errInvalidSandboxIDs
	}
	if len(ids) > maxSandboxMetricsIDs {
		return nil, errInvalidSandboxIDs
	}
	return ids, nil
}

func parseMetricsIntervalQuery(r *http.Request) error {
	for _, key := range []string{"start", "end"} {
		raw := strings.TrimSpace(r.URL.Query().Get(key))
		if raw == "" {
			continue
		}
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			return errInvalidMetricsQuery
		}
	}
	return nil
}
