// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"context"

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
}

// Policy chooses placement, eviction, and resume targets.
type Policy interface {
	Name() string
	Place(ctx context.Context, req PlaceRequest) (PlaceResult, error)
	Resume(ctx context.Context, req ResumeRequest) (PlaceResult, error)
}
