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

import "time"

const (
	DefaultCPUCount   = 2
	DefaultMemoryMB   = 512
	DefaultDiskSizeMB = 512
)

// Sample is an E2B SandboxMetric snapshot.
type Sample struct {
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

// LatestResponse is the envd internal GET /metrics payload.
type LatestResponse struct {
	Metric Sample `json:"metric"`
}

// HistoryResponse is the envd internal GET /metrics/history payload.
type HistoryResponse struct {
	Metrics []Sample `json:"metrics"`
}

// NewSample builds a metric sample at the given time.
func NewSample(now time.Time, cpuCount int, cpuUsedPct float64, memUsed, memTotal, memCache, diskUsed, diskTotal int64) Sample {
	now = now.UTC()
	return Sample{
		Timestamp:     now.Format(time.RFC3339),
		TimestampUnix: now.Unix(),
		CPUCount:      cpuCount,
		CPUUsedPct:    cpuUsedPct,
		MemUsed:       memUsed,
		MemTotal:      memTotal,
		MemCache:      memCache,
		DiskUsed:      diskUsed,
		DiskTotal:     diskTotal,
	}
}

// FallbackSample returns zero-usage defaults when envd is unreachable.
func FallbackSample(now time.Time) Sample {
	return NewSample(
		now,
		DefaultCPUCount,
		0,
		0,
		int64(DefaultMemoryMB)*1024*1024,
		0,
		0,
		int64(DefaultDiskSizeMB)*1024*1024,
	)
}
