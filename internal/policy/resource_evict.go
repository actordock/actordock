// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"context"
	"fmt"
)

// ResourceEvict places onto the least-loaded idle Worker; when full, suspends the
// running sandbox with the lowest FaasCache / GreedyDual-Size keep-alive priority H.
type ResourceEvict struct{}

func NewResourceEvict() *ResourceEvict { return &ResourceEvict{} }

func (p *ResourceEvict) Name() string { return "resource-evict" }

func (p *ResourceEvict) Place(_ context.Context, req PlaceRequest) (PlaceResult, error) {
	if w, ok := pickIdleWorker(req.Workers, req.WorkerSignals, req.Running, req.SandboxSignals); ok {
		return PlaceResult{
			WorkerID: w.ID,
			Reason:   "resource-evict: least-loaded idle worker",
		}, nil
	}
	if len(req.Running) == 0 {
		return PlaceResult{}, fmt.Errorf("resource-evict: no capacity and nothing to suspend")
	}

	victim := pickGDSVictim(req.Running, req.SandboxSignals)
	return PlaceResult{
		WorkerID: victim.WorkerID,
		VictimID: victim.ID,
		Reason:   "resource-evict: suspend lowest GreedyDual-Size keep-alive H",
	}, nil
}

func (p *ResourceEvict) Resume(ctx context.Context, req ResumeRequest) (PlaceResult, error) {
	if res, ok := tryStickyResume(req, "resource-evict: sticky resume to last idle worker"); ok {
		return res, nil
	}
	return p.Place(ctx, placeFromResume(req))
}
