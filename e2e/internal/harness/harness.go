// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

// Package harness is the shared Kind/controlplane client for e2e suites.
package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/actordock/actordock/internal/signals"
	"github.com/actordock/actordock/internal/types"
)

const defaultAPI = "http://127.0.0.1:18080"

// Harness talks to a live controlplane (Kind).
type Harness struct {
	API    string
	Client *http.Client
	pfCmd  *exec.Cmd
	t      *testing.T
}

// New connects to the controlplane (port-forward if needed) and cleans sandboxes.
func New(t *testing.T) *Harness {
	t.Helper()
	api := EnvOr("ACTORDOCK_API", defaultAPI)
	h := &Harness{
		API:    api,
		Client: &http.Client{Timeout: 3 * time.Minute},
		t:      t,
	}
	if err := h.ensureAPI(context.Background()); err != nil {
		t.Fatalf("controlplane: %v", err)
	}
	h.CleanupSandboxes(context.Background())
	t.Cleanup(h.close)
	return h
}

func (h *Harness) close() {
	if h.pfCmd != nil && h.pfCmd.Process != nil {
		_ = h.pfCmd.Process.Kill()
		_, _ = h.pfCmd.Process.Wait()
	}
}

func (h *Harness) ensureAPI(ctx context.Context) error {
	if err := h.ping(ctx); err == nil {
		return nil
	}
	if os.Getenv("ACTORDOCK_SKIP_PORT_FORWARD") == "1" {
		return fmt.Errorf("API %s unreachable and port-forward disabled", h.API)
	}
	ns := EnvOr("ACTORDOCK_NAMESPACE", "actordock")
	cmd := exec.CommandContext(ctx, "kubectl", "-n", ns, "port-forward", "svc/controlplane", "18080:8080")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("port-forward: %w", err)
	}
	h.pfCmd = cmd
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if err := h.ping(ctx); err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s/healthz", h.API)
}

func (h *Harness) ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.API+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := h.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz status %s", resp.Status)
	}
	return nil
}

func (h *Harness) WaitWorkers(ctx context.Context, min int) {
	h.t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		ws, err := h.ListWorkers(ctx)
		if err == nil && len(ws) >= min {
			return
		}
		time.Sleep(2 * time.Second)
	}
	h.t.Fatalf("timed out waiting for %d workers", min)
}

func (h *Harness) DoJSON(ctx context.Context, method, path string, in any, out any) {
	h.t.Helper()
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			h.t.Fatal(err)
		}
		body = bytes.NewReader(b)
	} else if method == http.MethodPost || method == http.MethodPut {
		body = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, h.API+path, body)
	if err != nil {
		h.t.Fatal(err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.Client.Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		h.t.Fatalf("%s %s: %s: %s", method, path, resp.Status, bytes.TrimSpace(raw))
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			h.t.Fatalf("decode %s: %v\nbody=%s", path, err, raw)
		}
	}
}

func (h *Harness) CreateSandbox(ctx context.Context) types.Sandbox {
	h.t.Helper()
	var sb types.Sandbox
	h.DoJSON(ctx, http.MethodPost, "/v1/sandboxes", nil, &sb)
	return sb
}

func (h *Harness) WaitGolden(ctx context.Context) {
	h.t.Helper()
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.API+"/v1/golden/ensure", nil)
		if err != nil {
			h.t.Fatal(err)
		}
		resp, err := h.Client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 300 {
				return
			}
		}
		time.Sleep(3 * time.Second)
	}
	h.t.Fatal("timed out waiting for golden snapshot")
}

func (h *Harness) CleanupSandboxes(ctx context.Context) {
	h.t.Helper()
	list := h.ListSandboxes(ctx)
	for _, sb := range list {
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, h.API+"/v1/sandboxes/"+sb.ID, nil)
		if err != nil {
			continue
		}
		resp, err := h.Client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
	}
}

func (h *Harness) GetSandbox(ctx context.Context, id string) types.Sandbox {
	h.t.Helper()
	var sb types.Sandbox
	h.DoJSON(ctx, http.MethodGet, "/v1/sandboxes/"+id, nil, &sb)
	return sb
}

func (h *Harness) ListSandboxes(ctx context.Context) []types.Sandbox {
	h.t.Helper()
	var list []types.Sandbox
	h.DoJSON(ctx, http.MethodGet, "/v1/sandboxes", nil, &list)
	return list
}

func (h *Harness) ListWorkers(ctx context.Context) ([]types.Worker, error) {
	var list []types.Worker
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.API+"/v1/workers", nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list workers: %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	return list, nil
}

func (h *Harness) Pause(ctx context.Context, id string) types.Sandbox {
	h.t.Helper()
	var sb types.Sandbox
	h.DoJSON(ctx, http.MethodPost, "/v1/sandboxes/"+id+"/pause", nil, &sb)
	return sb
}

func (h *Harness) Suspend(ctx context.Context, id string) types.Sandbox {
	h.t.Helper()
	var sb types.Sandbox
	h.DoJSON(ctx, http.MethodPost, "/v1/sandboxes/"+id+"/suspend", nil, &sb)
	return sb
}

func (h *Harness) Resume(ctx context.Context, id string) types.Sandbox {
	h.t.Helper()
	var sb types.Sandbox
	h.DoJSON(ctx, http.MethodPost, "/v1/sandboxes/"+id+"/resume", nil, &sb)
	return sb
}

func (h *Harness) Exec(ctx context.Context, id string, argv ...string) string {
	h.t.Helper()
	var out struct {
		Stdout string `json:"stdout"`
	}
	h.DoJSON(ctx, http.MethodPost, "/v1/sandboxes/"+id+"/exec", map[string]any{"argv": argv}, &out)
	return out.Stdout
}

func (h *Harness) WriteFile(ctx context.Context, id, path, content string) {
	h.t.Helper()
	_ = h.Exec(ctx, id, "/bin/busybox", "sh", "-c", "printf '%s' '"+content+"' > "+path)
}

func (h *Harness) ReadFile(ctx context.Context, id, path string) string {
	h.t.Helper()
	return h.Exec(ctx, id, "/bin/busybox", "cat", path)
}

func (h *Harness) WorkerBusy(ctx context.Context, workerID string) bool {
	h.t.Helper()
	for _, sb := range h.ListSandboxes(ctx) {
		if sb.State == types.SandboxRunning && sb.WorkerID == workerID {
			return true
		}
	}
	return false
}

// OccupyWorker resumes fillers until workerID hosts a running sandbox.
func (h *Harness) OccupyWorker(ctx context.Context, workerID string) {
	h.t.Helper()
	for i := 0; i < 32; i++ {
		if h.WorkerBusy(ctx, workerID) {
			return
		}
		sb := h.CreateSandbox(ctx)
		_ = h.Resume(ctx, sb.ID)
	}
	h.t.Fatalf("could not occupy worker %s", workerID)
}

// EnsureIdleExcept suspends one running sandbox not on exceptWorker so resume
// has a free Worker that is not the origin.
func (h *Harness) EnsureIdleExcept(ctx context.Context, exceptWorker string) {
	h.t.Helper()
	workers, err := h.ListWorkers(ctx)
	if err != nil {
		h.t.Fatal(err)
	}
	busy := map[string]bool{}
	var candidates []types.Sandbox
	for _, sb := range h.ListSandboxes(ctx) {
		if sb.State == types.SandboxRunning {
			busy[sb.WorkerID] = true
			if sb.WorkerID != exceptWorker {
				candidates = append(candidates, sb)
			}
		}
	}
	free := 0
	for _, w := range workers {
		if w.Healthy && !busy[w.ID] && w.ID != exceptWorker {
			free++
		}
	}
	if free > 0 {
		return
	}
	if len(candidates) == 0 {
		h.t.Fatalf("no non-origin running sandbox to free; except=%s", exceptWorker)
	}
	_ = h.Suspend(ctx, candidates[0].ID)
}

func (h *Harness) Policy(ctx context.Context) string {
	h.t.Helper()
	var out struct {
		Policy string `json:"policy"`
	}
	h.DoJSON(ctx, http.MethodGet, "/v1/policy", nil, &out)
	return out.Policy
}

// SetPolicy restarts controlplane with POLICY=<name> and waits until /v1/policy matches.
func (h *Harness) SetPolicy(ctx context.Context, name string) {
	h.t.Helper()
	ns := EnvOr("ACTORDOCK_NAMESPACE", "actordock")
	set := exec.CommandContext(ctx, "kubectl", "-n", ns, "set", "env", "deployment/controlplane", "POLICY="+name)
	if out, err := set.CombinedOutput(); err != nil {
		h.t.Fatalf("set policy %s: %v: %s", name, err, out)
	}
	roll := exec.CommandContext(ctx, "kubectl", "-n", ns, "rollout", "status", "deployment/controlplane", "--timeout=180s")
	if out, err := roll.CombinedOutput(); err != nil {
		h.t.Fatalf("rollout controlplane: %v: %s", err, out)
	}
	h.close()
	h.pfCmd = nil
	if err := h.ensureAPI(ctx); err != nil {
		h.t.Fatalf("api after policy change: %v", err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if h.Policy(ctx) == name {
			h.CleanupSandboxes(ctx)
			return
		}
		time.Sleep(time.Second)
	}
	h.t.Fatalf("policy still %q, want %q", h.Policy(ctx), name)
}

func (h *Harness) FetchMetrics(ctx context.Context) string {
	h.t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.API+"/metrics", nil)
	if err != nil {
		h.t.Fatal(err)
	}
	resp, err := h.Client.Do(req)
	if err != nil {
		h.t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	if resp.StatusCode >= 300 {
		h.t.Fatalf("GET /metrics: %s: %s", resp.Status, bytes.TrimSpace(raw))
	}
	return string(raw)
}

func (h *Harness) ListSandboxSignals(ctx context.Context) []signals.SandboxSignals {
	h.t.Helper()
	var list []signals.SandboxSignals
	h.DoJSON(ctx, http.MethodGet, "/v1/signals/sandboxes", nil, &list)
	return list
}

func (h *Harness) GetSandboxSignals(ctx context.Context, id string) signals.SandboxSignals {
	h.t.Helper()
	var sig signals.SandboxSignals
	h.DoJSON(ctx, http.MethodGet, "/v1/signals/sandboxes/"+id, nil, &sig)
	return sig
}

func (h *Harness) ListWorkerSignals(ctx context.Context) []signals.WorkerResource {
	h.t.Helper()
	var list []signals.WorkerResource
	h.DoJSON(ctx, http.MethodGet, "/v1/signals/workers", nil, &list)
	return list
}

// WaitPositiveResourceSignals polls until sandboxID and its worker have all
// numeric resource-plugin metrics > 0 (and healthy / timestamps set).
func (h *Harness) WaitPositiveResourceSignals(ctx context.Context, sandboxID, workerID string, timeout time.Duration) {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	var lastSB signals.SandboxSignals
	var lastW signals.WorkerResource
	for time.Now().Before(deadline) {
		okSB, okW := false, false
		for _, sig := range h.ListSandboxSignals(ctx) {
			if sig.SandboxID == sandboxID {
				lastSB = sig
				okSB = sandboxSignalsAllPositive(sig)
				break
			}
		}
		for _, w := range h.ListWorkerSignals(ctx) {
			if w.WorkerID == workerID {
				lastW = w
				okW = workerSignalsAllPositive(w)
				break
			}
		}
		if okSB && okW {
			return
		}
		time.Sleep(time.Second)
	}
	h.t.Fatalf("timeout waiting for positive resource signals\nsandbox=%+v\nworker=%+v", lastSB, lastW)
}

func sandboxSignalsAllPositive(sig signals.SandboxSignals) bool {
	rt := sig.Runtime
	snap := sig.Snapshot
	return rt.CPUUtil > 0 &&
		rt.MemRSSBytes > 0 &&
		!rt.LastActiveAt.IsZero() &&
		snap.LastCheckpointBytes > 0 &&
		snap.LastPreemptCostSec > 0 &&
		!snap.LastCheckpointAt.IsZero() &&
		snap.LastCheckpointDur > 0 &&
		!snap.LastRestoreAt.IsZero() &&
		snap.LastRestoreDur > 0 &&
		sig.KeepAliveH > 0 &&
		!sig.ReportedAt.IsZero() &&
		sig.WorkerID != ""
}

func workerSignalsAllPositive(w signals.WorkerResource) bool {
	return w.WorkerID != "" &&
		w.MaxSlots > 0 &&
		w.UsedSlots > 0 &&
		w.Healthy &&
		w.CPUUtil > 0 &&
		w.MemUtil > 0 &&
		w.MemBytes > 0 &&
		!w.ReportedAt.IsZero()
}

func EnvOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func EnvInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	var n int
	for _, c := range v {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}
