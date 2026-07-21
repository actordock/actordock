// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build e2e

package functional

import (
	"context"
	"testing"
	"time"

	"github.com/actordock/actordock/e2e/internal/harness"
)

// burnCPU runs a short busy loop inside the sandbox so cgroup cpuUtil becomes > 0
// across two Worker push intervals.
func burnCPU(t *testing.T, h *harness.Harness, ctx context.Context, id string) {
	t.Helper()
	_ = h.Exec(ctx, id, "/bin/busybox", "sh", "-c",
		"i=0; while [ $i -lt 200000 ]; do i=$((i+1)); done")
}

// TestResourceSignalsAllMetricsPositive proves the resource plugin publishes
// runtime + snapshot + worker signals with every numeric field > 0.
func TestResourceSignalsAllMetricsPositive(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	h.WaitWorkers(ctx, 1)
	h.WaitGolden(ctx)
	h.CleanupSandboxes(ctx)

	sb := h.CreateSandbox(ctx)
	sb = h.Resume(ctx, sb.ID)

	// Snapshot Cost/Size: real Suspend then Resume (records checkpoint + restore).
	_ = h.Suspend(ctx, sb.ID)
	sb = h.Resume(ctx, sb.ID)

	// Runtime Size + activity + CPU (need two push windows for cpu util delta).
	_ = h.Exec(ctx, sb.ID, "/bin/busybox", "dd", "if=/dev/zero", "of=/dev/shm/sig", "bs=1M", "count=8")
	burnCPU(t, h, ctx, sb.ID)
	time.Sleep(signalPushWait())
	burnCPU(t, h, ctx, sb.ID)
	time.Sleep(signalPushWait())

	h.WaitPositiveResourceSignals(ctx, sb.ID, sb.WorkerID, 45*time.Second)

	sig := h.GetSandboxSignals(ctx, sb.ID)
	t.Logf("sandbox signals: runtime=%+v snapshot=%+v H=%v", sig.Runtime, sig.Snapshot, sig.KeepAliveH)
	workers := h.ListWorkerSignals(ctx)
	for _, w := range workers {
		if w.WorkerID == sb.WorkerID {
			t.Logf("worker signals: %+v", w)
		}
	}
}
