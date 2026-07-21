// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergePolicyEvalArtifacts(t *testing.T) {
	arts := []PolicyEvalArtifact{
		BuildPolicyEvalArtifact("fifo", []PolicyReport{
			{Policy: "fifo", Scenario: "S1_cold_start", GoldenCold: 3, ResumeLatencyMean: 0.2, ResumeLatencyN: 3},
			{Policy: "fifo", Scenario: "S2_hot_wake", StickyLocal: 2, ResumeLatencyMean: 0.1, ResumeLatencyN: 2},
		}),
		BuildPolicyEvalArtifact("random", []PolicyReport{
			{Policy: "random", Scenario: "S1_cold_start", GoldenCold: 3, ResumeLatencyMean: 0.3, ResumeLatencyN: 3},
			{Policy: "random", Scenario: "S2_hot_wake", StickyLocal: 2, ResumeLatencyMean: 0.15, ResumeLatencyN: 2},
		}),
	}
	doc := MergePolicyEvalArtifacts(arts)
	if !strings.Contains(doc, "| fifo |") || !strings.Contains(doc, "| random |") {
		t.Fatalf("missing policies:\n%s", doc)
	}
	if !strings.Contains(doc, "S1_cold_start") || !strings.Contains(doc, "all policies") {
		t.Fatalf("bad merge doc:\n%s", doc)
	}

	dir := t.TempDir()
	for _, art := range arts {
		if _, err := WritePolicyEvalJSON(dir, art); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "policy_compare.md")
	if _, err := MergePolicyEvalDir(dir, out); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "| fifo |") {
		t.Fatalf("wrote bad file:\n%s", raw)
	}
}
