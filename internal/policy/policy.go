// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"context"
	"time"

	"github.com/actordock/actordock/internal/signals"
	"github.com/actordock/actordock/internal/types"
)

// PlaceRequest asks a policy where to run a sandbox.
type PlaceRequest struct {
	SandboxID      string
	Workers        []types.Worker
	Running        []types.Sandbox
	SandboxSignals map[string]signals.SandboxSignals
	WorkerSignals  map[string]signals.WorkerResource
}

// PlaceResult is either a direct placement or a suspend-then-place plan.
type PlaceResult struct {
	WorkerID string
	VictimID string
	Reason   string
}

// ResumeRequest asks which Worker should restore a suspended sandbox.
type ResumeRequest struct {
	Sandbox        types.Sandbox
	Workers        []types.Worker
	Running        []types.Sandbox
	SandboxSignals map[string]signals.SandboxSignals
	WorkerSignals  map[string]signals.WorkerResource
	// Waiting is the set of sandboxes currently blocked in Resume (including Sandbox).
	// semantic-score uses this so only the highest-score knocker may Place/Evict;
	// other policies ignore it.
	Waiting []types.Sandbox
	// WaitingSince maps sandbox ID → when it joined the Resume lobby.
	// Used for queue-age boost so long waiters eventually become top-ranked.
	WaitingSince map[string]time.Time
}

// Policy chooses placement, eviction, and resume targets.
type Policy interface {
	Name() string
	Place(ctx context.Context, req PlaceRequest) (PlaceResult, error)
	Resume(ctx context.Context, req ResumeRequest) (PlaceResult, error)
}
