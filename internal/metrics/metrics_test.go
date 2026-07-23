// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"context"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestRecordResumePathAndLatency(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	m, err := NewWithMeter(provider.Meter("test"), "fifo")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	m.RecordResumePath(ctx, PathStickyLocal)
	m.RecordResumeLatency(ctx, PathStickyLocal, 150*time.Millisecond)
	m.RecordDecision(ctx, "sticky", "ok", "local snapshot")
	m.RecordEviction(ctx, "pool full", "llm_wait", false)
	m.RecordEviction(ctx, "semantic-score", "tool_loop", true)
	m.RecordSemanticStarvationWait(ctx, StarvationEnter)
	m.RecordSemanticStarvationWait(ctx, StarvationResolved)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatal(err)
	}

	gotPath := false
	gotLatency := false
	gotDecision := false
	gotEviction := false
	gotStarvation := false
	var evictionPoints []metricdata.DataPoint[int64]
	for _, sm := range rm.ScopeMetrics {
		for _, met := range sm.Metrics {
			switch met.Name {
			case "actordock.resume.path":
				sum := met.Data.(metricdata.Sum[int64])
				if len(sum.DataPoints) == 0 || sum.DataPoints[0].Value != 1 {
					t.Fatalf("resume.path = %+v", sum.DataPoints)
				}
				gotPath = true
			case "actordock.sandbox.resume_latency":
				hist := met.Data.(metricdata.Histogram[float64])
				if len(hist.DataPoints) == 0 || hist.DataPoints[0].Count != 1 {
					t.Fatalf("resume_latency = %+v", hist.DataPoints)
				}
				gotLatency = true
			case "actordock.schedule.decision":
				gotDecision = true
			case "actordock.schedule.eviction":
				gotEviction = true
				evictionPoints = met.Data.(metricdata.Sum[int64]).DataPoints
			case "actordock.schedule.semantic_starvation_wait":
				gotStarvation = true
				sum := met.Data.(metricdata.Sum[int64])
				if len(sum.DataPoints) < 2 {
					t.Fatalf("starvation_wait points=%+v", sum.DataPoints)
				}
			}
		}
	}
	if !gotPath || !gotLatency || !gotDecision || !gotEviction || !gotStarvation {
		t.Fatalf("missing metrics path=%v latency=%v decision=%v eviction=%v starvation=%v",
			gotPath, gotLatency, gotDecision, gotEviction, gotStarvation)
	}
	if len(evictionPoints) < 2 {
		t.Fatalf("want ≥2 eviction series (phase/lock labels), got %+v", evictionPoints)
	}
	sawToolLock := false
	for _, dp := range evictionPoints {
		attrs := dp.Attributes.ToSlice()
		phase, lock := "", ""
		for _, a := range attrs {
			switch string(a.Key) {
			case "victim_phase":
				phase = a.Value.AsString()
			case "victim_lock":
				lock = a.Value.AsString()
			}
		}
		if phase == "tool_loop" && lock == "true" {
			sawToolLock = true
		}
	}
	if !sawToolLock {
		t.Fatalf("missing mid-tool eviction labels in %+v", evictionPoints)
	}
}

func TestClassifyResumePath(t *testing.T) {
	cases := []struct {
		prev, chosen string
		localOnly    bool
		objectKey    string
		golden       bool
		want         string
	}{
		{"w1", "w1", true, "", false, PathStickyLocal},
		{"w1", "w2", false, "sandboxes/x", false, PathCrossWorker},
		{"w1", "w1", false, "sandboxes/x", false, PathStickyLocal},
		{"", "w1", false, "templates/default/golden", true, PathGoldenCold},
	}
	for _, tc := range cases {
		got := ClassifyResumePath(tc.prev, tc.chosen, tc.localOnly, tc.objectKey, tc.golden)
		if got != tc.want {
			t.Fatalf("ClassifyResumePath(%v) = %q, want %q", tc, got, tc.want)
		}
	}
}

func TestSlotHoldAndIdleGap(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	m, err := NewWithMeter(provider.Meter("test"), "fifo")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	t0 := time.Now()
	m.MarkRunning(ctx, "sb1", "w1", t0)
	m.MarkSlotFreed(ctx, "sb1", "w1", t0.Add(2*time.Second))
	m.MarkRunning(ctx, "sb2", "w1", t0.Add(3*time.Second))

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatal(err)
	}
	gotHold, gotIdle := false, false
	for _, sm := range rm.ScopeMetrics {
		for _, met := range sm.Metrics {
			switch met.Name {
			case "actordock.sandbox.slot_hold_time":
				gotHold = true
			case "actordock.worker.idle_gap":
				gotIdle = true
			}
		}
	}
	if !gotHold || !gotIdle {
		t.Fatalf("hold=%v idle=%v", gotHold, gotIdle)
	}
}
