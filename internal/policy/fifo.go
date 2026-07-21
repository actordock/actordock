// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"context"
	"fmt"
	"sort"

	"github.com/actordock/actordock/internal/types"
)

// FIFO places onto the earliest-registered idle Worker (MaxSlots=1 semantics:
// one running sandbox per Worker). When the pool is full, it suspends the
// oldest running sandbox and reuses that sandbox's Worker.
type FIFO struct{}

func NewFIFO() *FIFO { return &FIFO{} }

func (p *FIFO) Name() string { return "fifo" }

func (p *FIFO) Place(_ context.Context, req PlaceRequest) (PlaceResult, error) {
	workers := append([]types.Worker(nil), req.Workers...)
	sort.Slice(workers, func(i, j int) bool {
		if workers[i].RegisteredAt.Equal(workers[j].RegisteredAt) {
			return workers[i].ID < workers[j].ID
		}
		return workers[i].RegisteredAt.Before(workers[j].RegisteredAt)
	})

	for _, w := range workers {
		if w.Healthy && w.FreeSlots() > 0 {
			return PlaceResult{
				WorkerID: w.ID,
				Reason:   "fifo: earliest idle worker",
			}, nil
		}
	}

	if len(req.Running) == 0 {
		return PlaceResult{}, fmt.Errorf("fifo: no capacity and nothing to suspend")
	}

	running := append([]types.Sandbox(nil), req.Running...)
	sort.Slice(running, func(i, j int) bool {
		if running[i].CreatedAt.Equal(running[j].CreatedAt) {
			return running[i].ID < running[j].ID
		}
		return running[i].CreatedAt.Before(running[j].CreatedAt)
	})
	victim := running[0]
	return PlaceResult{
		WorkerID: victim.WorkerID,
		VictimID: victim.ID,
		Reason:   "fifo: suspend oldest to free its worker",
	}, nil
}

func (p *FIFO) Resume(_ context.Context, req ResumeRequest) (PlaceResult, error) {
	if req.Sandbox.WorkerID != "" {
		for _, w := range req.Workers {
			if w.ID == req.Sandbox.WorkerID && w.Healthy && w.FreeSlots() > 0 {
				return PlaceResult{
					WorkerID: w.ID,
					Reason:   "fifo: sticky resume to last idle worker",
				}, nil
			}
		}
	}
	return p.Place(context.Background(), PlaceRequest{
		SandboxID: req.Sandbox.ID,
		Workers:   req.Workers,
		Running:   req.Running,
	})
}
