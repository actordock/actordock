// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package signals

import (
	"testing"
	"time"
)

func TestStoreTTL(t *testing.T) {
	st := NewStore(10 * time.Second)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	st.ApplyPush(Push{
		WorkerID: "w1",
		Worker:   WorkerResource{WorkerID: "w1", MaxSlots: 1, Healthy: true, ReportedAt: now},
		Samples: []SandboxSignals{{
			SandboxID:  "s1",
			WorkerID:   "w1",
			ReportedAt: now,
			Runtime:    RuntimeResource{LastActiveAt: now},
		}},
	}, now)

	if _, ok := st.GetSandbox("s1", now.Add(5*time.Second)); !ok {
		t.Fatal("expected fresh sample")
	}
	if _, ok := st.GetSandbox("s1", now.Add(11*time.Second)); ok {
		t.Fatal("expected expired sample")
	}
}

func TestStoreCheckpointPreservesRuntime(t *testing.T) {
	st := NewStore(30 * time.Second)
	now := time.Now()
	st.ApplyPush(Push{
		WorkerID: "w1",
		Samples: []SandboxSignals{{
			SandboxID: "s1",
			WorkerID:  "w1",
			Runtime:   RuntimeResource{LastActiveAt: now.Add(-time.Minute), MemRSSBytes: 32 * 1024 * 1024},
		}},
	}, now)
	st.RecordCheckpoint("s1", SnapshotResource{
		LastCheckpointBytes: 1024,
		LastPreemptCostSec:  1.5,
		LastCheckpointAt:    now,
		LastCheckpointDur:   time.Second,
	}, now)
	sig, ok := st.GetSandbox("s1", now)
	if !ok {
		t.Fatal("missing sandbox")
	}
	if sig.Runtime.LastActiveAt.IsZero() {
		t.Fatal("runtime should survive checkpoint write")
	}
	if sig.Snapshot.LastCheckpointBytes != 1024 {
		t.Fatalf("snapshot bytes=%d", sig.Snapshot.LastCheckpointBytes)
	}
	if sig.KeepAliveH <= 0 {
		t.Fatalf("expected positive keep-alive H, got %v", sig.KeepAliveH)
	}
}

func TestApplySemanticAndAttainedWait(t *testing.T) {
	st := NewStore(30 * time.Second)
	now := time.Now().UTC()
	st.ApplySemantic(SemanticPush{
		SandboxID: "s1",
		Semantic:  SemanticResource{Phase: PhaseToolLoop, Lock: true},
	}, now)
	sig, ok := st.GetSandbox("s1", now)
	if !ok {
		t.Fatal("missing after semantic")
	}
	if sig.Semantic.Phase != PhaseToolLoop || !sig.Semantic.Lock {
		t.Fatalf("semantic=%+v", sig.Semantic)
	}

	st.MarkRunning("s1", now)
	st.MarkSuspended("s1", now.Add(10*time.Second))
	sig, ok = st.GetSandbox("s1", now.Add(10*time.Second))
	if !ok {
		t.Fatal("missing after suspend mark")
	}
	if sig.Semantic.AttainedServiceSec < 9.5 {
		t.Fatalf("attained=%v want ~10", sig.Semantic.AttainedServiceSec)
	}
	st.MarkRunning("s1", now.Add(20*time.Second))
	sig, _ = st.GetSandbox("s1", now.Add(20*time.Second))
	if sig.Semantic.WaitSec < 9.5 {
		t.Fatalf("wait=%v want ~10", sig.Semantic.WaitSec)
	}
}

func TestGDSOnEvictAdvancesClock(t *testing.T) {
	st := NewStore(30 * time.Second)
	now := time.Now()
	st.ApplyPush(Push{
		WorkerID: "w1",
		Samples: []SandboxSignals{{
			SandboxID: "vic",
			WorkerID:  "w1",
			Runtime:   RuntimeResource{LastActiveAt: now, MemRSSBytes: bytesPerMiB},
			Snapshot:  SnapshotResource{LastPreemptCostSec: 4},
		}},
	}, now)
	sig, _ := st.GetSandbox("vic", now)
	wantL := sig.KeepAliveH
	st.OnEvict("vic")
	if st.GDClock() != wantL {
		t.Fatalf("L=%v want %v", st.GDClock(), wantL)
	}
	// New access after eviction ages above L.
	later := now.Add(time.Second)
	st.ApplyPush(Push{
		WorkerID: "w1",
		Samples: []SandboxSignals{{
			SandboxID: "other",
			WorkerID:  "w1",
			Runtime:   RuntimeResource{LastActiveAt: later, MemRSSBytes: bytesPerMiB},
		}},
	}, later)
	other, _ := st.GetSandbox("other", later)
	if other.KeepAliveH <= wantL {
		t.Fatalf("new H=%v should be > L=%v", other.KeepAliveH, wantL)
	}
}

func TestGDSCostSize(t *testing.T) {
	if GDSCost(SnapshotResource{}) != 1 {
		t.Fatal("default cost")
	}
	if GDSSize(RuntimeResource{}, SnapshotResource{}) != 1 {
		t.Fatal("default size")
	}
	c := GDSCost(SnapshotResource{LastPreemptCostSec: 2, LastRestoreDur: time.Second})
	if c != 3 {
		t.Fatalf("cost=%v want 3", c)
	}
}
