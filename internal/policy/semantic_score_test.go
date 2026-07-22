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
