// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build e2e

package functional

import (
	"context"
	"testing"

	"github.com/actordock/actordock/e2e/internal/harness"
)

// placeOnFreeWorker: with MaxSlots=1, an occupied Worker is not a Place candidate.
// Resume a second sandbox and assert it lands on a different Worker (live /status slots).
func placeOnFreeWorker(t *testing.T, h *harness.Harness, ctx context.Context, policy string) (firstWorker, secondWorker string) {
	t.Helper()
	h.SetPolicy(ctx, policy)
	h.WaitGolden(ctx)
	h.CleanupSandboxes(ctx)

	a := h.CreateSandbox(ctx)
	a = h.Resume(ctx, a.ID)
	b := h.CreateSandbox(ctx)
	b = h.Resume(ctx, b.ID)
	if b.WorkerID == "" {
		t.Fatal("empty workerID on second place")
	}
	if b.WorkerID == a.WorkerID {
		t.Fatalf("%s placed second sandbox on busy worker %s", policy, a.WorkerID)
	}
	return a.WorkerID, b.WorkerID
}

func TestPlaceResourceEvictUsesFreeWorker(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	h.WaitWorkers(ctx, harness.EnvInt("MIN_WORKERS", 2))
	h.WaitGolden(ctx)
	placeOnFreeWorker(t, h, ctx, "resource-evict")
}

func TestPlaceLRUIdleUsesFreeWorker(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	h.WaitWorkers(ctx, harness.EnvInt("MIN_WORKERS", 2))
	h.WaitGolden(ctx)
	placeOnFreeWorker(t, h, ctx, "lru-idle")
}

func TestPlaceFifoUsesFreeWorker(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	h.WaitWorkers(ctx, harness.EnvInt("MIN_WORKERS", 2))
	h.WaitGolden(ctx)
	placeOnFreeWorker(t, h, ctx, "fifo")
}

func TestPlaceSemanticScoreUsesFreeWorker(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	h.WaitWorkers(ctx, harness.EnvInt("MIN_WORKERS", 2))
	h.WaitGolden(ctx)
	placeOnFreeWorker(t, h, ctx, "semantic-score")
}
