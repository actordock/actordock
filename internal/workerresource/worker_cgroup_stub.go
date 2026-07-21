// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package workerresource

// ReadWorkerCgroup returns zeros when cgroup stats are unavailable.
func ReadWorkerCgroup() (cpuUtil, memUtil float64, memBytes uint64, ok bool) {
	return 0, 0, 0, false
}
