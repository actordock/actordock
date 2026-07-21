// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package signals

import "time"

// RuntimeResource describes live resource behavior of a running sandbox.
type RuntimeResource struct {
	CPUUtil      float64   `json:"cpuUtil"` // observability only; not used by GDS eviction
	MemRSSBytes  uint64    `json:"memRSSBytes"`
	LastActiveAt time.Time `json:"lastActiveAt"`
}

// SnapshotResource describes checkpoint/snapshot cost and in-flight state.
type SnapshotResource struct {
	CheckpointInProgress bool          `json:"checkpointInProgress"`
	LastCheckpointBytes  uint64        `json:"lastCheckpointBytes"`
	LastPreemptCostSec   float64       `json:"lastPreemptCostSec"`
	LastCheckpointAt     time.Time     `json:"lastCheckpointAt"` // observability
	LastCheckpointDur    time.Duration `json:"lastCheckpointDur"`
	LastRestoreAt        time.Time     `json:"lastRestoreAt"` // observability
	LastRestoreDur       time.Duration `json:"lastRestoreDur"`
}

// WorkerResource describes live Worker/Pod capacity and pressure (one row per workerID).
type WorkerResource struct {
	WorkerID   string    `json:"workerID"`
	MaxSlots   int       `json:"maxSlots"`
	UsedSlots  int       `json:"usedSlots"`
	Healthy    bool      `json:"healthy"`
	CPUUtil    float64   `json:"cpuUtil"`
	MemUtil    float64   `json:"memUtil"`
	MemBytes   uint64    `json:"memBytes"`
	ReportedAt time.Time `json:"reportedAt"`
}

// SandboxSignals is runtime + snapshot for one sandbox (scheduling view).
type SandboxSignals struct {
	SandboxID  string    `json:"sandboxID"`
	WorkerID   string    `json:"workerID"`
	ReportedAt time.Time `json:"reportedAt"`

	Runtime  RuntimeResource  `json:"runtime"`
	Snapshot SnapshotResource `json:"snapshot"`

	// KeepAliveH is FaasCache/GreedyDual-Size priority (evict minimum H).
	KeepAliveH float64 `json:"keepAliveH"`

	// Legacy JSON flat fields; normalized into runtime on ingest.
	CPUUtil      float64   `json:"cpuUtil,omitempty"`
	MemRSSBytes  uint64    `json:"memRSSBytes,omitempty"`
	LastActiveAt time.Time `json:"lastActiveAt,omitempty"`
}

// Push is the Worker → control plane resource plugin body.
type Push struct {
	WorkerID string           `json:"workerID"`
	Worker   WorkerResource   `json:"worker"`
	Samples  []SandboxSignals `json:"samples"`
}

// NormalizeLegacy copies flat JSON fields into runtime.
func (s *SandboxSignals) NormalizeLegacy() {
	if s.Runtime.CPUUtil == 0 {
		s.Runtime.CPUUtil = s.CPUUtil
	}
	if s.Runtime.MemRSSBytes == 0 {
		s.Runtime.MemRSSBytes = s.MemRSSBytes
	}
	if s.Runtime.LastActiveAt.IsZero() {
		s.Runtime.LastActiveAt = s.LastActiveAt
	}
}
