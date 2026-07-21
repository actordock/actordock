// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"math"
	"time"

	"github.com/actordock/actordock/internal/signals"
	"github.com/actordock/actordock/internal/types"
)

// pickLRUVictim suspends the sandbox with the longest runtime idle time.
func pickLRUVictim(running []types.Sandbox, sandboxSig map[string]signals.SandboxSignals, now time.Time) types.Sandbox {
	bestIdx := 0
	bestIdle := runtimeIdle(running[0], sandboxSig, now)
	for i := 1; i < len(running); i++ {
		d := runtimeIdle(running[i], sandboxSig, now)
		if d > bestIdle || (d == bestIdle && running[i].CreatedAt.Before(running[bestIdx].CreatedAt)) {
			bestIdx = i
			bestIdle = d
		}
	}
	return running[bestIdx]
}

// pickGDSVictim suspends the sandbox with the lowest FaasCache/GreedyDual-Size keep-alive H.
func pickGDSVictim(running []types.Sandbox, sandboxSig map[string]signals.SandboxSignals) types.Sandbox {
	bestIdx := 0
	bestH := keepAliveH(running[0], sandboxSig)
	for i := 1; i < len(running); i++ {
		h := keepAliveH(running[i], sandboxSig)
		if h < bestH || (h == bestH && running[i].CreatedAt.Before(running[bestIdx].CreatedAt)) {
			bestIdx = i
			bestH = h
		}
	}
	return running[bestIdx]
}

func runtimeIdle(sb types.Sandbox, sandboxSig map[string]signals.SandboxSignals, now time.Time) time.Duration {
	if sandboxSig != nil {
		if sig, ok := sandboxSig[sb.ID]; ok {
			if !sig.Runtime.LastActiveAt.IsZero() {
				return now.Sub(sig.Runtime.LastActiveAt)
			}
			if !sig.LastActiveAt.IsZero() {
				return now.Sub(sig.LastActiveAt)
			}
		}
	}
	if !sb.UpdatedAt.IsZero() {
		return now.Sub(sb.UpdatedAt)
	}
	return now.Sub(sb.CreatedAt)
}

func keepAliveH(sb types.Sandbox, sandboxSig map[string]signals.SandboxSignals) float64 {
	if sandboxSig == nil {
		return signals.GDSPriority(0, signals.RuntimeResource{}, signals.SnapshotResource{})
	}
	sig, ok := sandboxSig[sb.ID]
	if !ok {
		return signals.GDSPriority(0, signals.RuntimeResource{}, signals.SnapshotResource{})
	}
	if sig.Snapshot.CheckpointInProgress {
		return math.Inf(1) // never evict mid-checkpoint (FaasCache: do not terminate busy work)
	}
	if sig.KeepAliveH != 0 {
		return sig.KeepAliveH
	}
	// Ephemeral compute when Store has not yet assigned H (same L=0 GD-Size formula).
	return signals.GDSPriority(0, sig.Runtime, sig.Snapshot)
}
