// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package workerresource

// ReadSandboxCgroup returns cpuUtil 0 and mem 0 when cgroup stats are unavailable.
func ReadSandboxCgroup(_ string) (cpuUtil float64, memRSS uint64, ok bool) {
	return 0, 0, false
}
