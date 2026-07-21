// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

// Package types holds shared domain models (slim Substrate actor/worker model).
package types

import "time"

// SandboxState mirrors Substrate actor lifecycle (API-driven; no atenet).
type SandboxState string

const (
	SandboxSuspended SandboxState = "suspended" // registered; restored from golden/latest
	SandboxRunning   SandboxState = "running"
	SandboxPaused    SandboxState = "paused" // local snapshot only (sticky)
	SandboxDeleting  SandboxState = "deleting"
)

// SnapshotSource says where Resume should load state from.
type SnapshotSource string

const (
	SnapshotNone     SnapshotSource = ""         // use golden
	SnapshotLocal    SnapshotSource = "local"    // pause
	SnapshotExternal SnapshotSource = "external" // suspend / golden URI
)

// Sandbox is one agent session. Create only registers (suspended); Resume schedules.
type Sandbox struct {
	ID                string         `json:"id"`
	State             SandboxState   `json:"state"`
	WorkerID          string         `json:"workerID,omitempty"`
	LocalSnapshotPath string         `json:"localSnapshotPath,omitempty"`
	ObjectKey         string         `json:"objectKey,omitempty"` // object-store prefix
	SnapshotSource    SnapshotSource `json:"snapshotSource,omitempty"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
}

// Worker is a warm Pod hosting at most one running sandbox (Substrate-aligned).
type Worker struct {
	ID           string    `json:"id"`
	Address      string    `json:"address"`
	MaxSlots     int       `json:"maxSlots"` // always 1 for agent semantics
	UsedSlots    int       `json:"usedSlots"`
	Healthy      bool      `json:"healthy"`
	RegisteredAt time.Time `json:"registeredAt"`
}

func (w Worker) FreeSlots() int {
	if w.MaxSlots <= 0 {
		w.MaxSlots = 1
	}
	free := w.MaxSlots - w.UsedSlots
	if free < 0 {
		return 0
	}
	return free
}

// Decision is a structured scheduling choice for logs and eval.
type Decision struct {
	Policy    string `json:"policy"`
	Action    string `json:"action"`
	SandboxID string `json:"sandboxID,omitempty"`
	WorkerID  string `json:"workerID,omitempty"`
	VictimID  string `json:"victimID,omitempty"`
	Reason    string `json:"reason,omitempty"`
}
