// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build e2e

package functional

import (
	"context"
	"strings"
	"testing"

	"github.com/actordock/actordock/e2e/internal/harness"
)

const markerPath = "/tmp/actordock-marker"
const markerValue = "checkpoint-ok"

// TestFSPreservedAcrossPause: write a file in-sandbox, pause+resume sticky,
// file must still be readable. Scheduling affinity is covered elsewhere.
func TestFSPreservedAcrossPause(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	h.WaitWorkers(ctx, 1)
	h.WaitGolden(ctx)

	sb := h.CreateSandbox(ctx)
	sb = h.Resume(ctx, sb.ID)
	h.WriteFile(ctx, sb.ID, markerPath, markerValue)

	_ = h.Pause(ctx, sb.ID)
	_ = h.Resume(ctx, sb.ID)

	got := strings.TrimSpace(h.ReadFile(ctx, sb.ID, markerPath))
	if got != markerValue {
		t.Fatalf("after pause/resume marker=%q want %q", got, markerValue)
	}
}

// TestFSPreservedAcrossSuspend: write a file, suspend (upload), resume
// (possibly same Worker), file must survive. Cross-Worker placement is
// covered by TestSuspendMigratesOffOrigin.
func TestFSPreservedAcrossSuspend(t *testing.T) {
	ctx := context.Background()
	h := harness.New(t)
	h.WaitWorkers(ctx, 1)
	h.WaitGolden(ctx)

	sb := h.CreateSandbox(ctx)
	sb = h.Resume(ctx, sb.ID)
	h.WriteFile(ctx, sb.ID, markerPath, markerValue)

	_ = h.Suspend(ctx, sb.ID)
	_ = h.Resume(ctx, sb.ID)

	got := strings.TrimSpace(h.ReadFile(ctx, sb.ID, markerPath))
	if got != markerValue {
		t.Fatalf("after suspend/resume marker=%q want %q", got, markerValue)
	}
}
