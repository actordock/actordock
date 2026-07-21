// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build e2e

package eval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/actordock/actordock/e2e/internal/harness"
)

var evalPolicies = []string{"fifo", "random", "lru-idle", "resource-evict"}

// TestEvalAllPolicies runs S1–S5 and writes a comparison markdown artifact.
//
// EVAL_POLICY=<name> — run only that policy (CI matrix / parallel Kind jobs).
// When unset, runs all four policies sequentially (local full compare; uses SetPolicy).
func TestEvalAllPolicies(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	workers := harness.EnvInt("MIN_WORKERS", 4)
	h.WaitWorkers(ctx, workers)
	h.WaitGolden(ctx)

	policies := policiesToRun(t)
	byKey := make(map[string]PolicyReport)
	perPolicy := make(map[string][]PolicyReport)

	for _, policy := range policies {
		ensurePolicy(t, h, ctx, policy, workers)

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

	agg := make([]PolicyReport, 0, len(policies))
	for _, policy := range policies {
		agg = append(agg, AggregateReports(perPolicy[policy]))
	}

	summary := FormatComparisonTable(agg)
	detail := FormatScenarioComparisonTable(scenarioIDs, policies, byKey)
	title := "all policies"
	if len(policies) == 1 {
		title = policies[0]
	}
	doc := "# Actordock policy eval comparison (" + title + ")\n\n## Aggregate (S1–S5)\n\n" + summary +
		"\n## Per scenario\n\n" + detail + "\n"

	t.Log("\n" + doc)
	writeEvalArtifact(t, policies, doc, perPolicy)
}

func policiesToRun(t *testing.T) []string {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv("EVAL_POLICY"))
	if raw == "" {
		return append([]string(nil), evalPolicies...)
	}
	for _, p := range evalPolicies {
		if p == raw {
			return []string{p}
		}
	}
	t.Fatalf("EVAL_POLICY=%q not in %v", raw, evalPolicies)
	return nil
}

// ensurePolicy uses the live controlplane policy when it already matches (CI Kind
// started with POLICY=...); otherwise restarts via SetPolicy (local multi-policy).
func ensurePolicy(t *testing.T, h *harness.Harness, ctx context.Context, policy string, workers int) {
	t.Helper()
	if h.Policy(ctx) == policy {
		h.CleanupSandboxes(ctx)
		return
	}
	h.SetPolicy(ctx, policy)
	h.WaitWorkers(ctx, workers)
	h.WaitGolden(ctx)
}

func writeEvalArtifact(t *testing.T, policies []string, doc string, perPolicy map[string][]PolicyReport) {
	t.Helper()
	dir := os.Getenv("EVAL_OUT_DIR")
	if dir == "" {
		dir = "docs/eval/results"
	}
	if !filepath.IsAbs(dir) {
		// go test runs with package cwd (e2e/eval); resolve relative to module root.
		root, err := findModuleRoot()
		if err != nil {
			t.Fatalf("module root: %v", err)
		}
		dir = filepath.Join(root, dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("EVAL_OUT_DIR mkdir: %v", err)
	}
	paths := []string{filepath.Join(dir, "policy_compare.md")}
	if len(policies) == 1 {
		paths = append(paths, filepath.Join(dir, "policy_compare_"+policies[0]+".md"))
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("wrote %s", path)
	}
	for _, policy := range policies {
		art := BuildPolicyEvalArtifact(policy, perPolicy[policy])
		path, err := WritePolicyEvalJSON(dir, art)
		if err != nil {
			t.Fatalf("write json: %v", err)
		}
		t.Logf("wrote %s", path)
	}
}

func findModuleRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from %s", wd)
		}
		dir = parent
	}
}
