// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PolicyEvalArtifact is the machine-readable per-policy eval output for CI merge.
type PolicyEvalArtifact struct {
	Policy     string         `json:"policy"`
	Aggregate  PolicyReport   `json:"aggregate"`
	Scenarios  []PolicyReport `json:"scenarios"`
}

// BuildPolicyEvalArtifact packs one policy's scenario deltas + aggregate.
func BuildPolicyEvalArtifact(policy string, scenarios []PolicyReport) PolicyEvalArtifact {
	return PolicyEvalArtifact{
		Policy:    policy,
		Aggregate: AggregateReports(scenarios),
		Scenarios: append([]PolicyReport(nil), scenarios...),
	}
}

// WritePolicyEvalJSON writes policy_report_<policy>.json under dir.
func WritePolicyEvalJSON(dir string, art PolicyEvalArtifact) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "policy_report_"+art.Policy+".json")
	b, err := json.MarshalIndent(art, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// LoadPolicyEvalJSON reads one policy_report_*.json file.
func LoadPolicyEvalJSON(path string) (PolicyEvalArtifact, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return PolicyEvalArtifact{}, err
	}
	var art PolicyEvalArtifact
	if err := json.Unmarshal(raw, &art); err != nil {
		return PolicyEvalArtifact{}, err
	}
	return art, nil
}

// MergePolicyEvalArtifacts builds the cross-policy comparison markdown.
func MergePolicyEvalArtifacts(arts []PolicyEvalArtifact) string {
	if len(arts) == 0 {
		return "# Actordock policy eval comparison\n\n_no policy reports_\n"
	}
	sort.Slice(arts, func(i, j int) bool { return arts[i].Policy < arts[j].Policy })

	policies := make([]string, 0, len(arts))
	agg := make([]PolicyReport, 0, len(arts))
	byKey := make(map[string]PolicyReport)
	scenarioSet := map[string]struct{}{}
	for _, art := range arts {
		policies = append(policies, art.Policy)
		agg = append(agg, art.Aggregate)
		for _, sc := range art.Scenarios {
			id := sc.Scenario
			if id == "" {
				id = "-"
			}
			scenarioSet[id] = struct{}{}
			byKey[id+"|"+art.Policy] = sc
		}
	}
	scenarioIDs := make([]string, 0, len(scenarioSet))
	for id := range scenarioSet {
		scenarioIDs = append(scenarioIDs, id)
	}
	sort.Strings(scenarioIDs)

	var b strings.Builder
	b.WriteString("# Actordock policy eval comparison (all policies)\n\n")
	b.WriteString("## Aggregate (S1–S5)\n\n")
	b.WriteString(FormatComparisonTable(agg))
	b.WriteString("\n## Per scenario\n\n")
	b.WriteString(FormatScenarioComparisonTable(scenarioIDs, policies, byKey))
	b.WriteString("\n")
	return b.String()
}

// MergePolicyEvalDir loads every policy_report_*.json under dir (recursive) and writes policy_compare.md.
func MergePolicyEvalDir(dir, outPath string) (string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if strings.HasPrefix(base, "policy_report_") && strings.HasSuffix(base, ".json") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("no policy_report_*.json under %s", dir)
	}
	sort.Strings(paths)
	arts := make([]PolicyEvalArtifact, 0, len(paths))
	for _, p := range paths {
		art, err := LoadPolicyEvalJSON(p)
		if err != nil {
			return "", fmt.Errorf("%s: %w", p, err)
		}
		arts = append(arts, art)
	}
	doc := MergePolicyEvalArtifacts(arts)
	if outPath == "" {
		outPath = filepath.Join(dir, "policy_compare.md")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(outPath, []byte(doc), 0o644); err != nil {
		return "", err
	}
	return outPath, nil
}
