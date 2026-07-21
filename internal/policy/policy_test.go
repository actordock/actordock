// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package policy_test

import (
	"context"
	"testing"
	"time"

	"github.com/actordock/actordock/internal/policy"
	"github.com/actordock/actordock/internal/types"
)

func TestFIFOPrefersEarliestWorker(t *testing.T) {
	p := policy.NewFIFO()
	now := time.Now()
	workers := []types.Worker{
		{ID: "w2", MaxSlots: 1, UsedSlots: 0, Healthy: true, RegisteredAt: now.Add(time.Minute)},
		{ID: "w1", MaxSlots: 1, UsedSlots: 0, Healthy: true, RegisteredAt: now},
	}
	res, err := p.Place(context.Background(), policy.PlaceRequest{SandboxID: "s1", Workers: workers})
	if err != nil {
		t.Fatal(err)
	}
	if res.WorkerID != "w1" {
		t.Fatalf("got worker %s, want w1", res.WorkerID)
	}
}

func TestFIFOEvictsOldest(t *testing.T) {
	p := policy.NewFIFO()
	now := time.Now()
	workers := []types.Worker{
		{ID: "w1", MaxSlots: 1, UsedSlots: 1, Healthy: true, RegisteredAt: now},
	}
	running := []types.Sandbox{
		{ID: "old", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now.Add(-time.Hour)},
	}
	res, err := p.Place(context.Background(), policy.PlaceRequest{
		SandboxID: "new",
		Workers:   workers,
		Running:   running,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.VictimID != "old" || res.WorkerID != "w1" {
		t.Fatalf("got %+v", res)
	}
}

func TestRandomPlacesOnFreeWorker(t *testing.T) {
	p := policy.NewRandom(nil)
	workers := []types.Worker{
		{ID: "w1", MaxSlots: 1, UsedSlots: 0, Healthy: true},
	}
	res, err := p.Place(context.Background(), policy.PlaceRequest{SandboxID: "s1", Workers: workers})
	if err != nil {
		t.Fatal(err)
	}
	if res.WorkerID != "w1" || res.VictimID != "" {
		t.Fatalf("got %+v", res)
	}
}
