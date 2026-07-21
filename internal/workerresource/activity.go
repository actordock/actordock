// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

// Package workerresource samples sandbox resource usage on the Worker and pushes to the control plane.
package workerresource

import (
	"sync"
	"time"
)

// Activity tracks last sandbox activity on this Worker (exec / restore / boot).
type Activity struct {
	mu            sync.Mutex
	lastActive    map[string]time.Time
	lastCPUUtil   map[string]float64
	lastMemRSS    map[string]uint64
	checkpointing map[string]struct{}
}

func NewActivity() *Activity {
	return &Activity{
		lastActive:    make(map[string]time.Time),
		lastCPUUtil:   make(map[string]float64),
		lastMemRSS:    make(map[string]uint64),
		checkpointing: make(map[string]struct{}),
	}
}

func (a *Activity) MarkActive(id string, at time.Time) {
	a.mu.Lock()
	a.lastActive[id] = at
	a.mu.Unlock()
}

func (a *Activity) SetMetrics(id string, cpu float64, memRSS uint64) {
	a.mu.Lock()
	a.lastCPUUtil[id] = cpu
	a.lastMemRSS[id] = memRSS
	a.mu.Unlock()
}

func (a *Activity) Remove(id string) {
	a.mu.Lock()
	delete(a.lastActive, id)
	delete(a.lastCPUUtil, id)
	delete(a.lastMemRSS, id)
	delete(a.checkpointing, id)
	a.mu.Unlock()
}

func (a *Activity) SetCheckpointing(id string, on bool) {
	a.mu.Lock()
	if on {
		a.checkpointing[id] = struct{}{}
	} else {
		delete(a.checkpointing, id)
	}
	a.mu.Unlock()
}

func (a *Activity) BuildSamples(ids []string, workerID string, now time.Time) []Sample {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]Sample, 0, len(ids))
	for _, id := range ids {
		active := a.lastActive[id]
		if active.IsZero() {
			active = now
		}
		_, cp := a.checkpointing[id]
		out = append(out, Sample{
			SandboxID:              id,
			WorkerID:               workerID,
			CPUUtil:                a.lastCPUUtil[id],
			MemRSSBytes:            a.lastMemRSS[id],
			LastActiveAt:           active,
			CheckpointInProgress:   cp,
		})
	}
	return out
}

// Sample is one sandbox row before HTTP push.
type Sample struct {
	SandboxID            string
	WorkerID             string
	CPUUtil              float64
	MemRSSBytes          uint64
	LastActiveAt         time.Time
	CheckpointInProgress bool
}
