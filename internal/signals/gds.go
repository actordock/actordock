// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package signals

// GreedyDual-Size keep-alive helpers (FaasCache / Cao & Irani GD-Size).
// H = L + Cost/Size; evict minimum H. Frequency is fixed at 1 (GD-Size, not GDSF).

const (
	gdsDefaultCost = 1.0
	gdsDefaultSize = 1.0 // MiB units when unknown
	bytesPerMiB    = 1024 * 1024
)

// GDSCost is re-materialization cost in seconds (cold-start analogue).
// Prefers preempt+restore; falls back to last checkpoint duration; else 1.
func GDSCost(snap SnapshotResource) float64 {
	c := snap.LastPreemptCostSec + snap.LastRestoreDur.Seconds()
	if c > 0 {
		return c
	}
	if snap.LastCheckpointDur > 0 {
		return snap.LastCheckpointDur.Seconds()
	}
	return gdsDefaultCost
}

// GDSSize is memory footprint in MiB (FaasCache "size").
// Prefers live RSS; falls back to last checkpoint bytes; else 1.
func GDSSize(runtime RuntimeResource, snap SnapshotResource) float64 {
	if runtime.MemRSSBytes > 0 {
		s := float64(runtime.MemRSSBytes) / bytesPerMiB
		if s < gdsDefaultSize {
			return gdsDefaultSize
		}
		return s
	}
	if snap.LastCheckpointBytes > 0 {
		s := float64(snap.LastCheckpointBytes) / bytesPerMiB
		if s < gdsDefaultSize {
			return gdsDefaultSize
		}
		return s
	}
	return gdsDefaultSize
}

// GDSPriority returns L + Cost/Size.
func GDSPriority(L float64, runtime RuntimeResource, snap SnapshotResource) float64 {
	return L + GDSCost(snap)/GDSSize(runtime, snap)
}
