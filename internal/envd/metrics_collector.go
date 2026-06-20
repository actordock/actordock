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
	"runtime"
	"sync"
	"time"

	"github.com/actordock/actordock/internal/metrics"
)

const defaultHistoryCap = 3600

// CgroupStats holds raw resource readings from the sandbox cgroup.
type CgroupStats struct {
	MemUsed      int64
	MemTotal     int64
	MemCache     int64
	DiskUsed     int64
	DiskTotal    int64
	CPUUsageUsec uint64
	CPUCount     int
}

// CgroupReader reads cgroup-backed resource usage for the sandbox.
type CgroupReader interface {
	Read() (CgroupStats, error)
}

// Collector samples cgroup stats and retains a bounded history.
type Collector struct {
	mu          sync.Mutex
	reader      CgroupReader
	history     []metrics.Sample
	maxHistory  int
	lastCPUUsec uint64
	lastCPUTime time.Time
}

func NewCollector(reader CgroupReader) *Collector {
	if reader == nil {
		reader = NewProcCgroupReader()
	}
	return &Collector{
		reader:     reader,
		maxHistory: defaultHistoryCap,
	}
}

func (c *Collector) Snapshot(now time.Time) metrics.Sample {
	c.mu.Lock()
	defer c.mu.Unlock()

	sample := c.snapshotLocked(now)
	c.history = append(c.history, sample)
	if len(c.history) > c.maxHistory {
		c.history = c.history[len(c.history)-c.maxHistory:]
	}
	return sample
}

func (c *Collector) Latest(now time.Time) metrics.Sample {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.history) == 0 {
		return c.snapshotLocked(now)
	}
	return c.history[len(c.history)-1]
}

func (c *Collector) History(q metrics.HistoryQuery) []metrics.Sample {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.history) == 0 {
		return []metrics.Sample{}
	}
	copied := append([]metrics.Sample(nil), c.history...)
	filtered := metrics.FilterHistory(copied, q)
	if filtered == nil {
		return []metrics.Sample{}
	}
	return filtered
}

func (c *Collector) snapshotLocked(now time.Time) metrics.Sample {
	stats, err := c.reader.Read()
	if err != nil {
		stats = defaultCgroupStats()
	}
	if stats.CPUCount <= 0 {
		stats.CPUCount = runtime.NumCPU()
		if stats.CPUCount <= 0 {
			stats.CPUCount = metrics.DefaultCPUCount
		}
	}

	cpuPct := 0.0
	if !c.lastCPUTime.IsZero() {
		deltaSec := now.Sub(c.lastCPUTime).Seconds()
		if deltaSec > 0 && stats.CPUUsageUsec >= c.lastCPUUsec {
			deltaUsec := stats.CPUUsageUsec - c.lastCPUUsec
			cpuPct = float64(deltaUsec) / 1e6 / deltaSec / float64(stats.CPUCount) * 100
			if cpuPct > 100 {
				cpuPct = 100
			}
		}
	}
	c.lastCPUUsec = stats.CPUUsageUsec
	c.lastCPUTime = now

	return metrics.NewSample(
		now,
		stats.CPUCount,
		cpuPct,
		stats.MemUsed,
		stats.MemTotal,
		stats.MemCache,
		stats.DiskUsed,
		stats.DiskTotal,
	)
}

func defaultCgroupStats() CgroupStats {
	return CgroupStats{
		MemTotal:  int64(metrics.DefaultMemoryMB) * 1024 * 1024,
		DiskTotal: int64(metrics.DefaultDiskSizeMB) * 1024 * 1024,
		CPUCount:  metrics.DefaultCPUCount,
	}
}

func stubMetricsCollector() *Collector {
	return NewCollector(FakeCgroupReader{
		MemUsed:      128 * 1024 * 1024,
		MemTotal:     int64(metrics.DefaultMemoryMB) * 1024 * 1024,
		MemCache:     32 * 1024 * 1024,
		DiskUsed:     1 * 1024 * 1024 * 1024,
		DiskTotal:    int64(metrics.DefaultDiskSizeMB) * 1024 * 1024,
		CPUUsageUsec: 1_000_000,
		CPUCount:     metrics.DefaultCPUCount,
	})
}
