// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build e2e

package functional

import (
	"context"
	"testing"
	"time"

	"github.com/actordock/actordock/e2e/internal/harness"
	"github.com/actordock/actordock/internal/types"
)

func signalPushWait() time.Duration {
	return time.Duration(harness.EnvInt("SIGNAL_PUSH_WAIT_SEC", 8)) * time.Second
}

// fillAllWorkers resumes sandboxes until every healthy Worker has a running slot.
func fillAllWorkers(t *testing.T, h *harness.Harness, ctx context.Context) []types.Sandbox {
	t.Helper()
	h.CleanupSandboxes(ctx)
	var running []types.Sandbox
	for i := 0; i < 32; i++ {
		workers, err := h.ListWorkers(ctx)
		if err != nil {
			t.Fatal(err)
		}
		free := 0
		for _, w := range workers {
			if w.Healthy && !h.WorkerBusy(ctx, w.ID) {
				free++
			}
		}
		if free == 0 {
			if len(running) == 0 {
				t.Fatal("no running sandboxes after fill")
			}
			return running
		}
		sb := h.CreateSandbox(ctx)
		sb = h.Resume(ctx, sb.ID)
		running = append(running, sb)
	}
	t.Fatal("could not fill all workers")
	return nil
}

func resumeForcesEvict(t *testing.T, h *harness.Harness, ctx context.Context) string {
	t.Helper()
	sb := h.CreateSandbox(ctx)
	_ = h.Resume(ctx, sb.ID)
	return sb.ID
}

func findSuspendedVictim(t *testing.T, h *harness.Harness, ctx context.Context, candidates map[string]struct{}) string {
	t.Helper()
	for _, sb := range h.ListSandboxes(ctx) {
		if sb.State == types.SandboxSuspended && sb.ObjectKey != "" {
			if _, ok := candidates[sb.ID]; ok {
				return sb.ID
			}
		}
	}
	t.Fatal("expected a suspended eviction victim among filled sandboxes")
	return ""
}

func candidateSet(sbs []types.Sandbox) map[string]struct{} {
	out := make(map[string]struct{}, len(sbs))
	for _, sb := range sbs {
		out[sb.ID] = struct{}{}
	}
	return out
}

// TestPolicyFifoEvictsOldestCreated: with pool full, fifo suspends oldest CreatedAt.
func TestPolicyFifoEvictsOldestCreated(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	h.WaitWorkers(ctx, harness.EnvInt("MIN_WORKERS", 2))
	h.WaitGolden(ctx)
	h.SetPolicy(ctx, "fifo")
	h.WaitGolden(ctx)

	filled := fillAllWorkers(t, h, ctx)
	oldest := filled[0].ID
	_ = resumeForcesEvict(t, h, ctx)
	victim := findSuspendedVictim(t, h, ctx, candidateSet(filled))
	if victim != oldest {
		t.Fatalf("fifo victim=%s want oldest %s", victim, oldest)
	}
}

// TestPolicyLRUIdleEvictsLongestIdle: real exec refreshes lastActiveAt via Worker push.
func TestPolicyLRUIdleEvictsLongestIdle(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	h.WaitWorkers(ctx, harness.EnvInt("MIN_WORKERS", 2))
	h.WaitGolden(ctx)
	h.SetPolicy(ctx, "lru-idle")
	h.WaitGolden(ctx)

	filled := fillAllWorkers(t, h, ctx)
	// Refresh the oldest so it is no longer longest-idle; expect another victim.
	_ = h.Exec(ctx, filled[0].ID, "/bin/busybox", "true")
	time.Sleep(signalPushWait())

	_ = resumeForcesEvict(t, h, ctx)
	victim := findSuspendedVictim(t, h, ctx, candidateSet(filled))
	if victim == filled[0].ID {
		t.Fatalf("lru-idle victim=%s should not be freshly-exec'd sandbox", victim)
	}
}

// TestPolicyResourceEvictGDS: Size-dominant GDS — inflate RSS only on one sandbox.
// Do NOT re-Suspend everyone afterward: that couples Cost to Size and flips victims on CI.
func TestPolicyResourceEvictGDS(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	h.WaitWorkers(ctx, harness.EnvInt("MIN_WORKERS", 2))
	h.WaitGolden(ctx)
	h.SetPolicy(ctx, "resource-evict")
	h.WaitGolden(ctx)

	filled := fillAllWorkers(t, h, ctx)
	heavy := filled[0]
	// Equalize LastActiveAt / keep Cost from the shared cold-resume path.
	for _, sb := range filled {
		_ = h.Exec(ctx, sb.ID, "/bin/busybox", "true")
	}
	time.Sleep(signalPushWait())

	// Large anonymous memory → high GDS Size; Cost stays comparable across peers.
	_ = h.Exec(ctx, heavy.ID, "/bin/busybox", "dd", "if=/dev/zero", "of=/dev/shm/heavy", "bs=1M", "count=64")
	for _, sb := range filled[1:] {
		_ = h.Exec(ctx, sb.ID, "/bin/busybox", "true")
	}
	time.Sleep(signalPushWait())

	_ = resumeForcesEvict(t, h, ctx)
	victim := findSuspendedVictim(t, h, ctx, candidateSet(filled))
	// H = L + Cost/Size with similar Cost → larger Size → lower H → heavy evicted.
	if victim != heavy.ID {
		t.Fatalf("resource-evict victim=%s want heavy %s (larger Size → lower keep-alive H)", victim, heavy.ID)
	}
	t.Logf("resource-evict victim=heavy %s", heavy.ID)
}

// TestPolicyRandomEvictsUnderContention: random must Suspend someone when pool is full.
func TestPolicyRandomEvictsUnderContention(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	h.WaitWorkers(ctx, harness.EnvInt("MIN_WORKERS", 2))
	h.WaitGolden(ctx)
	h.SetPolicy(ctx, "random")
	h.WaitGolden(ctx)

	filled := fillAllWorkers(t, h, ctx)
	third := resumeForcesEvict(t, h, ctx)
	victim := findSuspendedVictim(t, h, ctx, candidateSet(filled))
	var thirdRunning bool
	for _, sb := range h.ListSandboxes(ctx) {
		if sb.ID == third && sb.State == types.SandboxRunning {
			thirdRunning = true
		}
	}
	if !thirdRunning {
		t.Fatal("random: expected new sandbox running after eviction")
	}
	t.Logf("random victim=%s new=%s", victim, third)
}
