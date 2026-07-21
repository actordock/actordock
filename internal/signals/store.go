// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package signals

import (
	"sync"
	"time"
)

// Store caches sandbox (runtime+snapshot) and worker signals with a short TTL,
// and maintains GreedyDual-Size keep-alive state (L and per-sandbox H).
type Store struct {
	ttl time.Duration

	mu        sync.RWMutex
	L         float64 // GreedyDual aging clock
	bySandbox map[string]sandboxRecord
	byWorker  map[string]WorkerResource
}

type sandboxRecord struct {
	WorkerID  string
	Runtime   RuntimeResource
	Snapshot  SnapshotResource
	H         float64
	UpdatedAt time.Time
}

// NewStore creates an in-memory signal cache. ttl<=0 defaults to 30s.
func NewStore(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &Store{
		ttl:       ttl,
		bySandbox: make(map[string]sandboxRecord),
		byWorker:  make(map[string]WorkerResource),
	}
}

// GDClock returns the current GreedyDual aging value L.
func (s *Store) GDClock() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.L
}

// ApplyPush ingests one Worker periodic push (worker row + per-sandbox runtime and checkpoint-in-flight).
// Advances H only when LastActiveAt moves forward (cache "hit" / access).
func (s *Store) ApplyPush(push Push, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if push.WorkerID != "" {
		w := push.Worker
		if w.WorkerID == "" {
			w.WorkerID = push.WorkerID
		}
		if w.ReportedAt.IsZero() {
			w.ReportedAt = now
		}
		s.byWorker[w.WorkerID] = w
	}
	for _, sample := range push.Samples {
		if sample.SandboxID == "" {
			continue
		}
		sample.NormalizeLegacy()
		rec, existed := s.bySandbox[sample.SandboxID]
		prevActive := rec.Runtime.LastActiveAt
		rec.WorkerID = sample.WorkerID
		rec.Runtime = sample.Runtime
		rec.Snapshot.CheckpointInProgress = sample.Snapshot.CheckpointInProgress
		rec.UpdatedAt = now
		if !existed || sample.Runtime.LastActiveAt.After(prevActive) {
			rec.H = GDSPriority(s.L, rec.Runtime, rec.Snapshot)
		}
		s.bySandbox[sample.SandboxID] = rec
	}
	s.purgeLocked(now)
}

// RecordCheckpoint writes completed checkpoint metrics and refreshes H (cost/size changed).
func (s *Store) RecordCheckpoint(sandboxID string, snap SnapshotResource, now time.Time) {
	if sandboxID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.bySandbox[sandboxID]
	rec.Snapshot.CheckpointInProgress = false
	if snap.LastCheckpointBytes != 0 {
		rec.Snapshot.LastCheckpointBytes = snap.LastCheckpointBytes
	}
	if snap.LastPreemptCostSec != 0 {
		rec.Snapshot.LastPreemptCostSec = snap.LastPreemptCostSec
	}
	if !snap.LastCheckpointAt.IsZero() {
		rec.Snapshot.LastCheckpointAt = snap.LastCheckpointAt
		rec.Snapshot.LastCheckpointDur = snap.LastCheckpointDur
	}
	rec.UpdatedAt = now
	rec.H = GDSPriority(s.L, rec.Runtime, rec.Snapshot)
	s.bySandbox[sandboxID] = rec
	s.purgeLocked(now)
}

// RecordRestore writes restore latency and refreshes H (cost changed).
func (s *Store) RecordRestore(sandboxID string, at time.Time, dur time.Duration, now time.Time) {
	if sandboxID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.bySandbox[sandboxID]
	rec.Snapshot.LastRestoreAt = at
	rec.Snapshot.LastRestoreDur = dur
	rec.UpdatedAt = now
	rec.H = GDSPriority(s.L, rec.Runtime, rec.Snapshot)
	s.bySandbox[sandboxID] = rec
	s.purgeLocked(now)
}

// OnEvict advances the GreedyDual clock: L := H(victim).
// Call after choosing a victim and before/after suspend; H must still be present.
func (s *Store) OnEvict(sandboxID string) {
	if sandboxID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.bySandbox[sandboxID]
	if !ok {
		return
	}
	s.L = rec.H
}

func (s *Store) purgeLocked(now time.Time) {
	for id, rec := range s.bySandbox {
		if now.Sub(rec.UpdatedAt) > s.ttl {
			delete(s.bySandbox, id)
		}
	}
	for id, w := range s.byWorker {
		if now.Sub(w.ReportedAt) > s.ttl {
			delete(s.byWorker, id)
		}
	}
}

func (s *Store) toSandboxSignals(id string, rec sandboxRecord) SandboxSignals {
	return SandboxSignals{
		SandboxID:  id,
		WorkerID:   rec.WorkerID,
		ReportedAt: rec.UpdatedAt,
		Runtime:    rec.Runtime,
		Snapshot:   rec.Snapshot,
		KeepAliveH: rec.H,
	}
}

// GetSandbox returns fresh sandbox signals.
func (s *Store) GetSandbox(sandboxID string, now time.Time) (SandboxSignals, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.bySandbox[sandboxID]
	if !ok || now.Sub(rec.UpdatedAt) > s.ttl {
		return SandboxSignals{}, false
	}
	return s.toSandboxSignals(sandboxID, rec), true
}

// GetWorker returns fresh worker signals.
func (s *Store) GetWorker(workerID string, now time.Time) (WorkerResource, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.byWorker[workerID]
	if !ok || now.Sub(w.ReportedAt) > s.ttl {
		return WorkerResource{}, false
	}
	return w, true
}

// ListSandboxes returns all fresh sandbox signals.
func (s *Store) ListSandboxes(now time.Time) map[string]SandboxSignals {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]SandboxSignals, len(s.bySandbox))
	for id, rec := range s.bySandbox {
		if now.Sub(rec.UpdatedAt) <= s.ttl {
			out[id] = s.toSandboxSignals(id, rec)
		}
	}
	return out
}

// ListWorkers returns all fresh worker signals.
func (s *Store) ListWorkers(now time.Time) map[string]WorkerResource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]WorkerResource, len(s.byWorker))
	for id, w := range s.byWorker {
		if now.Sub(w.ReportedAt) <= s.ttl {
			out[id] = w
		}
	}
	return out
}
