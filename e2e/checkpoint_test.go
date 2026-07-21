// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"
)

const markerPath = "/tmp/actordock-marker"
const markerValue = "checkpoint-ok"

// TestFSPreservedAcrossPause: write a file in-sandbox, pause+resume sticky,
// file must still be readable. Scheduling affinity is covered elsewhere.
func TestFSPreservedAcrossPause(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.waitWorkers(ctx, 1)
	h.waitGolden(ctx)

	sb := h.createSandbox(ctx)
	sb = h.resume(ctx, sb.ID)
	h.writeFile(ctx, sb.ID, markerPath, markerValue)

	_ = h.pause(ctx, sb.ID)
	_ = h.resume(ctx, sb.ID)

	got := strings.TrimSpace(h.readFile(ctx, sb.ID, markerPath))
	if got != markerValue {
		t.Fatalf("after pause/resume marker=%q want %q", got, markerValue)
	}
}

// TestFSPreservedAcrossSuspend: write a file, suspend (upload), resume
// (possibly same Worker), file must survive. Cross-Worker placement is
// covered by TestSuspendMigratesOffOrigin.
func TestFSPreservedAcrossSuspend(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.waitWorkers(ctx, 1)
	h.waitGolden(ctx)

	sb := h.createSandbox(ctx)
	sb = h.resume(ctx, sb.ID)
	h.writeFile(ctx, sb.ID, markerPath, markerValue)

	_ = h.suspend(ctx, sb.ID)
	_ = h.resume(ctx, sb.ID)

	got := strings.TrimSpace(h.readFile(ctx, sb.ID, markerPath))
	if got != markerValue {
		t.Fatalf("after suspend/resume marker=%q want %q", got, markerValue)
	}
}
