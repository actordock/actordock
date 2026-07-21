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

func TestLRUIdleEvictsLongestIdle(t *testing.T) {
	p := policy.NewLRUIdle()
	now := time.Now()
	workers := []types.Worker{
		{ID: "w1", MaxSlots: 1, UsedSlots: 1, Healthy: true, RegisteredAt: now},
	}
	running := []types.Sandbox{
		{ID: "busy", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now.Add(-time.Hour)},
		{ID: "idle", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now},
	}
	sandboxSig := map[string]signals.SandboxSignals{
		"busy": {SandboxID: "busy", LastActiveAt: now.Add(-time.Minute)},
		"idle": {SandboxID: "idle", LastActiveAt: now.Add(-time.Hour)},
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
	if res.VictimID != "idle" {
		t.Fatalf("got victim %q, want idle", res.VictimID)
	}
}
