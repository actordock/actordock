// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/actordock/actordock/internal/metrics"
	"github.com/actordock/actordock/internal/policy"
	"github.com/actordock/actordock/internal/signals"
	"github.com/actordock/actordock/internal/snapshotstore"
	"github.com/actordock/actordock/internal/store"
	"github.com/actordock/actordock/internal/types"
	"github.com/actordock/actordock/internal/workerclient"
	"github.com/google/uuid"
)

const goldenPrefix = "templates/default/golden"

// Scheduler multiplexes sandboxes onto Workers (1:1 while running), Substrate-style:
// Create registers suspended; Resume restores from Latest or Golden; Pause/Suspend free the Worker.
type Scheduler struct {
	store    store.Store
	policy   policy.Policy
	workers  *workerclient.Client
	snapRoot string
	log      *slog.Logger
	metrics  *metrics.Metrics
	signals  *signals.Store

	goldenMu sync.Mutex
}

func New(st store.Store, pol policy.Policy, snapRoot string, log *slog.Logger, m *metrics.Metrics, sig *signals.Store) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	if snapRoot == "" {
		snapRoot = "/var/lib/actordock/snapshots"
	}
	s := &Scheduler{
		store:    st,
		policy:   pol,
		workers:  workerclient.New(),
		snapRoot: snapRoot,
		log:      log,
		metrics:  m,
		signals:  sig,
	}
	if m != nil {
		m.SetPoolStats(s)
	}
	return s
}

func (s *Scheduler) PolicyName() string { return s.policy.Name() }

func (s *Scheduler) signalViews(now time.Time) (map[string]signals.SandboxSignals, map[string]signals.WorkerResource) {
	if s.signals == nil {
		return nil, nil
	}
	return s.signals.ListSandboxes(now), s.signals.ListWorkers(now)
}

func (s *Scheduler) localPath(workerID, sandboxID string) string {
	return filepath.Join(s.snapRoot, workerID, sandboxID)
}

func (s *Scheduler) logDecision(d types.Decision) {
	s.log.Info("schedule decision",
		"policy", d.Policy,
		"action", d.Action,
		"sandboxID", d.SandboxID,
		"workerID", d.WorkerID,
		"victimID", d.VictimID,
		"reason", d.Reason,
	)
}

func (s *Scheduler) SandboxCounts(ctx context.Context) (running, paused, suspended int64, err error) {
	all, err := s.store.ListSandboxes(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	for _, sb := range all {
		switch sb.State {
		case types.SandboxRunning:
			running++
		case types.SandboxPaused:
			paused++
		case types.SandboxSuspended:
			suspended++
		}
	}
	return running, paused, suspended, nil
}

func (s *Scheduler) HealthyWorkers(ctx context.Context) (int64, error) {
	all, err := s.store.ListWorkers(ctx)
	if err != nil {
		return 0, err
	}
	var n int64
	for _, w := range all {
		if w.Healthy {
			n++
		}
	}
	return n, nil
}

func (s *Scheduler) healthyWorkers(ctx context.Context) ([]types.Worker, error) {
	all, err := s.store.ListWorkers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]types.Worker, 0, len(all))
	for _, w := range all {
		st, err := s.workers.Status(ctx, w.Address)
		if err != nil {
			w.Healthy = false
			w.UsedSlots = 0
			_ = s.store.PutWorker(ctx, w)
			continue
		}
		w.Healthy = st.Healthy
		w.UsedSlots = st.UsedSlots
		w.MaxSlots = 1
		_ = s.store.PutWorker(ctx, w)
		if w.Healthy {
			out = append(out, w)
		}
	}
	return out, nil
}

func (s *Scheduler) running(ctx context.Context) ([]types.Sandbox, error) {
	all, err := s.store.ListSandboxes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]types.Sandbox, 0, len(all))
	for _, sb := range all {
		if sb.State == types.SandboxRunning {
			out = append(out, sb)
		}
	}
	return out, nil
}

// Create registers a suspended sandbox (Substrate CreateActor). No Worker yet.
func (s *Scheduler) Create(ctx context.Context) (types.Sandbox, error) {
	now := time.Now().UTC()
	sb := types.Sandbox{
		ID:        uuid.NewString(),
		State:     types.SandboxSuspended,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.store.PutSandbox(ctx, sb); err != nil {
		return types.Sandbox{}, err
	}
	s.logDecision(types.Decision{Policy: s.policy.Name(), Action: "create", SandboxID: sb.ID, Reason: "registered suspended"})
	s.metrics.RecordDecision(ctx, "create", "ok", "registered suspended")
	return sb, nil
}

// EnsureGolden builds the shared golden snapshot once: Boot → Suspend upload.
func (s *Scheduler) EnsureGolden(ctx context.Context) (string, error) {
	if p, err := s.store.GetGolden(ctx); err == nil && p != "" {
		return p, nil
	}

	s.goldenMu.Lock()
	defer s.goldenMu.Unlock()
	if p, err := s.store.GetGolden(ctx); err == nil && p != "" {
		return p, nil
	}

	workers, err := s.healthyWorkers(ctx)
	if err != nil {
		return "", err
	}
	running, err := s.running(ctx)
	if err != nil {
		return "", err
	}
	decStart := time.Now()
	now := decStart
	sandboxSig, workerSig := s.signalViews(now)
	res, err := s.policy.Place(ctx, policy.PlaceRequest{
		SandboxID: "golden", Workers: workers, Running: running,
		SandboxSignals: sandboxSig, WorkerSignals: workerSig,
	})
	s.metrics.RecordDecisionLatency(ctx, time.Since(decStart))
	if err != nil {
		s.metrics.RecordDecision(ctx, "place", "error", err.Error())
		return "", fmt.Errorf("ensure golden place: %w", err)
	}
	action := "place"
	if res.VictimID != "" {
		action = "evict"
		evictStart := time.Now()
		if s.signals != nil {
			s.signals.OnEvict(res.VictimID)
		}
		if _, err := s.suspendLocked(ctx, res.VictimID); err != nil {
			s.metrics.RecordDecision(ctx, action, "error", err.Error())
			return "", fmt.Errorf("ensure golden evict: %w", err)
		}
		s.metrics.RecordEviction(ctx, res.Reason)
		s.metrics.RecordPreemptCost(ctx, time.Since(evictStart))
	}
	s.metrics.RecordDecision(ctx, action, "ok", res.Reason)
	w, err := s.store.GetWorker(ctx, res.WorkerID)
	if err != nil {
		return "", err
	}

	id := "golden-" + uuid.NewString()
	s.metrics.MarkRunning(ctx, id, w.ID, time.Now().UTC())
	if err := s.workers.Boot(ctx, w.Address, id); err != nil {
		s.metrics.MarkSlotFreed(ctx, id, w.ID, time.Now().UTC())
		return "", fmt.Errorf("golden boot: %w", err)
	}
	path := s.localPath(w.ID, id)
	cpStart := time.Now()
	if err := s.workers.Checkpoint(ctx, w.Address, id, workerclient.CheckpointOpts{
		ImagePath: path,
		ObjectKey: goldenPrefix,
	}); err != nil {
		_ = s.workers.Delete(ctx, w.Address, id)
		s.metrics.MarkSlotFreed(ctx, id, w.ID, time.Now().UTC())
		return "", fmt.Errorf("golden suspend: %w", err)
	}
	s.metrics.RecordCheckpointLatency(ctx, "suspend", time.Since(cpStart))
	s.metrics.MarkSlotFreed(ctx, id, w.ID, time.Now().UTC())
	if err := s.store.PutGolden(ctx, goldenPrefix); err != nil {
		return "", err
	}
	s.log.Info("golden snapshot ready", "prefix", goldenPrefix, "workerID", w.ID)
	return goldenPrefix, nil
}

// Resume restores from Latest (external/local) or Golden onto an idle Worker.
func (s *Scheduler) Resume(ctx context.Context, id string) (types.Sandbox, error) {
	resumeStart := time.Now()
	sb, err := s.store.GetSandbox(ctx, id)
	if err != nil {
		return types.Sandbox{}, err
	}
	if sb.State != types.SandboxSuspended && sb.State != types.SandboxPaused {
		return types.Sandbox{}, fmt.Errorf("sandbox %s is %s, want suspended|paused", id, sb.State)
	}
	prevWorkerID := sb.WorkerID

	workers, err := s.healthyWorkers(ctx)
	if err != nil {
		return types.Sandbox{}, err
	}
	running, err := s.running(ctx)
	if err != nil {
		return types.Sandbox{}, err
	}

	if sb.State == types.SandboxPaused {
		sticky, ok := workerByID(workers, sb.WorkerID)
		if !ok {
			return types.Sandbox{}, fmt.Errorf("paused sandbox requires sticky worker %s", sb.WorkerID)
		}
		workers = []types.Worker{sticky}
		running = filterRunningOn(running, sb.WorkerID)
	} else if sb.SnapshotSource == types.SnapshotLocal && sb.WorkerID != "" {
		if sticky, ok := workerByID(workers, sb.WorkerID); ok && sticky.FreeSlots() > 0 {
			if exists, err := s.workers.LocalSnapshotExists(ctx, sticky.Address, sb.LocalSnapshotPath); err == nil && exists {
				workers = []types.Worker{sticky}
				running = filterRunningOn(running, sb.WorkerID)
			}
		}
	} else if sb.WorkerID != "" && sb.SnapshotSource == types.SnapshotExternal {
		if sticky, ok := workerByID(workers, sb.WorkerID); ok && sticky.FreeSlots() > 0 {
			if exists, err := s.workers.LocalSnapshotExists(ctx, sticky.Address, sb.LocalSnapshotPath); err == nil && exists {
				workers = []types.Worker{sticky}
				running = filterRunningOn(running, sb.WorkerID)
			}
		}
	}

	decStart := time.Now()
	now := decStart
	sandboxSig, workerSig := s.signalViews(now)
	res, err := s.policy.Resume(ctx, policy.ResumeRequest{
		Sandbox: sb, Workers: workers, Running: running,
		SandboxSignals: sandboxSig, WorkerSignals: workerSig,
	})
	s.metrics.RecordDecisionLatency(ctx, time.Since(decStart))
	if err != nil {
		s.metrics.RecordDecision(ctx, "resume", "error", err.Error())
		return types.Sandbox{}, err
	}
	if res.VictimID != "" {
		evictStart := time.Now()
		if s.signals != nil {
			s.signals.OnEvict(res.VictimID)
		}
		if _, err := s.suspendLocked(ctx, res.VictimID); err != nil {
			s.metrics.RecordDecision(ctx, "evict", "error", err.Error())
			return types.Sandbox{}, fmt.Errorf("evict %s: %w", res.VictimID, err)
		}
		s.metrics.RecordEviction(ctx, res.Reason)
		s.metrics.RecordPreemptCost(ctx, time.Since(evictStart))
		s.metrics.RecordDecision(ctx, "evict", "ok", res.Reason)
	}
	w, err := s.store.GetWorker(ctx, res.WorkerID)
	if err != nil {
		return types.Sandbox{}, err
	}

	objectKey := sb.ObjectKey
	localPath := sb.LocalSnapshotPath
	localOnly := false
	usedGolden := false
	switch {
	case sb.State == types.SandboxPaused || sb.SnapshotSource == types.SnapshotLocal:
		if localPath == "" {
			localPath = s.localPath(w.ID, id)
		}
		objectKey = "" // local only
		localOnly = true
	case sb.SnapshotSource == types.SnapshotExternal && objectKey != "":
		if localPath == "" || w.ID != sb.WorkerID {
			localPath = s.localPath(w.ID, id)
		}
	default:
		// First resume (or no latest): Golden.
		g, err := s.EnsureGolden(ctx)
		if err != nil {
			return types.Sandbox{}, err
		}
		objectKey = g
		localPath = s.localPath(w.ID, id)
		usedGolden = true
	}

	path := metrics.ClassifyResumePath(prevWorkerID, w.ID, localOnly, objectKey, usedGolden)
	action := "migrate"
	switch path {
	case metrics.PathStickyLocal:
		action = "sticky"
	case metrics.PathGoldenCold:
		action = "place"
	}
	s.metrics.RecordDecision(ctx, action, "ok", res.Reason)
	s.metrics.RecordResumeWait(ctx, time.Since(resumeStart))

	opts := workerclient.RestoreOpts{ImagePath: localPath, ObjectKey: objectKey}
	restoreStart := time.Now()
	if err := s.workers.Restore(ctx, w.Address, id, opts); err != nil {
		return types.Sandbox{}, err
	}
	s.metrics.RecordRestoreLatency(ctx, path, time.Since(restoreStart))
	if s.signals != nil {
		s.signals.RecordRestore(id, time.Now().UTC(), time.Since(restoreStart), time.Now().UTC())
	}

	now = time.Now().UTC()
	sb.State = types.SandboxRunning
	sb.WorkerID = w.ID
	sb.LocalSnapshotPath = localPath
	sb.UpdatedAt = now
	if err := s.store.PutSandbox(ctx, sb); err != nil {
		return types.Sandbox{}, err
	}
	s.metrics.MarkRunning(ctx, id, w.ID, now)
	s.metrics.RecordResumePath(ctx, path)
	s.metrics.RecordResumeLatency(ctx, path, time.Since(resumeStart))
	s.logDecision(types.Decision{
		Policy: s.policy.Name(), Action: "resume",
		SandboxID: id, WorkerID: w.ID, VictimID: res.VictimID, Reason: res.Reason,
	})
	return sb, nil
}

func (s *Scheduler) Pause(ctx context.Context, id string) (types.Sandbox, error) {
	return s.pauseOrSuspend(ctx, id, false)
}

func (s *Scheduler) Suspend(ctx context.Context, id string) (types.Sandbox, error) {
	return s.suspendLocked(ctx, id)
}

func (s *Scheduler) suspendLocked(ctx context.Context, id string) (types.Sandbox, error) {
	return s.pauseOrSuspend(ctx, id, true)
}

func (s *Scheduler) pauseOrSuspend(ctx context.Context, id string, upload bool) (types.Sandbox, error) {
	sb, err := s.store.GetSandbox(ctx, id)
	if err != nil {
		return types.Sandbox{}, err
	}
	if sb.State != types.SandboxRunning {
		return types.Sandbox{}, fmt.Errorf("sandbox %s is %s, want running", id, sb.State)
	}
	w, err := s.store.GetWorker(ctx, sb.WorkerID)
	if err != nil {
		return types.Sandbox{}, err
	}
	path := s.localPath(w.ID, id)
	opts := workerclient.CheckpointOpts{ImagePath: path}
	mode := "pause"
	if upload {
		opts.ObjectKey = snapshotstore.ObjectKeyFor(id)
		mode = "suspend"
	}
	cpStart := time.Now()
	if err := s.workers.Checkpoint(ctx, w.Address, id, opts); err != nil {
		return types.Sandbox{}, err
	}
	s.metrics.RecordCheckpointLatency(ctx, mode, time.Since(cpStart))
	if s.signals != nil {
		s.signals.RecordCheckpoint(id, signals.SnapshotResource{
			LastCheckpointBytes: dirSize(path),
			LastPreemptCostSec:  time.Since(cpStart).Seconds(),
			LastCheckpointAt:    time.Now().UTC(),
			LastCheckpointDur:   time.Since(cpStart),
		}, time.Now().UTC())
	}
	now := time.Now().UTC()
	sb.LocalSnapshotPath = path
	sb.UpdatedAt = now
	if upload {
		sb.State = types.SandboxSuspended
		sb.ObjectKey = opts.ObjectKey
		sb.SnapshotSource = types.SnapshotExternal
	} else {
		sb.State = types.SandboxPaused
		sb.SnapshotSource = types.SnapshotLocal
	}
	if err := s.store.PutSandbox(ctx, sb); err != nil {
		return types.Sandbox{}, err
	}
	s.metrics.MarkSlotFreed(ctx, id, w.ID, now)
	action := "pause"
	if upload {
		action = "suspend"
	}
	s.logDecision(types.Decision{Policy: s.policy.Name(), Action: action, SandboxID: id, WorkerID: w.ID})
	s.metrics.RecordDecision(ctx, action, "ok", "")
	return sb, nil
}

func (s *Scheduler) Delete(ctx context.Context, id string) error {
	sb, err := s.store.GetSandbox(ctx, id)
	if err != nil {
		return err
	}
	if sb.State == types.SandboxRunning && sb.WorkerID != "" {
		if w, err := s.store.GetWorker(ctx, sb.WorkerID); err == nil {
			_ = s.workers.Delete(ctx, w.Address, id)
		}
		s.metrics.MarkSlotFreed(ctx, id, sb.WorkerID, time.Now().UTC())
	}
	return s.store.DeleteSandbox(ctx, id)
}

func (s *Scheduler) Exec(ctx context.Context, id string, argv []string) (string, error) {
	sb, err := s.store.GetSandbox(ctx, id)
	if err != nil {
		return "", err
	}
	if sb.State != types.SandboxRunning {
		return "", fmt.Errorf("sandbox %s is %s, want running", id, sb.State)
	}
	w, err := s.store.GetWorker(ctx, sb.WorkerID)
	if err != nil {
		return "", err
	}
	return s.workers.Exec(ctx, w.Address, id, argv)
}

func (s *Scheduler) Get(ctx context.Context, id string) (types.Sandbox, error) {
	return s.store.GetSandbox(ctx, id)
}

func (s *Scheduler) List(ctx context.Context) ([]types.Sandbox, error) {
	return s.store.ListSandboxes(ctx)
}

func workerByID(workers []types.Worker, id string) (types.Worker, bool) {
	for _, w := range workers {
		if w.ID == id {
			return w, true
		}
	}
	return types.Worker{}, false
}

func filterRunningOn(running []types.Sandbox, workerID string) []types.Sandbox {
	out := make([]types.Sandbox, 0, len(running))
	for _, sb := range running {
		if sb.WorkerID == workerID {
			out = append(out, sb)
		}
	}
	return out
}

func dirSize(root string) uint64 {
	if root == "" {
		return 0
	}
	var total uint64
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Size() > 0 {
			total += uint64(info.Size())
		}
		return nil
	})
	return total
}
