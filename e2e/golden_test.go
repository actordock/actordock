// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"

	"github.com/actordock/actordock/internal/types"
)

// TestGoldenEnsureAndColdResume: golden object exists; first resume of a new
// sandbox boots from golden into running. No pause/suspend/scheduling stress.
func TestGoldenEnsureAndColdResume(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.waitWorkers(ctx, 1)
	h.waitGolden(ctx)

	var golden struct {
		ObjectKey string `json:"objectKey"`
	}
	h.doJSON(ctx, http.MethodGet, "/v1/golden", nil, &golden)
	if golden.ObjectKey == "" {
		t.Fatal("GET /v1/golden returned empty objectKey")
	}

	sb := h.createSandbox(ctx)
	if sb.State != types.SandboxSuspended {
		t.Fatalf("create state=%s want suspended", sb.State)
	}
	if sb.ObjectKey != "" {
		t.Fatalf("fresh sandbox should have no latest snapshot, objectKey=%q", sb.ObjectKey)
	}

	resumed := h.resume(ctx, sb.ID)
	if resumed.State != types.SandboxRunning {
		t.Fatalf("cold resume state=%s", resumed.State)
	}
	if resumed.WorkerID == "" {
		t.Fatal("cold resume missing workerID")
	}
}
