// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package signals

import "time"

// RuntimeResource describes live resource behavior of a running sandbox.
type RuntimeResource struct {
	CPUUtil      float64   `json:"cpuUtil"` // observability only; not used by GDS eviction
	MemRSSBytes  uint64    `json:"memRSSBytes"`
	LastActiveAt time.Time `json:"lastActiveAt"`
}

// SnapshotResource describes checkpoint/snapshot cost and in-flight state.
type SnapshotResource struct {
	CheckpointInProgress bool          `json:"checkpointInProgress"`
	LastCheckpointBytes  uint64        `json:"lastCheckpointBytes"`
	LastPreemptCostSec   float64       `json:"lastPreemptCostSec"`
	LastCheckpointAt     time.Time     `json:"lastCheckpointAt"` // observability
	LastCheckpointDur    time.Duration `json:"lastCheckpointDur"`
	LastRestoreAt        time.Time     `json:"lastRestoreAt"` // observability
	LastRestoreDur       time.Duration `json:"lastRestoreDur"`
}

// WorkerResource describes live Worker/Pod capacity and pressure (one row per workerID).
type WorkerResource struct {
	WorkerID   string    `json:"workerID"`
	MaxSlots   int       `json:"maxSlots"`
	UsedSlots  int       `json:"usedSlots"`
	Healthy    bool      `json:"healthy"`
	CPUUtil    float64   `json:"cpuUtil"`
	MemUtil    float64   `json:"memUtil"`
	MemBytes   uint64    `json:"memBytes"`
	ReportedAt time.Time `json:"reportedAt"`
}

// Semantic phases (agent L1 heartbeat). Align with docs/architecture/semantic-score.md.
const (
	PhaseLLMWait  = "llm_wait"
	PhaseToolLoop = "tool_loop"
	PhaseIdle     = "idle"
)

// TaskProfile is an optional L3 prior from local HF models (domain + embed;
// complexity via SR-style continuous signal). Ignored when Confidence is low.
// See docs/architecture/semantic-score.md §6.
type TaskProfile struct {
	Version string `json:"version,omitempty"`
	// ComplexitySignal is SR hardScore−easyScore (continuous). Preferred for urgencyPrior.
	ComplexitySignal *float64 `json:"complexitySignal,omitempty"`
	// DifficultyTier is optional debug label only; not used by keepScore.
	DifficultyTier string  `json:"difficultyTier,omitempty"`
	Domain          string  `json:"domain,omitempty"`
	EmbeddingSim    float64 `json:"embeddingSim,omitempty"` // 0..1
	Confidence      float64 `json:"confidence,omitempty"`   // 0..1; low ⇒ ignore prior
	ModelID         string  `json:"modelID,omitempty"`
	ScoredAt        time.Time `json:"scoredAt,omitempty"`

	// Legacy fields (ignored by semantic-score urgencyPrior).
	ExpectedSteps      float64 `json:"expectedSteps,omitempty"`
	ExpectedToolSec    float64 `json:"expectedToolSec,omitempty"`
	ExpectedLLMWaitSec float64 `json:"expectedLLMWaitSec,omitempty"`
}

// SemanticResource is agent-session meaning for scheduling (optional keys).
type SemanticResource struct {
	Version        string       `json:"version,omitempty"`
	Phase          string       `json:"phase,omitempty"` // llm_wait|tool_loop|idle
	Lock           bool         `json:"lock,omitempty"`
	RemainingSteps *int         `json:"remainingSteps,omitempty"`
	Deadline       *time.Time   `json:"deadline,omitempty"`
	WorkflowID     string       `json:"workflowID,omitempty"`
	TaskProfile    *TaskProfile `json:"taskProfile,omitempty"`

	// Platform-owned (filled by signals.Store on read).
	AttainedServiceSec float64   `json:"attainedServiceSec,omitempty"`
	WaitSec            float64   `json:"waitSec,omitempty"`
	ReportedAt         time.Time `json:"reportedAt,omitempty"`
}

// SandboxSignals is runtime + snapshot + optional semantic for one sandbox.
type SandboxSignals struct {
	SandboxID  string    `json:"sandboxID"`
	WorkerID   string    `json:"workerID"`
	ReportedAt time.Time `json:"reportedAt"`

	Runtime  RuntimeResource  `json:"runtime"`
	Snapshot SnapshotResource `json:"snapshot"`
	Semantic SemanticResource `json:"semantic,omitempty"`

	// KeepAliveH is FaasCache/GreedyDual-Size priority (evict minimum H).
	KeepAliveH float64 `json:"keepAliveH"`

	// Legacy JSON flat fields; normalized into runtime on ingest.
	CPUUtil      float64   `json:"cpuUtil,omitempty"`
	MemRSSBytes  uint64    `json:"memRSSBytes,omitempty"`
	LastActiveAt time.Time `json:"lastActiveAt,omitempty"`
}

// Push is the Worker → control plane resource plugin body.
type Push struct {
	WorkerID string           `json:"workerID"`
	Worker   WorkerResource   `json:"worker"`
	Samples  []SandboxSignals `json:"samples"`
}

// SemanticPush is agent/orchestrator → control plane semantic heartbeat.
type SemanticPush struct {
	SandboxID string           `json:"sandboxID"`
	Semantic  SemanticResource `json:"semantic"`
}

// NormalizeLegacy copies flat JSON fields into runtime.
func (s *SandboxSignals) NormalizeLegacy() {
	if s.Runtime.CPUUtil == 0 {
		s.Runtime.CPUUtil = s.CPUUtil
	}
	if s.Runtime.MemRSSBytes == 0 {
		s.Runtime.MemRSSBytes = s.MemRSSBytes
	}
	if s.Runtime.LastActiveAt.IsZero() {
		s.Runtime.LastActiveAt = s.LastActiveAt
	}
}
