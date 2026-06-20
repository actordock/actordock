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
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/actordock/actordock/internal/metrics"
)

// ProcCgroupReader reads cgroup stats for the current process.
type ProcCgroupReader struct{}

func NewProcCgroupReader() *ProcCgroupReader {
	return &ProcCgroupReader{}
}

func (ProcCgroupReader) Read() (CgroupStats, error) {
	stats := defaultCgroupStats()
	stats.CPUCount = runtime.NumCPU()
	if stats.CPUCount <= 0 {
		stats.CPUCount = 1
	}

	memUsed, memTotal, memCache, err := readCgroupMemory()
	if err == nil {
		stats.MemUsed = memUsed
		if memTotal > 0 {
			stats.MemTotal = memTotal
		}
		stats.MemCache = memCache
	}
	if cpuUsec, err := readCgroupCPUUsage(); err == nil {
		stats.CPUUsageUsec = cpuUsec
	}
	if diskUsed, diskTotal, err := readDiskUsage("/"); err == nil {
		stats.DiskUsed = diskUsed
		if diskTotal > 0 {
			stats.DiskTotal = diskTotal
		}
	}
	return stats, nil
}

// FakeCgroupReader returns fixed stats for tests.
type FakeCgroupReader struct {
	MemUsed      int64
	MemTotal     int64
	MemCache     int64
	DiskUsed     int64
	DiskTotal    int64
	CPUUsageUsec uint64
	CPUCount     int
}

func (f FakeCgroupReader) Read() (CgroupStats, error) {
	cpuCount := f.CPUCount
	if cpuCount <= 0 {
		cpuCount = metrics.DefaultCPUCount
	}
	return CgroupStats{
		MemUsed:      f.MemUsed,
		MemTotal:     f.MemTotal,
		MemCache:     f.MemCache,
		DiskUsed:     f.DiskUsed,
		DiskTotal:    f.DiskTotal,
		CPUUsageUsec: f.CPUUsageUsec,
		CPUCount:     cpuCount,
	}, nil
}

func readCgroupMemory() (used, total, cache int64, err error) {
	path, err := cgroupPath()
	if err != nil {
		return 0, 0, 0, err
	}
	used, err = readInt64File(filepath.Join(path, "memory.current"))
	if err != nil {
		used, err = readInt64File(filepath.Join(path, "memory.usage_in_bytes"))
		if err != nil {
			return 0, 0, 0, err
		}
	}
	total, _ = readInt64File(filepath.Join(path, "memory.max"))
	if total <= 0 {
		total, _ = readInt64File(filepath.Join(path, "memory.limit_in_bytes"))
	}
	cache = readMemoryStat(filepath.Join(path, "memory.stat"), "inactive_file")
	if cache == 0 {
		cache = readMemoryStat(filepath.Join(path, "memory.stat"), "cache")
	}
	return used, total, cache, nil
}

func readCgroupCPUUsage() (uint64, error) {
	path, err := cgroupPath()
	if err != nil {
		return 0, err
	}
	data, err := os.ReadFile(filepath.Join(path, "cpu.stat"))
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "usage_usec" {
			return strconv.ParseUint(fields[1], 10, 64)
		}
	}
	return 0, fmt.Errorf("usage_usec not found")
}

func cgroupPath() (string, error) {
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		if parts[0] == "0" && parts[1] == "" && parts[2] != "" {
			return filepath.Join("/sys/fs/cgroup", strings.TrimPrefix(parts[2], "/")), nil
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 || parts[2] == "" {
			continue
		}
		if strings.Contains(parts[1], "memory") {
			return filepath.Join("/sys/fs/cgroup/memory", parts[2]), nil
		}
	}
	return "", fmt.Errorf("cgroup path not found")
}

func readInt64File(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	raw := strings.TrimSpace(string(data))
	if raw == "max" {
		return 0, nil
	}
	return strconv.ParseInt(raw, 10, 64)
}

func readMemoryStat(path, key string) int64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == key {
			v, err := strconv.ParseInt(fields[1], 10, 64)
			if err == nil {
				return v
			}
		}
	}
	return 0
}

func readDiskUsage(path string) (used, total int64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	blockSize := int64(stat.Bsize)
	total = int64(stat.Blocks) * blockSize
	free := int64(stat.Bfree) * blockSize
	used = total - free
	return used, total, nil
}
