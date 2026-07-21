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
	workerCPUMu    sync.Mutex
	workerPrevUse  uint64
	workerPrevTime time.Time
)

// ReadWorkerCgroup samples this process cgroup (Pod) CPU and memory pressure.
func ReadWorkerCgroup() (cpuUtil, memUtil float64, memBytes uint64, ok bool) {
	root, okRoot := selfCgroupRoot()
	if !okRoot {
		return 0, 0, 0, false
	}
	memBytes, memOK := readCgroupUint(filepath.Join(root, "memory.current"))
	limit, limitOK := readCgroupUint(filepath.Join(root, "memory.max"))
	if memOK && memBytes > 0 {
		if limitOK && limit > 0 {
			memUtil = float64(memBytes) / float64(limit)
		} else {
			// Kind often has memory.max=max; still report a positive utilization signal.
			const nominal = float64(1 << 30) // 1 GiB
			memUtil = float64(memBytes) / nominal
		}
		if memUtil > 1 {
			memUtil = 1
		}
	}
	cpuUtil, cpuOK := readCgroupCPUUtil(root)
	return cpuUtil, memUtil, memBytes, memOK || cpuOK
}

func selfCgroupRoot() (string, bool) {
	b, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// cgroup v2: 0::/path
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 || parts[0] != "0" {
			continue
		}
		rel := strings.TrimPrefix(parts[2], "/")
		if rel == "" {
			return "/sys/fs/cgroup", true
		}
		return filepath.Join("/sys/fs/cgroup", rel), true
	}
	return "", false
}

func readCgroupCPUUtil(root string) (float64, bool) {
	use, ok := parseCPUStatUsage(filepath.Join(root, "cpu.stat"))
	if !ok {
		return 0, false
	}
	now := time.Now()
	workerCPUMu.Lock()
	defer workerCPUMu.Unlock()
	prevUse := workerPrevUse
	prevAt := workerPrevTime
	workerPrevUse = use
	workerPrevTime = now
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
	util := delta / (dt * 1e6)
	if util < 0 {
		util = 0
	}
	if util > 1 {
		util = 1
	}
	return util, true
}

func readCgroupUint(path string) (uint64, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(b))
	if s == "max" {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 10, 64)
	return v, err == nil
}
