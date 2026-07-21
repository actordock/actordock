// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"sort"

	"github.com/actordock/actordock/internal/signals"
	"github.com/actordock/actordock/internal/types"
)

// pickIdleWorker chooses an idle Worker using registry slots + worker signal load.
func pickIdleWorker(workers []types.Worker, workerSig map[string]signals.WorkerResource, running []types.Sandbox, sandboxSig map[string]signals.SandboxSignals) (types.Worker, bool) {
	for _, w := range sortWorkersByLoad(workers, workerSig, running, sandboxSig) {
		if !workerEligibleForPlace(w, workerSig) {
			continue
		}
		if workerHostingCheckpoint(w.ID, running, sandboxSig) {
			continue
		}
		return w, true
	}
	return types.Worker{}, false
}

// idleWorkerCandidates is the set policies like random draw from.
func idleWorkerCandidates(workers []types.Worker, workerSig map[string]signals.WorkerResource, running []types.Sandbox, sandboxSig map[string]signals.SandboxSignals) []types.Worker {
	out := make([]types.Worker, 0, len(workers))
	for _, w := range workers {
		if !workerEligibleForPlace(w, workerSig) {
			continue
		}
		if workerHostingCheckpoint(w.ID, running, sandboxSig) {
			continue
		}
		out = append(out, w)
	}
	return out
}

func workerEligibleForPlace(w types.Worker, workerSig map[string]signals.WorkerResource) bool {
	if !w.Healthy || w.FreeSlots() <= 0 {
		return false
	}
	if workerSig != nil {
		if sig, ok := workerSig[w.ID]; ok && !sig.Healthy {
			return false
		}
	}
	return true
}

func workerHostingCheckpoint(workerID string, running []types.Sandbox, sandboxSig map[string]signals.SandboxSignals) bool {
	for _, sb := range running {
		if sb.WorkerID != workerID {
			continue
		}
		if sandboxSig == nil {
			return false
		}
		sig, ok := sandboxSig[sb.ID]
		return ok && sig.Snapshot.CheckpointInProgress
	}
	return false
}

func workerLoadScore(w types.Worker, workerSig map[string]signals.WorkerResource) float64 {
	if workerSig == nil {
		return 0
	}
	sig, ok := workerSig[w.ID]
	if !ok {
		return 0
	}
	return sig.CPUUtil + sig.MemUtil
}

func sortWorkersByLoad(workers []types.Worker, workerSig map[string]signals.WorkerResource, running []types.Sandbox, sandboxSig map[string]signals.SandboxSignals) []types.Worker {
	out := append([]types.Worker(nil), workers...)
	sort.Slice(out, func(i, j int) bool {
		wi, wj := out[i], out[j]
		busyI := workerHostingCheckpoint(wi.ID, running, sandboxSig)
		busyJ := workerHostingCheckpoint(wj.ID, running, sandboxSig)
		if busyI != busyJ {
			return !busyI
		}
		li, lj := workerLoadScore(wi, workerSig), workerLoadScore(wj, workerSig)
		if li != lj {
			return li < lj
		}
		if wi.RegisteredAt.Equal(wj.RegisteredAt) {
			return wi.ID < wj.ID
		}
		return wi.RegisteredAt.Before(wj.RegisteredAt)
	})
	return out
}
