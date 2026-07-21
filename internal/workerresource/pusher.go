// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package workerresource

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/actordock/actordock/internal/signals"
)

// Pusher periodically samples and POSTs resource signals to the control plane.
type Pusher struct {
	WorkerID        string
	ControlPlaneURL string
	Interval        time.Duration
	Log             *slog.Logger
	Client          *http.Client

	Activity   *Activity
	ListActive func() []string
	IsHealthy  func() bool
}

// Run blocks until ctx is cancelled.
func (p *Pusher) Run(ctx context.Context) {
	if p.Client == nil {
		p.Client = &http.Client{Timeout: 5 * time.Second}
	}
	if p.Interval <= 0 {
		p.Interval = 5 * time.Second
	}
	if p.Activity == nil {
		p.Activity = NewActivity()
	}
	tick := time.NewTicker(p.Interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			p.pushOnce(ctx, now)
		}
	}
}

func (p *Pusher) pushOnce(ctx context.Context, now time.Time) {
	if p.ListActive == nil || p.ControlPlaneURL == "" {
		return
	}
	ids := p.ListActive()
	for _, id := range ids {
		if cpu, mem, ok := ReadSandboxCgroup(id); ok {
			p.Activity.SetMetrics(id, cpu, mem)
		}
	}
	cpuW, memW, memBytes, _ := ReadWorkerCgroup()
	healthy := true
	if p.IsHealthy != nil {
		healthy = p.IsHealthy()
	}
	push := signals.Push{
		WorkerID: p.WorkerID,
		Worker: signals.WorkerResource{
			WorkerID:   p.WorkerID,
			MaxSlots:   1,
			UsedSlots:  len(ids),
			Healthy:    healthy,
			CPUUtil:    cpuW,
			MemUtil:    memW,
			MemBytes:   memBytes,
			ReportedAt: now,
		},
	}
	for _, s := range p.Activity.BuildSamples(ids, p.WorkerID, now) {
		push.Samples = append(push.Samples, signals.SandboxSignals{
			SandboxID:  s.SandboxID,
			WorkerID:   s.WorkerID,
			ReportedAt: now,
			Runtime: signals.RuntimeResource{
				CPUUtil:      s.CPUUtil,
				MemRSSBytes:  s.MemRSSBytes,
				LastActiveAt: s.LastActiveAt,
			},
			Snapshot: signals.SnapshotResource{
				CheckpointInProgress: s.CheckpointInProgress,
			},
		})
	}
	b, err := json.Marshal(push)
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.ControlPlaneURL+"/v1/signals/resource", bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.Client.Do(req)
	if err != nil {
		if p.Log != nil {
			p.Log.Warn("push resource signals", "err", err)
		}
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 300 && p.Log != nil {
		p.Log.Warn("push resource signals", "status", resp.StatusCode)
	}
}
