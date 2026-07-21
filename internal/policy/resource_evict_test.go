// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package policy_test

import (
	"context"
	"testing"
	"time"

	"github.com/actordock/actordock/internal/policy"
	"github.com/actordock/actordock/internal/signals"
	"github.com/actordock/actordock/internal/types"
)

func TestResourceEvictGDSPrefersLowCostOverHighCost(t *testing.T) {
	p := policy.NewResourceEvict()
	now := time.Now()

	workers := []types.Worker{
		{ID: "w1", MaxSlots: 1, UsedSlots: 1, Healthy: true, RegisteredAt: now},
	}
	running := []types.Sandbox{
		{ID: "a", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now.Add(-3 * time.Hour)},
		{ID: "b", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now.Add(-2 * time.Hour)},
	}
	// H = Cost/Size (L=0). a has huge preempt cost → high H (keep). b default cost=1 → low H (evict).
	sandboxSig := map[string]signals.SandboxSignals{
		"a": {
			SandboxID:  "a",
			KeepAliveH: signals.GDSPriority(0, signals.RuntimeResource{}, signals.SnapshotResource{LastPreemptCostSec: 8000}),
			Snapshot:   signals.SnapshotResource{LastPreemptCostSec: 8000},
		},
		"b": {
			SandboxID:  "b",
			KeepAliveH: signals.GDSPriority(0, signals.RuntimeResource{}, signals.SnapshotResource{}),
		},
	}

	res, err := p.Place(context.Background(), policy.PlaceRequest{
		SandboxID:      "new",
		Workers:        workers,
		Running:        running,
		SandboxSignals: sandboxSig,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.VictimID != "b" {
		t.Fatalf("got victim %q, want b (lower keep-alive H)", res.VictimID)
	}
}

func TestResourceEvictGDSLargerSizeLowerPriority(t *testing.T) {
	p := policy.NewResourceEvict()
	now := time.Now()
	workers := []types.Worker{
		{ID: "w1", MaxSlots: 1, UsedSlots: 1, Healthy: true, RegisteredAt: now},
	}
	running := []types.Sandbox{
		{ID: "small", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now},
		{ID: "large", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now.Add(time.Second)},
	}
	cost := signals.SnapshotResource{LastPreemptCostSec: 10}
	smallRT := signals.RuntimeResource{MemRSSBytes: 64 * 1024 * 1024}   // 64 MiB
	largeRT := signals.RuntimeResource{MemRSSBytes: 512 * 1024 * 1024} // 512 MiB
	sandboxSig := map[string]signals.SandboxSignals{
		"small": {SandboxID: "small", Runtime: smallRT, Snapshot: cost, KeepAliveH: signals.GDSPriority(0, smallRT, cost)},
		"large": {SandboxID: "large", Runtime: largeRT, Snapshot: cost, KeepAliveH: signals.GDSPriority(0, largeRT, cost)},
	}
	res, err := p.Place(context.Background(), policy.PlaceRequest{
		SandboxID: "new", Workers: workers, Running: running, SandboxSignals: sandboxSig,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.VictimID != "large" {
		t.Fatalf("got victim %q, want large (lower H = Cost/Size)", res.VictimID)
	}
}
