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

func TestPlacePrefersLowerWorkerLoad(t *testing.T) {
	p := policy.NewFIFO()
	now := time.Now()
	workers := []types.Worker{
		{ID: "hot", MaxSlots: 1, UsedSlots: 0, Healthy: true, RegisteredAt: now},
		{ID: "cool", MaxSlots: 1, UsedSlots: 0, Healthy: true, RegisteredAt: now.Add(time.Minute)},
	}
	workerSig := map[string]signals.WorkerResource{
		"hot":  {WorkerID: "hot", CPUUtil: 0.9, MemUtil: 0.8, Healthy: true, ReportedAt: now},
		"cool": {WorkerID: "cool", CPUUtil: 0.1, MemUtil: 0.1, Healthy: true, ReportedAt: now},
	}
	res, err := p.Place(context.Background(), policy.PlaceRequest{
		SandboxID:     "new",
		Workers:       workers,
		WorkerSignals: workerSig,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.WorkerID != "cool" {
		t.Fatalf("got worker %q, want cool (lower load)", res.WorkerID)
	}
}
