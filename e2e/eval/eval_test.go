// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build e2e

package eval

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/actordock/actordock/e2e/internal/harness"
	"github.com/actordock/actordock/internal/types"
)

var evalPolicies = []string{"fifo", "random", "lru-idle", "resource-evict"}

// TestEvalAllPolicies runs S1–S5 under all four policies and writes a comparison table.
func TestEvalAllPolicies(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	workers := harness.EnvInt("MIN_WORKERS", 4)
	h.WaitWorkers(ctx, workers)
	h.WaitGolden(ctx)

	byKey := make(map[string]PolicyReport)
	perPolicy := make(map[string][]PolicyReport)

	for _, policy := range evalPolicies {
		h.SetPolicy(ctx, policy)
		h.WaitWorkers(ctx, workers)
		h.WaitGolden(ctx)

		for _, sc := range evalScenarios {
			h.CleanupSandboxes(ctx)
			time.Sleep(300 * time.Millisecond)
			before := h.FetchMetrics(ctx)

			sc.Run(t, h, ctx, workers)

			time.Sleep(500 * time.Millisecond)
			after := h.FetchMetrics(ctx)
			r := ReportDelta(policy, before, after)
			r.Scenario = sc.ID
			byKey[sc.ID+"|"+policy] = r
			perPolicy[policy] = append(perPolicy[policy], r)

			t.Log(FormatReport(r))
			if r.ResumeTotal < 1 {
				t.Fatalf("%s %s: expected resume.path delta, got 0", sc.ID, policy)
			}
		}
	}

	scenarioIDs := make([]string, 0, len(evalScenarios))
	for _, sc := range evalScenarios {
		scenarioIDs = append(scenarioIDs, sc.ID)
	}

	agg := make([]PolicyReport, 0, len(evalPolicies))
	for _, policy := range evalPolicies {
		agg = append(agg, AggregateReports(perPolicy[policy]))
	}

	summary := FormatComparisonTable(agg)
	detail := FormatScenarioComparisonTable(scenarioIDs, evalPolicies, byKey)
	doc := "# Actordock policy eval comparison\n\n## Aggregate (S1–S5)\n\n" + summary +
		"\n## Per scenario\n\n" + detail + "\n"

	t.Log("\n" + doc)
	writeEvalArtifact(t, doc)
}

// TestEvalFifoVsRandom keeps the legacy single combined workload for regression.
func TestEvalFifoVsRandom(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	workers := harness.EnvInt("MIN_WORKERS", 4)
	h.WaitWorkers(ctx, workers)

	fifo := runLegacyCombinedEval(t, h, ctx, "fifo", workers)
	random := runLegacyCombinedEval(t, h, ctx, "random", workers)

	t.Log(FormatReport(fifo))
	t.Log(FormatReport(random))
	t.Log(CompareReports(fifo, random))
}

func writeEvalArtifact(t *testing.T, doc string) {
	t.Helper()
	dir := os.Getenv("EVAL_OUT_DIR")
	if dir == "" {
		dir = "docs/eval/results"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("EVAL_OUT_DIR mkdir: %v", err)
	}
	path := filepath.Join(dir, "policy_compare.md")
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	t.Logf("wrote %s", path)
}

func runLegacyCombinedEval(t *testing.T, h *harness.Harness, ctx context.Context, policy string, workers int) PolicyReport {
	t.Helper()
	h.SetPolicy(ctx, policy)
	h.WaitWorkers(ctx, workers)
	h.WaitGolden(ctx)
	h.CleanupSandboxes(ctx)

	before := h.FetchMetrics(ctx)
	runLegacyWorkload(t, h, ctx, workers)
	time.Sleep(500 * time.Millisecond)
	after := h.FetchMetrics(ctx)
	r := ReportDelta(policy, before, after)
	r.Scenario = "legacy_combined"
	return r
}

func runLegacyWorkload(t *testing.T, h *harness.Harness, ctx context.Context, workers int) {
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
	var stickyID, stickyWorker string
	for _, sb := range h.ListSandboxes(ctx) {
		if sb.State == types.SandboxRunning {
			stickyID, stickyWorker = sb.ID, sb.WorkerID
			break
		}
	}
	if stickyID == "" {
		t.Fatal("legacy: no running sandbox for sticky")
	}
	_ = h.Pause(ctx, stickyID)
	_ = h.Resume(ctx, stickyID)
	var migrateID, origin string
	for _, sb := range h.ListSandboxes(ctx) {
		if sb.State == types.SandboxRunning && sb.ID != stickyID {
			migrateID, origin = sb.ID, sb.WorkerID
			break
		}
	}
	if migrateID == "" {
		migrateID, origin = stickyID, stickyWorker
	}
	_ = h.Suspend(ctx, migrateID)
	h.OccupyWorker(ctx, origin)
	h.EnsureIdleExcept(ctx, origin)
	_ = h.Resume(ctx, migrateID)
}
