// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build e2e

package functional

import (
	"context"
	"testing"

	"github.com/actordock/actordock/e2e/internal/harness"
	"github.com/actordock/actordock/internal/types"
)

// TestScheduleOversubscribeEvicts: N>Workers resume must keep running<=Workers
// and leave at least one suspended victim with an uploaded objectKey.
// Does not cover pause sticky, migration, or FS correctness.
func TestScheduleOversubscribeEvicts(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	h.WaitWorkers(ctx, harness.EnvInt("MIN_WORKERS", 4))
	h.WaitGolden(ctx)

	n := harness.EnvInt("SANDBOX_COUNT", 5)
	maxRunning := harness.EnvInt("MAX_RUNNING", 4)

	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		sb := h.CreateSandbox(ctx)
		if sb.State != types.SandboxSuspended {
			t.Fatalf("create state=%s want suspended", sb.State)
		}
		ids = append(ids, sb.ID)
	}
	for _, id := range ids {
		_ = h.Resume(ctx, id)
	}

	running, suspended := 0, 0
	for _, sb := range h.ListSandboxes(ctx) {
		switch sb.State {
		case types.SandboxRunning:
			running++
		case types.SandboxSuspended:
			suspended++
			if sb.ObjectKey == "" {
				t.Fatalf("evicted %s missing objectKey", sb.ID)
			}
		}
	}
	if running > maxRunning {
		t.Fatalf("running=%d > max=%d", running, maxRunning)
	}
	if n > maxRunning && suspended < 1 {
		t.Fatal("expected at least one suspended victim")
	}
}

// TestPauseStickyToSameWorker: pause keeps a local snapshot and resume must
// return to the same Worker. Does not assert FS contents or cross-Worker.
func TestPauseStickyToSameWorker(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	h.WaitWorkers(ctx, 1)
	h.WaitGolden(ctx)

	sb := h.CreateSandbox(ctx)
	sb = h.Resume(ctx, sb.ID)
	origin := sb.WorkerID

	paused := h.Pause(ctx, sb.ID)
	if paused.State != types.SandboxPaused {
		t.Fatalf("pause state=%s", paused.State)
	}
	if paused.SnapshotSource != types.SnapshotLocal {
		t.Fatalf("pause snapshotSource=%q want local", paused.SnapshotSource)
	}
	if paused.ObjectKey != "" {
		t.Fatalf("pause must not upload, objectKey=%q", paused.ObjectKey)
	}

	resumed := h.Resume(ctx, sb.ID)
	if resumed.State != types.SandboxRunning {
		t.Fatalf("resume state=%s", resumed.State)
	}
	if resumed.WorkerID != origin {
		t.Fatalf("sticky: want %s, got %s", origin, resumed.WorkerID)
	}
}

// TestSuspendMigratesOffOrigin: after suspend, occupy the origin Worker and
// leave capacity elsewhere; resume must land on a different Worker.
// Does not assert FS contents.
func TestSuspendMigratesOffOrigin(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	h.WaitWorkers(ctx, harness.EnvInt("MIN_WORKERS", 4))
	h.WaitGolden(ctx)

	sb := h.CreateSandbox(ctx)
	sb = h.Resume(ctx, sb.ID)
	origin := sb.WorkerID

	suspended := h.Suspend(ctx, sb.ID)
	if suspended.State != types.SandboxSuspended {
		t.Fatalf("suspend state=%s", suspended.State)
	}
	if suspended.ObjectKey == "" {
		t.Fatal("suspend missing objectKey")
	}
	if suspended.SnapshotSource != types.SnapshotExternal {
		t.Fatalf("suspend snapshotSource=%q want external", suspended.SnapshotSource)
	}

	h.OccupyWorker(ctx, origin)
	h.EnsureIdleExcept(ctx, origin)

	resumed := h.Resume(ctx, sb.ID)
	if resumed.State != types.SandboxRunning {
		t.Fatalf("resume state=%s", resumed.State)
	}
	if resumed.WorkerID == origin {
		t.Fatalf("expected migrate off %s, still on origin", origin)
	}
	t.Logf("migrated %s -> %s", origin, resumed.WorkerID)
}
