// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package policy_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/actordock/actordock/internal/policy"
	"github.com/actordock/actordock/internal/signals"
	"github.com/actordock/actordock/internal/types"
)

func TestSemanticScoreSparesToolLoop(t *testing.T) {
	p := policy.NewSemanticScore()
	now := time.Now()
	workers := []types.Worker{
		{ID: "w1", MaxSlots: 1, UsedSlots: 1, Healthy: true, RegisteredAt: now},
	}
	running := []types.Sandbox{
		{ID: "tool", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now.Add(-time.Hour)},
		{ID: "wait", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now},
	}
	sandboxSig := map[string]signals.SandboxSignals{
		"tool": {
			SandboxID: "tool",
			Semantic:  signals.SemanticResource{Phase: signals.PhaseToolLoop, Lock: true},
		},
		"wait": {
			SandboxID: "wait",
			Semantic:  signals.SemanticResource{Phase: signals.PhaseLLMWait, Lock: false},
		},
	}
	res, err := p.Place(context.Background(), policy.PlaceRequest{
		SandboxID: "new", Workers: workers, Running: running, SandboxSignals: sandboxSig,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.VictimID != "wait" {
		t.Fatalf("victim=%q want wait (llm_wait), tool_loop must be spared", res.VictimID)
	}
}

func TestSemanticScoreOverrideWhenAllLocked(t *testing.T) {
	p := policy.NewSemanticScore()
	p.Override = true
	p.PriorMix = 1.0
	now := time.Now()
	workers := []types.Worker{
		{ID: "w1", MaxSlots: 1, UsedSlots: 1, Healthy: true, RegisteredAt: now},
	}
	running := []types.Sandbox{
		{ID: "a", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "b", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now},
	}
	sigLow, sigHigh := -0.1, 0.2
	sandboxSig := map[string]signals.SandboxSignals{
		"a": {SandboxID: "a", Semantic: signals.SemanticResource{
			Phase: signals.PhaseToolLoop, Lock: true, AttainedServiceSec: 100,
			TaskProfile: &signals.TaskProfile{ComplexitySignal: &sigLow, Confidence: 0.9},
		}},
		"b": {SandboxID: "b", Semantic: signals.SemanticResource{
			Phase: signals.PhaseToolLoop, Lock: true, AttainedServiceSec: 1,
			TaskProfile: &signals.TaskProfile{ComplexitySignal: &sigHigh, Confidence: 0.9, EmbeddingSim: 0.9},
		}},
	}
	res, err := p.Place(context.Background(), policy.PlaceRequest{
		SandboxID: "new", Workers: workers, Running: running, SandboxSignals: sandboxSig,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Both locked → override. a has lower complexity signal + high attained → lower keepScore.
	if res.VictimID != "a" {
		t.Fatalf("victim=%q want a under override", res.VictimID)
	}
	if res.Reason == "" || res.Reason == "semantic-score: suspend lowest keepScore (unlocked)" {
		t.Fatalf("reason=%q want semantic-override", res.Reason)
	}
}

func TestSemanticScorePrefersKickLowComplexitySignal(t *testing.T) {
	p := policy.NewSemanticScore()
	p.PriorMix = 1.0
	p.EmbedAlpha = 0.2
	now := time.Now()
	workers := []types.Worker{
		{ID: "w1", MaxSlots: 1, UsedSlots: 1, Healthy: true, RegisteredAt: now},
	}
	running := []types.Sandbox{
		{ID: "easy", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now.Add(-time.Hour)},
		{ID: "hard", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now},
	}
	sigLow, sigHigh := -0.08, 0.15
	sandboxSig := map[string]signals.SandboxSignals{
		"easy": {SandboxID: "easy", Semantic: signals.SemanticResource{
			Phase: signals.PhaseLLMWait,
			TaskProfile: &signals.TaskProfile{
				ComplexitySignal: &sigLow, EmbeddingSim: 0.1, Confidence: 0.9,
			},
		}},
		"hard": {SandboxID: "hard", Semantic: signals.SemanticResource{
			Phase: signals.PhaseLLMWait,
			TaskProfile: &signals.TaskProfile{
				ComplexitySignal: &sigHigh, EmbeddingSim: 0.9, Confidence: 0.9,
			},
		}},
	}
	res, err := p.Place(context.Background(), policy.PlaceRequest{
		SandboxID: "new", Workers: workers, Running: running, SandboxSignals: sandboxSig,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.VictimID != "easy" {
		t.Fatalf("victim=%q want easy (lower complexitySignal prior)", res.VictimID)
	}
}

func TestSemanticScoreRejectsWhenAllLockedWithoutOverride(t *testing.T) {
	p := policy.NewSemanticScore()
	if p.Override {
		t.Fatal("default Override should be false")
	}
	now := time.Now()
	workers := []types.Worker{
		{ID: "w1", MaxSlots: 1, UsedSlots: 1, Healthy: true, RegisteredAt: now},
	}
	running := []types.Sandbox{
		{ID: "a", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now},
	}
	sandboxSig := map[string]signals.SandboxSignals{
		"a": {SandboxID: "a", Semantic: signals.SemanticResource{
			Phase: signals.PhaseToolLoop, Lock: true,
		}},
	}
	_, err := p.Place(context.Background(), policy.PlaceRequest{
		SandboxID: "new", Workers: workers, Running: running, SandboxSignals: sandboxSig,
	})
	if err == nil {
		t.Fatal("expected error when all locked and override disabled")
	}
	if !errors.Is(err, policy.ErrAllSemanticLocked) {
		t.Fatalf("err=%v want ErrAllSemanticLocked", err)
	}
}

func TestSemanticScorePrefersKickLowWaitHighAttained(t *testing.T) {
	p := policy.NewSemanticScore()
	now := time.Now()
	workers := []types.Worker{
		{ID: "w1", MaxSlots: 1, UsedSlots: 1, Healthy: true, RegisteredAt: now},
	}
	running := []types.Sandbox{
		{ID: "hog", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now.Add(-time.Hour)},
		{ID: "fair", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now},
	}
	sandboxSig := map[string]signals.SandboxSignals{
		"hog": {SandboxID: "hog", Semantic: signals.SemanticResource{
			Phase: signals.PhaseIdle, AttainedServiceSec: 1000, WaitSec: 0,
		}},
		"fair": {SandboxID: "fair", Semantic: signals.SemanticResource{
			Phase: signals.PhaseIdle, AttainedServiceSec: 1, WaitSec: 500,
		}},
	}
	res, err := p.Place(context.Background(), policy.PlaceRequest{
		SandboxID: "new", Workers: workers, Running: running, SandboxSignals: sandboxSig,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.VictimID != "hog" {
		t.Fatalf("victim=%q want hog (low fairness)", res.VictimID)
	}
}

func TestSemanticScoreKeepsNearDeadlineOverFar(t *testing.T) {
	p := policy.NewSemanticScore()
	now := time.Now()
	p.Now = func() time.Time { return now }
	near := now.Add(2 * time.Second)
	far := now.Add(2 * time.Hour)
	workers := []types.Worker{
		{ID: "w1", MaxSlots: 1, UsedSlots: 1, Healthy: true, RegisteredAt: now},
	}
	running := []types.Sandbox{
		{ID: "far", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now.Add(-time.Hour)},
		{ID: "near", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now},
	}
	sandboxSig := map[string]signals.SandboxSignals{
		"far": {SandboxID: "far", Semantic: signals.SemanticResource{
			Phase: signals.PhaseLLMWait, Deadline: &far,
		}},
		"near": {SandboxID: "near", Semantic: signals.SemanticResource{
			Phase: signals.PhaseLLMWait, Deadline: &near,
		}},
	}
	res, err := p.Place(context.Background(), policy.PlaceRequest{
		SandboxID: "new", Workers: workers, Running: running, SandboxSignals: sandboxSig,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.VictimID != "far" {
		t.Fatalf("victim=%q want far (lower urgency_online)", res.VictimID)
	}
}

func TestSemanticScoreHugePreemptCostDoesNotBeatToolLoopFilter(t *testing.T) {
	// Unlocked idle with tiny H vs locked tool_loop with huge H: still kick idle.
	p := policy.NewSemanticScore()
	now := time.Now()
	workers := []types.Worker{
		{ID: "w1", MaxSlots: 1, UsedSlots: 1, Healthy: true, RegisteredAt: now},
	}
	running := []types.Sandbox{
		{ID: "tool", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now.Add(-time.Hour)},
		{ID: "idle", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now},
	}
	sandboxSig := map[string]signals.SandboxSignals{
		"tool": {SandboxID: "tool", KeepAliveH: 1e18, Semantic: signals.SemanticResource{
			Phase: signals.PhaseToolLoop, Lock: true,
		}},
		"idle": {SandboxID: "idle", KeepAliveH: 1, Semantic: signals.SemanticResource{
			Phase: signals.PhaseIdle,
		}},
	}
	res, err := p.Place(context.Background(), policy.PlaceRequest{
		SandboxID: "new", Workers: workers, Running: running, SandboxSignals: sandboxSig,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.VictimID != "idle" {
		t.Fatalf("victim=%q want idle (lock filter before cost)", res.VictimID)
	}
}

func TestSemanticScoreResumeDefersToHigherScoreWaiter(t *testing.T) {
	p := policy.NewSemanticScore()
	now := time.Now()
	workers := []types.Worker{
		{ID: "w1", MaxSlots: 1, UsedSlots: 0, Healthy: true, RegisteredAt: now},
	}
	low := -0.4
	high := 0.4
	me := types.Sandbox{ID: "low", State: types.SandboxSuspended, CreatedAt: now}
	other := types.Sandbox{ID: "high", State: types.SandboxSuspended, CreatedAt: now.Add(-time.Minute)}
	sandboxSig := map[string]signals.SandboxSignals{
		"low": {SandboxID: "low", Semantic: signals.SemanticResource{
			Phase: signals.PhaseLLMWait,
			TaskProfile: &signals.TaskProfile{
				ComplexitySignal: &low, Confidence: 0.9,
			},
		}},
		"high": {SandboxID: "high", Semantic: signals.SemanticResource{
			Phase: signals.PhaseLLMWait,
			TaskProfile: &signals.TaskProfile{
				ComplexitySignal: &high, Confidence: 0.9,
			},
		}},
	}
	_, err := p.Resume(context.Background(), policy.ResumeRequest{
		Sandbox:        me,
		Workers:        workers,
		SandboxSignals: sandboxSig,
		Waiting:        []types.Sandbox{me, other},
	})
	if !errors.Is(err, policy.ErrNotBestWaiter) {
		t.Fatalf("err=%v want ErrNotBestWaiter", err)
	}

	res, err := p.Resume(context.Background(), policy.ResumeRequest{
		Sandbox:        other,
		Workers:        workers,
		SandboxSignals: sandboxSig,
		Waiting:        []types.Sandbox{me, other},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.WorkerID != "w1" || res.VictimID != "" {
		t.Fatalf("got %+v want idle place on w1", res)
	}
}

func TestSemanticScoreResumeBestWaiterCanPreemptLLMWait(t *testing.T) {
	p := policy.NewSemanticScore()
	now := time.Now()
	workers := []types.Worker{
		{ID: "w1", MaxSlots: 1, UsedSlots: 1, Healthy: true, RegisteredAt: now},
	}
	running := []types.Sandbox{
		{ID: "hold", WorkerID: "w1", State: types.SandboxRunning, CreatedAt: now.Add(-time.Hour)},
	}
	high := 0.4
	knocker := types.Sandbox{ID: "in", State: types.SandboxSuspended, CreatedAt: now}
	sandboxSig := map[string]signals.SandboxSignals{
		"hold": {SandboxID: "hold", Semantic: signals.SemanticResource{
			Phase: signals.PhaseLLMWait,
		}},
		"in": {SandboxID: "in", Semantic: signals.SemanticResource{
			Phase: signals.PhaseLLMWait,
			TaskProfile: &signals.TaskProfile{
				ComplexitySignal: &high, Confidence: 0.9,
			},
		}},
	}
	res, err := p.Resume(context.Background(), policy.ResumeRequest{
		Sandbox:        knocker,
		Workers:        workers,
		Running:        running,
		SandboxSignals: sandboxSig,
		Waiting:        []types.Sandbox{knocker},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.VictimID != "hold" {
		t.Fatalf("victim=%q want hold (llm_wait preempt)", res.VictimID)
	}
}

func TestSemanticScoreQueueAgingPromotesLongWaiter(t *testing.T) {
	p := policy.NewSemanticScore()
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	p.Now = func() time.Time { return now }

	low := -0.4
	high := 0.4
	longWait := types.Sandbox{ID: "long", State: types.SandboxSuspended, CreatedAt: now}
	shortWait := types.Sandbox{ID: "short", State: types.SandboxSuspended, CreatedAt: now.Add(-time.Minute)}
	sandboxSig := map[string]signals.SandboxSignals{
		"long": {SandboxID: "long", Semantic: signals.SemanticResource{
			Phase: signals.PhaseLLMWait,
			TaskProfile: &signals.TaskProfile{
				ComplexitySignal: &low, Confidence: 0.9,
			},
		}},
		"short": {SandboxID: "short", Semantic: signals.SemanticResource{
			Phase: signals.PhaseLLMWait,
			TaskProfile: &signals.TaskProfile{
				ComplexitySignal: &high, Confidence: 0.9,
			},
		}},
	}
	waiting := []types.Sandbox{longWait, shortWait}

	// Fresh lobby: high static score still wins.
	err := p.RequireBestWaiter(policy.ResumeRequest{
		Sandbox:        longWait,
		SandboxSignals: sandboxSig,
		Waiting:        waiting,
		WaitingSince: map[string]time.Time{
			"long":  now,
			"short": now,
		},
	})
	if !errors.Is(err, policy.ErrNotBestWaiter) {
		t.Fatalf("fresh long waiter err=%v want ErrNotBestWaiter", err)
	}

	// After ~3 minutes in lobby, queue aging should promote the low static score.
	err = p.RequireBestWaiter(policy.ResumeRequest{
		Sandbox:        longWait,
		SandboxSignals: sandboxSig,
		Waiting:        waiting,
		WaitingSince: map[string]time.Time{
			"long":  now.Add(-3 * time.Minute),
			"short": now.Add(-10 * time.Second),
		},
	})
	if err != nil {
		t.Fatalf("aged long waiter err=%v want nil", err)
	}
	err = p.RequireBestWaiter(policy.ResumeRequest{
		Sandbox:        shortWait,
		SandboxSignals: sandboxSig,
		Waiting:        waiting,
		WaitingSince: map[string]time.Time{
			"long":  now.Add(-3 * time.Minute),
			"short": now.Add(-10 * time.Second),
		},
	})
	if !errors.Is(err, policy.ErrNotBestWaiter) {
		t.Fatalf("short waiter err=%v want ErrNotBestWaiter after aging", err)
	}
}
