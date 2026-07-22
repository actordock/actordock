// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build e2e

package functional

import (
	"context"
	"testing"
	"time"

	"github.com/actordock/actordock/e2e/internal/harness"
	"github.com/actordock/actordock/internal/signals"
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

	// Directly assert resource plugin has non-zero Size signal for heavy before eviction.
	heavySig := h.GetSandboxSignals(ctx, heavy.ID)
	if heavySig.Runtime.MemRSSBytes == 0 && heavySig.Snapshot.LastCheckpointBytes == 0 {
		t.Fatalf("heavy sandbox has no Size signal (rss=0 checkpointBytes=0): %+v", heavySig)
	}
	if heavySig.KeepAliveH == 0 {
		t.Fatalf("heavy keepAliveH=0: %+v", heavySig)
	}

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

// TestPolicySemanticScoreSparesToolLoop: fake agent phases — tool_loop kept, llm_wait evicted.
func TestPolicySemanticScoreSparesToolLoop(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	h.WaitWorkers(ctx, harness.EnvInt("MIN_WORKERS", 2))
	h.WaitGolden(ctx)
	h.SetPolicy(ctx, "semantic-score")
	h.WaitGolden(ctx)

	filled := fillAllWorkers(t, h, ctx)
	if len(filled) < 2 {
		t.Fatalf("need >=2 running sandboxes, got %d", len(filled))
	}
	protected := filled[0].ID
	h.PostSemantic(ctx, protected, signals.PhaseToolLoop, true)
	for _, sb := range filled[1:] {
		h.PostSemantic(ctx, sb.ID, signals.PhaseLLMWait, false)
	}

	_ = resumeForcesEvict(t, h, ctx)
	victim := findSuspendedVictim(t, h, ctx, candidateSet(filled))
	if victim == protected {
		t.Fatalf("semantic-score victim=%s should spare tool_loop %s", victim, protected)
	}
	t.Logf("semantic-score spared tool_loop=%s victim=%s", protected, victim)
}

// TestPolicySemanticScoreRejectsWhenAllLocked: override off + SEMANTIC_WAIT_SEC=0 → Resume fails.
func TestPolicySemanticScoreRejectsWhenAllLocked(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	h.WaitWorkers(ctx, harness.EnvInt("MIN_WORKERS", 2))
	h.WaitGolden(ctx)
	h.SetControlplaneEnv(ctx, "POLICY=semantic-score", "SEMANTIC_WAIT_SEC=0")
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if h.Policy(ctx) == "semantic-score" {
			break
		}
		time.Sleep(time.Second)
	}
	if h.Policy(ctx) != "semantic-score" {
		t.Fatalf("policy=%q want semantic-score", h.Policy(ctx))
	}
	h.CleanupSandboxes(ctx)
	h.WaitGolden(ctx)

	filled := fillAllWorkers(t, h, ctx)
	for _, sb := range filled {
		h.PostSemantic(ctx, sb.ID, signals.PhaseToolLoop, true)
	}

	sb := h.CreateSandbox(ctx)
	code, body := h.TryResume(ctx, sb.ID)
	if code < 400 {
		t.Fatalf("resume with all tool_loop locked: status=%d body=%s want 4xx (wait=0)", code, body)
	}
	for _, f := range filled {
		cur := h.GetSandbox(ctx, f.ID)
		if cur.State != types.SandboxRunning {
			t.Fatalf("locked sandbox %s state=%s want still running", f.ID, cur.State)
		}
	}
	t.Logf("semantic-score rejected resume status=%d body=%s", code, body)
}
