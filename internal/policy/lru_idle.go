// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"context"
	"fmt"
	"time"
)

// LRUIdle places like FIFO on idle Workers; when full, suspends the running sandbox
// with the longest runtime-idle (pure LRU, no snapshot cost weighting).
type LRUIdle struct{}

func NewLRUIdle() *LRUIdle { return &LRUIdle{} }

func (p *LRUIdle) Name() string { return "lru-idle" }

func (p *LRUIdle) Place(_ context.Context, req PlaceRequest) (PlaceResult, error) {
	if w, ok := pickIdleWorker(req.Workers, req.WorkerSignals, req.Running, req.SandboxSignals); ok {
		return PlaceResult{
			WorkerID: w.ID,
			Reason:   "lru-idle: idle worker (load-aware tie-break)",
		}, nil
	}
	if len(req.Running) == 0 {
		return PlaceResult{}, fmt.Errorf("lru-idle: no capacity and nothing to suspend")
	}
	victim := pickLRUVictim(req.Running, req.SandboxSignals, time.Now())
	return PlaceResult{
		WorkerID: victim.WorkerID,
		VictimID: victim.ID,
		Reason:   "lru-idle: suspend longest runtime-idle sandbox",
	}, nil
}

func (p *LRUIdle) Resume(ctx context.Context, req ResumeRequest) (PlaceResult, error) {
	if res, ok := tryStickyResume(req, "lru-idle: sticky resume to last idle worker"); ok {
		return res, nil
	}
	return p.Place(ctx, placeFromResume(req))
}
