// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package workerresource

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	cpuMu       sync.Mutex
	prevCPUUse  = map[string]uint64{}
	prevCPUTime = map[string]time.Time{}
)

// ReadSandboxCgroup tries cgroup v2 paths for sandboxID.
func ReadSandboxCgroup(sandboxID string) (cpuUtil float64, memRSS uint64, ok bool) {
	memRSS, memOK := readMemoryCgroup(sandboxID)
	cpuUtil, cpuOK := readCPUUtil(sandboxID)
	return cpuUtil, memRSS, memOK || cpuOK
}

func readMemoryCgroup(sandboxID string) (uint64, bool) {
	candidates := []string{
		filepath.Join("/sys/fs/cgroup", sandboxID, "memory.current"),
		filepath.Join("/sys/fs/cgroup/system.slice", "runsc-"+sandboxID+".scope", "memory.current"),
	}
	for _, p := range candidates {
		if v, ok := readUintFile(p); ok {
			return v, true
		}
	}
	var found uint64
	var got bool
	_ = filepath.WalkDir("/sys/fs/cgroup", func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() || !strings.Contains(d.Name(), sandboxID) {
			return nil
		}
		if v, ok := readUintFile(filepath.Join(path, "memory.current")); ok {
			found = v
			got = true
			return filepath.SkipAll
		}
		return nil
	})
	return found, got
}

func readCPUUtil(sandboxID string) (float64, bool) {
	use, ok := readCPUUseMicros(sandboxID)
	if !ok {
		return 0, false
	}
	now := time.Now()
	cpuMu.Lock()
	defer cpuMu.Unlock()
	prevUse := prevCPUUse[sandboxID]
	prevAt := prevCPUTime[sandboxID]
	prevCPUUse[sandboxID] = use
	prevCPUTime[sandboxID] = now
	if prevAt.IsZero() {
		return 0, true
	}
	dt := now.Sub(prevAt).Seconds()
	if dt <= 0 {
		return 0, true
	}
	delta := float64(use - prevUse)
	if use < prevUse {
		delta = float64(use)
	}
	util := delta / (dt * 1e6) // micros to core-seconds
	if util < 0 {
		util = 0
	}
	if util > 1 {
		util = 1
	}
	return util, true
}

func readCPUUseMicros(sandboxID string) (uint64, bool) {
	candidates := []string{
		filepath.Join("/sys/fs/cgroup", sandboxID, "cpu.stat"),
		filepath.Join("/sys/fs/cgroup/system.slice", "runsc-"+sandboxID+".scope", "cpu.stat"),
	}
	for _, p := range candidates {
		if v, ok := parseCPUStatUsage(p); ok {
			return v, true
		}
	}
	return 0, false
}

func parseCPUStatUsage(path string) (uint64, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "usage_usec ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				v, err := strconv.ParseUint(fields[1], 10, 64)
				return v, err == nil
			}
		}
	}
	return 0, false
}

func readUintFile(path string) (uint64, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	return v, err == nil
}
