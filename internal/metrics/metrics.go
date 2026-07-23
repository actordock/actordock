// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

// Package metrics exposes Actordock allocation / reuse OpenTelemetry instruments.
// MaxSlots=1: density is time-multiplexing, not packing multiple sandboxes per Worker.
package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const meterName = "actordock"

// Resume path labels (MaxSlots=1 reuse).
const (
	PathStickyLocal  = "sticky_local"
	PathCrossWorker  = "cross_worker"
	PathGoldenCold   = "golden_cold"
)

// Eviction / semantic-wait label values for agent-semantic eval.
const (
	VictimPhaseUnknown = "unknown"
	StarvationEnter    = "enter"
	StarvationResolved = "resolved"
	StarvationTimeout  = "timeout"
)

// Metrics records scheduling and reuse signals for policy comparison.
type Metrics struct {
	policy string

	decision               metric.Int64Counter
	eviction               metric.Int64Counter
	semanticStarvationWait metric.Int64Counter
	resumePath             metric.Int64Counter
	decisionLatency        metric.Float64Histogram
	resumeLatency          metric.Float64Histogram
	resumeWait             metric.Float64Histogram
	checkpointLat          metric.Float64Histogram
	restoreLat             metric.Float64Histogram
	preemptCost            metric.Float64Histogram
	slotHold               metric.Float64Histogram
	idleGap                metric.Float64Histogram
	transferLat            metric.Float64Histogram
	transferBytes          metric.Int64Histogram

	mu          sync.Mutex
	runningAt   map[string]time.Time // sandboxID -> became running
	workerIdle  map[string]time.Time // workerID -> became idle
	pool        PoolStats
}

// PoolStats is a callback source for observable gauges.
type PoolStats interface {
	SandboxCounts(ctx context.Context) (running, paused, suspended int64, err error)
	HealthyWorkers(ctx context.Context) (int64, error)
}

// New creates instruments on the global MeterProvider (noop until exporter is installed).
func New(policy string) (*Metrics, error) {
	return NewWithMeter(otel.Meter(meterName), policy)
}

// NewWithMeter creates instruments on an explicit meter (tests).
func NewWithMeter(meter metric.Meter, policy string) (*Metrics, error) {
	m := &Metrics{
		policy:     policy,
		runningAt:  make(map[string]time.Time),
		workerIdle: make(map[string]time.Time),
	}
	var err error
	if m.decision, err = meter.Int64Counter("actordock.schedule.decision",
		metric.WithDescription("Scheduling decisions")); err != nil {
		return nil, err
	}
	if m.eviction, err = meter.Int64Counter("actordock.schedule.eviction",
		metric.WithDescription("Evictions that suspend a victim to free a Worker; labels include victim_phase and victim_lock")); err != nil {
		return nil, err
	}
	if m.semanticStarvationWait, err = meter.Int64Counter("actordock.schedule.semantic_starvation_wait",
		metric.WithDescription("Resume wait when all peers are tool_loop/lock (SEMANTIC_WAIT); outcome=enter|resolved|timeout")); err != nil {
		return nil, err
	}
	if m.resumePath, err = meter.Int64Counter("actordock.resume.path",
		metric.WithDescription("Resume path counts: sticky_local|cross_worker|golden_cold")); err != nil {
		return nil, err
	}
	if m.decisionLatency, err = meter.Float64Histogram("actordock.schedule.decision_latency",
		metric.WithDescription("Policy Place/Resume selection latency"),
		metric.WithUnit("s")); err != nil {
		return nil, err
	}
	if m.resumeLatency, err = meter.Float64Histogram("actordock.sandbox.resume_latency",
		metric.WithDescription("End-to-end Resume latency"),
		metric.WithUnit("s")); err != nil {
		return nil, err
	}
	if m.resumeWait, err = meter.Float64Histogram("actordock.sandbox.resume_wait",
		metric.WithDescription("Time from Resume request until restore starts (decision+eviction)"),
		metric.WithUnit("s")); err != nil {
		return nil, err
	}
	if m.checkpointLat, err = meter.Float64Histogram("actordock.sandbox.checkpoint_latency",
		metric.WithDescription("Worker checkpoint RPC latency"),
		metric.WithUnit("s")); err != nil {
		return nil, err
	}
	if m.restoreLat, err = meter.Float64Histogram("actordock.sandbox.restore_latency",
		metric.WithDescription("Worker restore RPC latency"),
		metric.WithUnit("s")); err != nil {
		return nil, err
	}
	if m.preemptCost, err = meter.Float64Histogram("actordock.sandbox.preempt_cost",
		metric.WithDescription("Eviction suspend cost (checkpoint+upload)"),
		metric.WithUnit("s")); err != nil {
		return nil, err
	}
	if m.slotHold, err = meter.Float64Histogram("actordock.sandbox.slot_hold_time",
		metric.WithDescription("Time a sandbox held a Worker slot while running"),
		metric.WithUnit("s")); err != nil {
		return nil, err
	}
	if m.idleGap, err = meter.Float64Histogram("actordock.worker.idle_gap",
		metric.WithDescription("Gap from Worker becoming idle to next claim"),
		metric.WithUnit("s")); err != nil {
		return nil, err
	}
	if m.transferLat, err = meter.Float64Histogram("actordock.snapshot.transfer_latency",
		metric.WithDescription("Snapshot object-store transfer latency"),
		metric.WithUnit("s")); err != nil {
		return nil, err
	}
	if m.transferBytes, err = meter.Int64Histogram("actordock.snapshot.transfer_bytes",
		metric.WithDescription("Snapshot object-store transfer size"),
		metric.WithUnit("By")); err != nil {
		return nil, err
	}

	_, err = meter.Int64ObservableGauge("actordock.sandbox.state",
		metric.WithDescription("Sandboxes by state"),
		metric.WithInt64Callback(m.observeSandboxState))
	if err != nil {
		return nil, err
	}
	_, err = meter.Int64ObservableGauge("actordock.worker.healthy",
		metric.WithDescription("Healthy Worker count (= available slots under MaxSlots=1)"),
		metric.WithInt64Callback(m.observeHealthyWorkers))
	if err != nil {
		return nil, err
	}
	return m, nil
}

// SetPoolStats wires gauge callbacks (call once after Scheduler is ready).
func (m *Metrics) SetPoolStats(p PoolStats) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.pool = p
	m.mu.Unlock()
}

func (m *Metrics) policyAttr() attribute.KeyValue {
	return attribute.String("policy", m.policy)
}

func (m *Metrics) RecordDecision(ctx context.Context, action, outcome, reason string) {
	if m == nil {
		return
	}
	m.decision.Add(ctx, 1,
		metric.WithAttributes(
			m.policyAttr(),
			attribute.String("action", action),
			attribute.String("outcome", outcome),
			attribute.String("reason", reason),
		))
}

func (m *Metrics) RecordDecisionLatency(ctx context.Context, d time.Duration) {
	if m == nil {
		return
	}
	m.decisionLatency.Record(ctx, d.Seconds(), metric.WithAttributes(m.policyAttr()))
}

// RecordEviction increments eviction with victim L1 snapshot labels.
// victimPhase empty → "unknown"; victimLock serialized as "true"|"false".
// mid_tool_suspend ≈ count where victim_phase=tool_loop OR victim_lock=true.
func (m *Metrics) RecordEviction(ctx context.Context, reason, victimPhase string, victimLock bool) {
	if m == nil {
		return
	}
	if victimPhase == "" {
		victimPhase = VictimPhaseUnknown
	}
	lock := "false"
	if victimLock {
		lock = "true"
	}
	m.eviction.Add(ctx, 1, metric.WithAttributes(
		m.policyAttr(),
		attribute.String("reason", reason),
		attribute.String("victim_phase", victimPhase),
		attribute.String("victim_lock", lock),
	))
}

// RecordSemanticStarvationWait records Resume all-locked wait lifecycle.
// outcome: enter | resolved | timeout.
func (m *Metrics) RecordSemanticStarvationWait(ctx context.Context, outcome string) {
	if m == nil {
		return
	}
	if outcome == "" {
		outcome = StarvationEnter
	}
	m.semanticStarvationWait.Add(ctx, 1, metric.WithAttributes(
		m.policyAttr(),
		attribute.String("outcome", outcome),
	))
}

func (m *Metrics) RecordResumePath(ctx context.Context, path string) {
	if m == nil {
		return
	}
	m.resumePath.Add(ctx, 1, metric.WithAttributes(m.policyAttr(), attribute.String("path", path)))
}

func (m *Metrics) RecordResumeLatency(ctx context.Context, path string, d time.Duration) {
	if m == nil {
		return
	}
	m.resumeLatency.Record(ctx, d.Seconds(),
		metric.WithAttributes(m.policyAttr(), attribute.String("path", path)))
}

func (m *Metrics) RecordResumeWait(ctx context.Context, d time.Duration) {
	if m == nil {
		return
	}
	m.resumeWait.Record(ctx, d.Seconds(), metric.WithAttributes(m.policyAttr()))
}

func (m *Metrics) RecordCheckpointLatency(ctx context.Context, mode string, d time.Duration) {
	if m == nil {
		return
	}
	m.checkpointLat.Record(ctx, d.Seconds(),
		metric.WithAttributes(m.policyAttr(), attribute.String("mode", mode)))
}

func (m *Metrics) RecordRestoreLatency(ctx context.Context, path string, d time.Duration) {
	if m == nil {
		return
	}
	m.restoreLat.Record(ctx, d.Seconds(),
		metric.WithAttributes(m.policyAttr(), attribute.String("path", path)))
}

func (m *Metrics) RecordPreemptCost(ctx context.Context, d time.Duration) {
	if m == nil {
		return
	}
	m.preemptCost.Record(ctx, d.Seconds(), metric.WithAttributes(m.policyAttr()))
}

func (m *Metrics) RecordTransfer(ctx context.Context, direction string, d time.Duration, bytes int64) {
	if m == nil {
		return
	}
	attrs := metric.WithAttributes(attribute.String("direction", direction))
	m.transferLat.Record(ctx, d.Seconds(), attrs)
	if bytes > 0 {
		m.transferBytes.Record(ctx, bytes, attrs)
	}
}

// MarkRunning records slot claim; emits idle_gap if the Worker was idle.
func (m *Metrics) MarkRunning(ctx context.Context, sandboxID, workerID string, at time.Time) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.runningAt[sandboxID] = at
	if idleAt, ok := m.workerIdle[workerID]; ok {
		gap := at.Sub(idleAt)
		delete(m.workerIdle, workerID)
		m.mu.Unlock()
		if gap > 0 {
			m.idleGap.Record(ctx, gap.Seconds(), metric.WithAttributes(m.policyAttr()))
		}
		return
	}
	m.mu.Unlock()
}

// MarkSlotFreed records slot_hold_time and marks the Worker idle.
func (m *Metrics) MarkSlotFreed(ctx context.Context, sandboxID, workerID string, at time.Time) {
	if m == nil {
		return
	}
	m.mu.Lock()
	started, ok := m.runningAt[sandboxID]
	delete(m.runningAt, sandboxID)
	if workerID != "" {
		m.workerIdle[workerID] = at
	}
	m.mu.Unlock()
	if ok {
		hold := at.Sub(started)
		if hold > 0 {
			m.slotHold.Record(ctx, hold.Seconds(), metric.WithAttributes(m.policyAttr()))
		}
	}
}

func (m *Metrics) observeSandboxState(ctx context.Context, obs metric.Int64Observer) error {
	m.mu.Lock()
	p := m.pool
	m.mu.Unlock()
	if p == nil {
		return nil
	}
	running, paused, suspended, err := p.SandboxCounts(ctx)
	if err != nil {
		return err
	}
	base := m.policyAttr()
	obs.Observe(running, metric.WithAttributes(base, attribute.String("state", "running")))
	obs.Observe(paused, metric.WithAttributes(base, attribute.String("state", "paused")))
	obs.Observe(suspended, metric.WithAttributes(base, attribute.String("state", "suspended")))
	return nil
}

func (m *Metrics) observeHealthyWorkers(ctx context.Context, obs metric.Int64Observer) error {
	m.mu.Lock()
	p := m.pool
	m.mu.Unlock()
	if p == nil {
		return nil
	}
	n, err := p.HealthyWorkers(ctx)
	if err != nil {
		return err
	}
	obs.Observe(n, metric.WithAttributes(m.policyAttr()))
	return nil
}

// ClassifyResumePath maps Resume inputs to sticky_local|cross_worker|golden_cold.
func ClassifyResumePath(prevWorkerID, chosenWorkerID string, localOnly bool, objectKey string, usedGolden bool) string {
	if usedGolden {
		return PathGoldenCold
	}
	if localOnly {
		return PathStickyLocal
	}
	if objectKey != "" && prevWorkerID != "" && prevWorkerID == chosenWorkerID {
		return PathStickyLocal
	}
	if objectKey != "" {
		return PathCrossWorker
	}
	return PathStickyLocal
}

// MustNew is New that panics on registration failure.
func MustNew(policy string) *Metrics {
	m, err := New(policy)
	if err != nil {
		panic(fmt.Sprintf("metrics: %v", err))
	}
	return m
}
