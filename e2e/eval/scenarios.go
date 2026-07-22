// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build e2e

package eval

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/actordock/actordock/e2e/internal/harness"
	"github.com/actordock/actordock/internal/signals"
	"github.com/actordock/actordock/internal/types"
)

// Agent-oriented eval scenarios (see e2e/README.md).
const (
	ScenarioS1ColdStart    = "S1_cold_start"
	ScenarioS2HotWake      = "S2_hot_wake"
	ScenarioS3Migrate      = "S3_migrate_sleep"
	ScenarioS4Contention   = "S4_pool_contention"
	ScenarioS5Stateful     = "S5_stateful_agent"
)

type scenarioFn func(t *testing.T, h *harness.Harness, ctx context.Context, workers int)

var evalScenarios = []struct {
	ID  string
	Run scenarioFn
}{
	{ScenarioS1ColdStart, scenarioS1ColdStart},
	{ScenarioS2HotWake, scenarioS2HotWake},
	{ScenarioS3Migrate, scenarioS3MigrateSleep},
	{ScenarioS4Contention, scenarioS4PoolContention},
	{ScenarioS5Stateful, scenarioS5StatefulAgent},
}

// scenarioS1ColdStart: new agent session, first resume from golden (no prior latest).
func scenarioS1ColdStart(t *testing.T, h *harness.Harness, ctx context.Context, _ int) {
	t.Helper()
	n := harness.EnvInt("EVAL_COLD_COUNT", 3)
	for i := 0; i < n; i++ {
		sb := h.CreateSandbox(ctx)
		_ = h.Resume(ctx, sb.ID)
	}
}

// scenarioS2HotWake: pause then resume on same Worker (short agent idle).
func scenarioS2HotWake(t *testing.T, h *harness.Harness, ctx context.Context, _ int) {
	t.Helper()
	sb := h.CreateSandbox(ctx)
	sb = h.Resume(ctx, sb.ID)
	idle := harness.EnvInt("EVAL_IDLE_SEC", 2)
	if idle > 0 {
		time.Sleep(time.Duration(idle) * time.Second)
	}
	_ = h.Pause(ctx, sb.ID)
	_ = h.Resume(ctx, sb.ID)
}

// scenarioS3MigrateSleep: suspend upload then resume on another Worker.
func scenarioS3MigrateSleep(t *testing.T, h *harness.Harness, ctx context.Context, workers int) {
	t.Helper()
	if workers < 2 {
		workers = 2
	}
	h.WaitWorkers(ctx, workers)

	sb := h.CreateSandbox(ctx)
	sb = h.Resume(ctx, sb.ID)
	origin := sb.WorkerID

	_ = h.Suspend(ctx, sb.ID)
	h.OccupyWorker(ctx, origin)
	h.EnsureIdleExcept(ctx, origin)

	resumed := h.Resume(ctx, sb.ID)
	if resumed.WorkerID == origin {
		t.Fatalf("S3: expected migrate off %s, got %s", origin, resumed.WorkerID)
	}
}

// scenarioS4PoolContention: sandboxes already have latest in object store; second resume wave oversubscribes.
func scenarioS4PoolContention(t *testing.T, h *harness.Harness, ctx context.Context, workers int) {
	t.Helper()
	n := harness.EnvInt("EVAL_SANDBOX_COUNT", workers*2)
	if n < workers+1 {
		n = workers + 1
	}

	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		ids = append(ids, h.CreateSandbox(ctx).ID)
	}
	for _, id := range ids {
		_ = h.Resume(ctx, id)
	}
	for _, sb := range h.ListSandboxes(ctx) {
		if sb.State == types.SandboxRunning {
			_ = h.Suspend(ctx, sb.ID)
		}
	}
	for i, id := range ids {
		_ = h.Resume(ctx, id)
		// Simulate agent phases once the pool is full so semantic-score ≠ fifo.
		if h.Policy(ctx) == "semantic-score" && i+1 == workers {
			running := make([]types.Sandbox, 0, workers)
			for _, sb := range h.ListSandboxes(ctx) {
				if sb.State == types.SandboxRunning {
					running = append(running, sb)
				}
			}
			for j, sb := range running {
				if j == 0 {
					h.PostSemantic(ctx, sb.ID, signals.PhaseToolLoop, true)
				} else {
					h.PostSemantic(ctx, sb.ID, signals.PhaseLLMWait, false)
				}
			}
		}
	}
}

// scenarioS5StatefulAgent: agent-like FS + memory footprint, then sticky and cross-worker paths.
func scenarioS5StatefulAgent(t *testing.T, h *harness.Harness, ctx context.Context, workers int) {
	t.Helper()
	if workers < 2 {
		workers = 2
	}
	h.WaitWorkers(ctx, workers)

	sb := h.CreateSandbox(ctx)
	sb = h.Resume(ctx, sb.ID)
	seedAgentState(t, h, ctx, sb.ID)

	_ = h.Pause(ctx, sb.ID)
	_ = h.Resume(ctx, sb.ID)

	origin := sb.WorkerID
	sb = h.GetSandbox(ctx, sb.ID)
	_ = h.Suspend(ctx, sb.ID)
	h.OccupyWorker(ctx, origin)
	h.EnsureIdleExcept(ctx, origin)
	resumed := h.Resume(ctx, sb.ID)
	if resumed.WorkerID == origin {
		t.Fatalf("S5: expected migrate off %s, got %s", origin, resumed.WorkerID)
	}
	if strings.TrimSpace(h.ReadFile(ctx, resumed.ID, agentStatePath)) != agentStateValue {
		t.Fatalf("S5: agent state lost after migrate")
	}
}

const agentStatePath = "/tmp/actordock-agent-state"
const agentStateValue = "agent-eval-marker"

func seedAgentState(t *testing.T, h *harness.Harness, ctx context.Context, sandboxID string) {
	t.Helper()
	h.WriteFile(ctx, sandboxID, agentStatePath, agentStateValue)

	fileKB := harness.EnvInt("EVAL_STATE_FILE_KB", 256)
	if fileKB > 0 {
		// Sparse-ish payload via busybox; errors ignored if shell limits hit.
		_ = h.Exec(ctx, sandboxID, "/bin/busybox", "sh", "-c",
			fmt.Sprintf("dd if=/dev/zero of=/tmp/agent-payload bs=1024 count=%d 2>/dev/null", fileKB))
	}
	memMB := harness.EnvInt("EVAL_STATE_MEM_MB", 8)
	if memMB > 0 {
		_ = h.Exec(ctx, sandboxID, "/bin/busybox", "sh", "-c",
			fmt.Sprintf("mkdir -p /dev/shm && dd if=/dev/zero of=/dev/shm/agent-mem bs=1M count=%d 2>/dev/null", memMB))
	}
}
