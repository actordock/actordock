// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/actordock/actordock/internal/signals"
	"github.com/actordock/actordock/internal/types"
)

// ErrAllSemanticLocked means every running candidate is tool_loop/lock and override is off.
// The scheduler may wait and retry Resume until a slot frees.
var ErrAllSemanticLocked = errors.New("semantic-score: all candidates locked and override disabled")

// ErrNotBestWaiter means another Resume waiter has a higher admit keepScore.
// The caller stays in the lobby (does not Place/Evict) until it becomes top-ranked.
var ErrNotBestWaiter = errors.New("semantic-score: not highest-score Resume waiter")

// SemanticScore places onto the least-loaded idle Worker; when full, suspends the
// running sandbox with the lowest agent-semantic keepScore after a phase-lock filter.
// See docs/architecture/semantic-score.md.
type SemanticScore struct {
	WL, WU, WF, WC, WQ float64
	Override           bool
	PriorMix           float64
	EmbedAlpha         float64
	Now                func() time.Time
}

func NewSemanticScore() *SemanticScore {
	return &SemanticScore{
		WL:         envFloat("SEMANTIC_W_L", 3),
		WU:         envFloat("SEMANTIC_W_U", 2),
		WF:         envFloat("SEMANTIC_W_F", 2),
		WC:         envFloat("SEMANTIC_W_C", 1),
		WQ:         envFloat("SEMANTIC_W_Q", 2),
		Override:   envBool("SEMANTIC_OVERRIDE", false),
		PriorMix:   envFloat("SEMANTIC_PRIOR_MIX", 0.3),
		EmbedAlpha: envFloat("SEMANTIC_EMBED_ALPHA", 0.2),
		Now:        time.Now,
	}
}

func (p *SemanticScore) Name() string { return "semantic-score" }

func (p *SemanticScore) Place(_ context.Context, req PlaceRequest) (PlaceResult, error) {
	if w, ok := pickIdleWorker(req.Workers, req.WorkerSignals, req.Running, req.SandboxSignals); ok {
		return PlaceResult{
			WorkerID: w.ID,
			Reason:   "semantic-score: least-loaded idle worker",
		}, nil
	}
	if len(req.Running) == 0 {
		return PlaceResult{}, fmt.Errorf("semantic-score: no capacity and nothing to suspend")
	}
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	victim, reason, err := pickSemanticVictim(req.Running, req.SandboxSignals, p, now)
	if err != nil {
		return PlaceResult{}, err
	}
	return PlaceResult{
		WorkerID: victim.WorkerID,
		VictimID: victim.ID,
		Reason:   reason,
	}, nil
}

func (p *SemanticScore) Resume(ctx context.Context, req ResumeRequest) (PlaceResult, error) {
	// Rank waiters first: only the top score may sticky-place or preempt.
	if err := p.RequireBestWaiter(req); err != nil {
		return PlaceResult{}, err
	}
	if res, ok := tryStickyResume(req, "semantic-score: sticky resume to last idle worker"); ok {
		return res, nil
	}
	return p.Place(ctx, placeFromResume(req))
}

// RequireBestWaiter returns ErrNotBestWaiter unless req.Sandbox has the highest
// admit score among Waiting (keepScore + queue-age boost; single-knocker ranking).
func (p *SemanticScore) RequireBestWaiter(req ResumeRequest) error {
	if len(req.Waiting) == 0 {
		return nil
	}
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	mine := admitScore(req.Sandbox, req.SandboxSignals, p, now, req.WaitingSince)
	for _, other := range req.Waiting {
		if other.ID == "" || other.ID == req.Sandbox.ID {
			continue
		}
		sc := admitScore(other, req.SandboxSignals, p, now, req.WaitingSince)
		if sc > mine {
			return ErrNotBestWaiter
		}
		if sc == mine {
			if other.CreatedAt.Before(req.Sandbox.CreatedAt) {
				return ErrNotBestWaiter
			}
			if other.CreatedAt.Equal(req.Sandbox.CreatedAt) && other.ID < req.Sandbox.ID {
				return ErrNotBestWaiter
			}
		}
	}
	return nil
}

// admitScore is keepScore plus lobby queue aging so long waiters eventually win.
func admitScore(sb types.Sandbox, sandboxSig map[string]signals.SandboxSignals, p *SemanticScore, now time.Time, waitingSince map[string]time.Time) float64 {
	base := keepScore(sb, sandboxSig, p, now)
	queueSec := 0.0
	if waitingSince != nil {
		if t, ok := waitingSince[sb.ID]; ok && !t.IsZero() {
			queueSec = now.Sub(t).Seconds()
			if queueSec < 0 {
				queueSec = 0
			}
		}
	}
	return base + p.WQ*queueAging(queueSec)
}

// queueAging maps lobby wait seconds → unscaled score units (per minute of wait).
func queueAging(sec float64) float64 {
	return sec / 60.0
}

func pickSemanticVictim(running []types.Sandbox, sandboxSig map[string]signals.SandboxSignals, p *SemanticScore, now time.Time) (types.Sandbox, string, error) {
	var candidates []types.Sandbox
	for _, sb := range running {
		if sandboxSig != nil {
			if sig, ok := sandboxSig[sb.ID]; ok && sig.Snapshot.CheckpointInProgress {
				continue
			}
		}
		candidates = append(candidates, sb)
	}
	if len(candidates) == 0 {
		return types.Sandbox{}, "", fmt.Errorf("semantic-score: all running sandboxes busy (checkpoint)")
	}

	var unlocked []types.Sandbox
	for _, sb := range candidates {
		if !semanticLocked(sb, sandboxSig) {
			unlocked = append(unlocked, sb)
		}
	}
	pool := unlocked
	reason := "semantic-score: suspend lowest keepScore (unlocked)"
	if len(pool) == 0 {
		if !p.Override {
			return types.Sandbox{}, "", ErrAllSemanticLocked
		}
		pool = candidates
		reason = "semantic-score: suspend lowest keepScore (semantic-override)"
	}

	bestIdx := 0
	bestScore := keepScore(pool[0], sandboxSig, p, now)
	for i := 1; i < len(pool); i++ {
		sc := keepScore(pool[i], sandboxSig, p, now)
		if sc < bestScore || (sc == bestScore && pool[i].CreatedAt.Before(pool[bestIdx].CreatedAt)) {
			bestIdx = i
			bestScore = sc
		}
	}
	return pool[bestIdx], reason, nil
}

func semanticLocked(sb types.Sandbox, sandboxSig map[string]signals.SandboxSignals) bool {
	if sandboxSig == nil {
		return false
	}
	sig, ok := sandboxSig[sb.ID]
	if !ok {
		return false
	}
	return sig.Semantic.Lock || sig.Semantic.Phase == signals.PhaseToolLoop
}

func keepScore(sb types.Sandbox, sandboxSig map[string]signals.SandboxSignals, p *SemanticScore, now time.Time) float64 {
	var sem signals.SemanticResource
	var h float64
	if sandboxSig != nil {
		if sig, ok := sandboxSig[sb.ID]; ok {
			sem = sig.Semantic
			h = sig.KeepAliveH
			if h == 0 {
				h = signals.GDSPriority(0, sig.Runtime, sig.Snapshot)
			}
		}
	}
	if h == 0 {
		h = 1
	}
	return p.WL*phaseProtect(sem) +
		p.WU*urgency(sem, p.PriorMix, p.EmbedAlpha, now) +
		p.WF*fairness(sem) +
		p.WC*normalizePreempt(h)
}

func phaseProtect(sem signals.SemanticResource) float64 {
	if sem.Lock || sem.Phase == signals.PhaseToolLoop {
		return 1.0
	}
	switch sem.Phase {
	case signals.PhaseLLMWait:
		return 0.2
	case signals.PhaseIdle:
		return 0.0
	case "":
		return 0.5
	default:
		return 0.5
	}
}

func urgency(sem signals.SemanticResource, priorMix, embedAlpha float64, now time.Time) float64 {
	online := urgencyOnline(sem, now)
	prior := urgencyPrior(sem, embedAlpha)
	if prior <= 0 {
		return online
	}
	conf := 0.0
	if sem.TaskProfile != nil {
		conf = sem.TaskProfile.Confidence
	}
	if conf < 0.3 {
		return online
	}
	if online <= 0 {
		return prior
	}
	if priorMix < 0 {
		priorMix = 0
	}
	if priorMix > 1 {
		priorMix = 1
	}
	return (1-priorMix)*online + priorMix*prior
}

func urgencyOnline(sem signals.SemanticResource, now time.Time) float64 {
	if sem.Deadline != nil && !sem.Deadline.IsZero() {
		sec := sem.Deadline.Sub(now).Seconds()
		if sec < 0 {
			sec = 0
		}
		// Maps remaining time → [0,1]: due now ⇒ 1, far ⇒ ~0.
		return 1.0 / (1.0 + sec)
	}
	return 0
}

func urgencyPrior(sem signals.SemanticResource, embedAlpha float64) float64 {
	if sem.TaskProfile == nil {
		return 0
	}
	tp := sem.TaskProfile
	// SR continuous signal → [0,1]: clamp(0.5 + signal).
	complexity := 0.0
	if tp.ComplexitySignal != nil {
		complexity = clamp01(0.5 + *tp.ComplexitySignal)
	}
	sim := clamp01(tp.EmbeddingSim)
	if embedAlpha < 0 {
		embedAlpha = 0
	}
	// Keep prior on the same [0,1] scale as other keepScore terms.
	return clamp01(complexity + embedAlpha*sim)
}

func fairness(sem signals.SemanticResource) float64 {
	att := sem.AttainedServiceSec
	if att < 0 {
		att = 0
	}
	wait := sem.WaitSec
	if wait < 0 {
		wait = 0
	}
	r := wait / (1 + att) // raw ratio, unbounded
	// Soft map [0,∞) → [0,1): 0→0, 1→0.5, ∞→1.
	return 1.0 - 1.0/(1.0+r)
}

// preemptHRef is the KeepAliveH / snapshot-cost scale that maps to ~1.0 after normalize.
const preemptHRef = 1e6

func normalizePreempt(h float64) float64 {
	if h <= 0 {
		return 0
	}
	if math.IsInf(h, 1) {
		return 1
	}
	// log1p(H) / log1p(H_ref) ∈ (0,1] for H ≤ H_ref; clamp above.
	return clamp01(math.Log1p(h) / math.Log1p(preemptHRef))
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func envFloat(k string, def float64) float64 {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func envBool(k string, def bool) bool {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
