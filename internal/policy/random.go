// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// Random places onto a random idle Worker (one running sandbox per Worker).
// When the pool is full, it suspends a random running sandbox and reuses that Worker.
type Random struct {
	rng *rand.Rand
}

// NewRandom returns a Random policy. If rng is nil, a default source is used.
func NewRandom(rng *rand.Rand) *Random {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &Random{rng: rng}
}

func (p *Random) Name() string { return "random" }

func (p *Random) Place(_ context.Context, req PlaceRequest) (PlaceResult, error) {
	candidates := idleWorkerCandidates(req.Workers, req.WorkerSignals, req.Running, req.SandboxSignals)
	if len(candidates) > 0 {
		w := candidates[p.rng.Intn(len(candidates))]
		return PlaceResult{
			WorkerID: w.ID,
			Reason:   "random free worker",
		}, nil
	}
	if len(req.Running) == 0 || len(req.Workers) == 0 {
		return PlaceResult{}, fmt.Errorf("random: no capacity and nothing to suspend")
	}
	victim := req.Running[p.rng.Intn(len(req.Running))]
	return PlaceResult{
		WorkerID: victim.WorkerID,
		VictimID: victim.ID,
		Reason:   "random eviction to free its worker",
	}, nil
}

func (p *Random) Resume(_ context.Context, req ResumeRequest) (PlaceResult, error) {
	if res, ok := tryStickyResume(req, "random: sticky resume to last idle worker"); ok {
		return res, nil
	}
	return p.Place(context.Background(), placeFromResume(req))
}
