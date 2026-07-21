// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package eval

import (
	"strings"
	"testing"
)

func TestParsePromAndReport(t *testing.T) {
	body := `
# HELP actordock_resume_path_total Resume path counts
# TYPE actordock_resume_path_total counter
actordock_resume_path_total{path="sticky_local",policy="fifo"} 3
actordock_resume_path_total{path="cross_worker",policy="fifo"} 1
actordock_resume_path_total{path="golden_cold",policy="fifo"} 4
actordock_sandbox_resume_latency_seconds_sum{path="sticky_local",policy="fifo"} 0.6
actordock_sandbox_resume_latency_seconds_count{path="sticky_local",policy="fifo"} 3
actordock_sandbox_resume_latency_seconds_sum{path="cross_worker",policy="fifo"} 0.5
actordock_sandbox_resume_latency_seconds_count{path="cross_worker",policy="fifo"} 1
actordock_sandbox_resume_latency_seconds_sum{path="golden_cold",policy="fifo"} 0.9
actordock_sandbox_resume_latency_seconds_count{path="golden_cold",policy="fifo"} 4
actordock_schedule_eviction_total{policy="fifo",reason="x"} 2
actordock_sandbox_preempt_cost_seconds_sum{policy="fifo"} 1.0
actordock_sandbox_preempt_cost_seconds_count{policy="fifo"} 2
actordock_sandbox_resume_wait_seconds_sum{policy="fifo"} 0.4
actordock_sandbox_resume_wait_seconds_count{policy="fifo"} 8
`
	r := ReportFromMetrics("fifo", body)
	if r.StickyLocal != 3 || r.CrossWorker != 1 || r.GoldenCold != 4 {
		t.Fatalf("paths: %+v", r)
	}
	if r.StickyRate != 0.75 {
		t.Fatalf("sticky_rate=%v", r.StickyRate)
	}
	if r.ResumeLatencyN != 8 || r.ResumeLatencyMean != 0.25 {
		t.Fatalf("latency mean=%v n=%v", r.ResumeLatencyMean, r.ResumeLatencyN)
	}
	if r.Evictions != 2 {
		t.Fatalf("evictions=%v", r.Evictions)
	}

	other := PolicyReport{
		Policy:            "random",
		StickyRate:        0.5,
		ResumeLatencyMean: 0.5,
		Evictions:         3,
		PreemptCostMean:   0.8,
		ResumeWaitMean:    0.1,
		CrossWorker:       2,
	}
	out := CompareReports(r, other)
	if !strings.Contains(out, "winner=fifo") {
		t.Fatalf("expected fifo wins some rows:\n%s", out)
	}
}

func TestReportDelta(t *testing.T) {
	before := `
actordock_resume_path_total{path="golden_cold",policy="fifo"} 2
actordock_sandbox_resume_latency_seconds_sum{policy="fifo"} 10
actordock_sandbox_resume_latency_seconds_count{policy="fifo"} 2
`
	after := `
actordock_resume_path_total{path="golden_cold",policy="fifo"} 5
actordock_sandbox_resume_latency_seconds_sum{policy="fifo"} 25
actordock_sandbox_resume_latency_seconds_count{policy="fifo"} 5
`
	r := ReportDelta("fifo", before, after)
	if r.GoldenCold != 3 {
		t.Fatalf("golden delta=%v", r.GoldenCold)
	}
	if r.ResumeLatencyN != 3 || r.ResumeLatencyMean != 5 {
		t.Fatalf("latency mean=%v n=%v", r.ResumeLatencyMean, r.ResumeLatencyN)
	}
}
