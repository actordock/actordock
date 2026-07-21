// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package eval

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type metricSample struct {
	Name   string
	Labels map[string]string
	Value  float64
}

var (
	promLineRE  = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{[^}]*\})?\s+([0-9.eE+-]+|NaN|Inf|-Inf)\s*$`)
	promLabelRE = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)="((?:\\.|[^"\\])*)"`)
)

func parsePromText(body string) []metricSample {
	var out []metricSample
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := promLineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		val, err := strconv.ParseFloat(m[3], 64)
		if err != nil {
			continue
		}
		labels := map[string]string{}
		if m[2] != "" {
			for _, lm := range promLabelRE.FindAllStringSubmatch(m[2], -1) {
				labels[lm[1]] = strings.ReplaceAll(lm[2], `\"`, `"`)
			}
		}
		out = append(out, metricSample{Name: m[1], Labels: labels, Value: val})
	}
	return out
}

func sumCounter(samples []metricSample, name string, wantLabels map[string]string) float64 {
	var sum float64
	for _, s := range samples {
		if !metricNameMatch(s.Name, name) {
			continue
		}
		if !labelsMatch(s.Labels, wantLabels) {
			continue
		}
		sum += s.Value
	}
	return sum
}

func histSumCount(samples []metricSample, name string, wantLabels map[string]string) (sum, count float64) {
	base := sanitizePromName(name)
	sumNames := map[string]struct{}{
		base + "_sum":         {},
		base + "_seconds_sum": {},
		base + "_bytes_sum":   {},
	}
	countNames := map[string]struct{}{
		base + "_count":         {},
		base + "_seconds_count": {},
		base + "_bytes_count":   {},
	}
	for _, s := range samples {
		if !labelsMatch(s.Labels, wantLabels) {
			continue
		}
		if _, ok := sumNames[s.Name]; ok {
			sum += s.Value
		}
		if _, ok := countNames[s.Name]; ok {
			count += s.Value
		}
	}
	return sum, count
}

func histMean(samples []metricSample, name string, wantLabels map[string]string) (mean float64, count float64, ok bool) {
	sum, n := histSumCount(samples, name, wantLabels)
	if n <= 0 {
		return 0, 0, false
	}
	return sum / n, n, true
}

func histDeltaMean(before, after []metricSample, name string, wantLabels map[string]string) (mean, count float64, ok bool) {
	sumB, countB := histSumCount(after, name, wantLabels)
	sumA, countA := histSumCount(before, name, wantLabels)
	dSum := sumB - sumA
	dCount := countB - countA
	if dCount <= 0 {
		return 0, 0, false
	}
	return dSum / dCount, dCount, true
}

func counterDelta(before, after []metricSample, name string, wantLabels map[string]string) float64 {
	return sumCounter(after, name, wantLabels) - sumCounter(before, name, wantLabels)
}

func sanitizePromName(name string) string {
	return strings.ReplaceAll(name, ".", "_")
}

func metricNameMatch(got, want string) bool {
	g := sanitizePromName(got)
	w := sanitizePromName(want)
	if g == w || g == w+"_total" {
		return true
	}
	for _, u := range []string{"_seconds", "_bytes"} {
		if g == w+u || g == w+u+"_total" {
			return true
		}
	}
	return false
}

func labelsMatch(got, want map[string]string) bool {
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

// PolicyReport aggregates eval KPIs for one POLICY run or scenario window.
type PolicyReport struct {
	Policy   string
	Scenario string

	ResumeTotal float64
	StickyLocal float64
	CrossWorker float64
	GoldenCold  float64
	StickyRate  float64

	ResumeLatencyMean float64
	ResumeLatencyN    float64
	StickyLatencyMean float64
	CrossLatencyMean  float64
	ColdLatencyMean   float64

	Evictions       float64
	PreemptCostMean float64
	PreemptCostN    float64
	ResumeWaitMean  float64
	ResumeWaitN     float64
}

func ReportFromMetrics(policy, body string) PolicyReport {
	samples := parsePromText(body)
	pol := map[string]string{"policy": policy}

	r := PolicyReport{Policy: policy}
	r.StickyLocal = sumCounter(samples, "actordock.resume.path", mergeLabels(pol, "path", "sticky_local"))
	r.CrossWorker = sumCounter(samples, "actordock.resume.path", mergeLabels(pol, "path", "cross_worker"))
	r.GoldenCold = sumCounter(samples, "actordock.resume.path", mergeLabels(pol, "path", "golden_cold"))
	r.ResumeTotal = r.StickyLocal + r.CrossWorker + r.GoldenCold
	if loc := r.StickyLocal + r.CrossWorker; loc > 0 {
		r.StickyRate = r.StickyLocal / loc
	}

	if mean, n, ok := histMean(samples, "actordock.sandbox.resume_latency", pol); ok {
		r.ResumeLatencyMean, r.ResumeLatencyN = mean, n
	}
	if mean, _, ok := histMean(samples, "actordock.sandbox.resume_latency", mergeLabels(pol, "path", "sticky_local")); ok {
		r.StickyLatencyMean = mean
	}
	if mean, _, ok := histMean(samples, "actordock.sandbox.resume_latency", mergeLabels(pol, "path", "cross_worker")); ok {
		r.CrossLatencyMean = mean
	}
	if mean, _, ok := histMean(samples, "actordock.sandbox.resume_latency", mergeLabels(pol, "path", "golden_cold")); ok {
		r.ColdLatencyMean = mean
	}

	r.Evictions = sumCounter(samples, "actordock.schedule.eviction", pol)
	if mean, n, ok := histMean(samples, "actordock.sandbox.preempt_cost", pol); ok {
		r.PreemptCostMean, r.PreemptCostN = mean, n
	}
	if mean, n, ok := histMean(samples, "actordock.sandbox.resume_wait", pol); ok {
		r.ResumeWaitMean, r.ResumeWaitN = mean, n
	}
	return r
}

// ReportDelta builds a PolicyReport from Prometheus scrape windows (cumulative counters/histograms).
func ReportDelta(policy, beforeBody, afterBody string) PolicyReport {
	before := parsePromText(beforeBody)
	after := parsePromText(afterBody)
	pol := map[string]string{"policy": policy}

	r := PolicyReport{Policy: policy}
	r.StickyLocal = counterDelta(before, after, "actordock.resume.path", mergeLabels(pol, "path", "sticky_local"))
	r.CrossWorker = counterDelta(before, after, "actordock.resume.path", mergeLabels(pol, "path", "cross_worker"))
	r.GoldenCold = counterDelta(before, after, "actordock.resume.path", mergeLabels(pol, "path", "golden_cold"))
	r.ResumeTotal = r.StickyLocal + r.CrossWorker + r.GoldenCold
	if loc := r.StickyLocal + r.CrossWorker; loc > 0 {
		r.StickyRate = r.StickyLocal / loc
	}

	if mean, n, ok := histDeltaMean(before, after, "actordock.sandbox.resume_latency", pol); ok {
		r.ResumeLatencyMean, r.ResumeLatencyN = mean, n
	}
	if mean, _, ok := histDeltaMean(before, after, "actordock.sandbox.resume_latency", mergeLabels(pol, "path", "sticky_local")); ok {
		r.StickyLatencyMean = mean
	}
	if mean, _, ok := histDeltaMean(before, after, "actordock.sandbox.resume_latency", mergeLabels(pol, "path", "cross_worker")); ok {
		r.CrossLatencyMean = mean
	}
	if mean, _, ok := histDeltaMean(before, after, "actordock.sandbox.resume_latency", mergeLabels(pol, "path", "golden_cold")); ok {
		r.ColdLatencyMean = mean
	}

	r.Evictions = counterDelta(before, after, "actordock.schedule.eviction", pol)
	if mean, n, ok := histDeltaMean(before, after, "actordock.sandbox.preempt_cost", pol); ok {
		r.PreemptCostMean, r.PreemptCostN = mean, n
	}
	if mean, n, ok := histDeltaMean(before, after, "actordock.sandbox.resume_wait", pol); ok {
		r.ResumeWaitMean, r.ResumeWaitN = mean, n
	}
	return r
}

func mergeLabels(base map[string]string, k, v string) map[string]string {
	out := make(map[string]string, len(base)+1)
	for bk, bv := range base {
		out[bk] = bv
	}
	out[k] = v
	return out
}

func FormatReport(r PolicyReport) string {
	sc := r.Scenario
	if sc == "" {
		sc = "-"
	}
	return fmt.Sprintf(
		"scenario=%s policy=%s resumes=%.0f (sticky=%.0f cross=%.0f cold=%.0f sticky_rate=%.2f) "+
			"resume_latency_mean=%.3fs (n=%.0f sticky=%.3fs cross=%.3fs cold=%.3fs) "+
			"evictions=%.0f preempt_cost_mean=%.3fs (n=%.0f) resume_wait_mean=%.3fs (n=%.0f)",
		sc, r.Policy, r.ResumeTotal, r.StickyLocal, r.CrossWorker, r.GoldenCold, r.StickyRate,
		r.ResumeLatencyMean, r.ResumeLatencyN, r.StickyLatencyMean, r.CrossLatencyMean, r.ColdLatencyMean,
		r.Evictions, r.PreemptCostMean, r.PreemptCostN, r.ResumeWaitMean, r.ResumeWaitN,
	)
}

// AggregateReports merges per-scenario deltas into one report per policy (sums + weighted means).
func AggregateReports(in []PolicyReport) PolicyReport {
	if len(in) == 0 {
		return PolicyReport{}
	}
	out := PolicyReport{Policy: in[0].Policy, Scenario: "ALL"}
	var latSum, latN, preemptSum, preemptN, waitSum, waitN float64
	for _, r := range in {
		out.StickyLocal += r.StickyLocal
		out.CrossWorker += r.CrossWorker
		out.GoldenCold += r.GoldenCold
		out.Evictions += r.Evictions
		latSum += r.ResumeLatencyMean * r.ResumeLatencyN
		latN += r.ResumeLatencyN
		preemptSum += r.PreemptCostMean * r.PreemptCostN
		preemptN += r.PreemptCostN
		waitSum += r.ResumeWaitMean * r.ResumeWaitN
		waitN += r.ResumeWaitN
	}
	out.ResumeTotal = out.StickyLocal + out.CrossWorker + out.GoldenCold
	if loc := out.StickyLocal + out.CrossWorker; loc > 0 {
		out.StickyRate = out.StickyLocal / loc
	}
	if latN > 0 {
		out.ResumeLatencyMean, out.ResumeLatencyN = latSum/latN, latN
	}
	if preemptN > 0 {
		out.PreemptCostMean, out.PreemptCostN = preemptSum/preemptN, preemptN
	}
	if waitN > 0 {
		out.ResumeWaitMean, out.ResumeWaitN = waitSum/waitN, waitN
	}
	return out
}

// FormatComparisonTable is a markdown table comparing policies (one row per policy).
func FormatComparisonTable(reports []PolicyReport) string {
	var b strings.Builder
	b.WriteString("| policy | resumes | sticky_rate | resume_latency_s | evictions | preempt_cost_s | resume_wait_s |\n")
	b.WriteString("|--------|--------:|------------:|-----------------:|----------:|---------------:|--------------:|\n")
	for _, r := range reports {
		fmt.Fprintf(&b, "| %s | %.0f | %.3f | %.4f | %.0f | %.4f | %.4f |\n",
			r.Policy, r.ResumeTotal, r.StickyRate, r.ResumeLatencyMean,
			r.Evictions, r.PreemptCostMean, r.ResumeWaitMean)
	}
	return b.String()
}

// FormatScenarioComparisonTable pivots scenario × policy for one metric column set.
func FormatScenarioComparisonTable(scenarioIDs []string, policies []string, byKey map[string]PolicyReport) string {
	var b strings.Builder
	b.WriteString("| scenario | policy | resumes | sticky_rate | resume_latency_s | evictions | preempt_cost_s | resume_wait_s |\n")
	b.WriteString("|----------|--------|--------:|------------:|-----------------:|----------:|---------------:|--------------:|\n")
	for _, sc := range scenarioIDs {
		for _, pol := range policies {
			r, ok := byKey[sc+"|"+pol]
			if !ok {
				continue
			}
			fmt.Fprintf(&b, "| %s | %s | %.0f | %.3f | %.4f | %.0f | %.4f | %.4f |\n",
				sc, pol, r.ResumeTotal, r.StickyRate, r.ResumeLatencyMean,
				r.Evictions, r.PreemptCostMean, r.ResumeWaitMean)
		}
	}
	return b.String()
}

func CompareReports(a, b PolicyReport) string {
	var buf strings.Builder
	title := a.Scenario
	if title == "" {
		title = "-"
	}
	fmt.Fprintf(&buf, "scenario=%s %s vs %s:\n", title, a.Policy, b.Policy)
	cmp := func(name string, x, y float64, lowerBetter bool) {
		winner := "tie"
		switch {
		case x == y:
			winner = "tie"
		case lowerBetter && x < y:
			winner = a.Policy
		case lowerBetter && x > y:
			winner = b.Policy
		case !lowerBetter && x > y:
			winner = a.Policy
		case !lowerBetter && x < y:
			winner = b.Policy
		}
		fmt.Fprintf(&buf, "  %-22s %s=%.4f %s=%.4f winner=%s\n", name, a.Policy, x, b.Policy, y, winner)
	}
	cmp("sticky_rate", a.StickyRate, b.StickyRate, false)
	cmp("resume_latency_mean", a.ResumeLatencyMean, b.ResumeLatencyMean, true)
	cmp("evictions", a.Evictions, b.Evictions, true)
	cmp("preempt_cost_mean", a.PreemptCostMean, b.PreemptCostMean, true)
	cmp("resume_wait_mean", a.ResumeWaitMean, b.ResumeWaitMean, true)
	cmp("cross_worker", a.CrossWorker, b.CrossWorker, true)
	return buf.String()
}
