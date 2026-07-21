// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/actordock/actordock/internal/types"
)

// Memory is an in-process Store for tests and single-node demos.
type Memory struct {
	mu        sync.RWMutex
	sandboxes map[string]types.Sandbox
	workers   map[string]types.Worker
	golden    string
}

func NewMemory() *Memory {
	return &Memory{
		sandboxes: make(map[string]types.Sandbox),
		workers:   make(map[string]types.Worker),
	}
}

func (m *Memory) PutSandbox(_ context.Context, sb types.Sandbox) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sandboxes[sb.ID] = sb
	return nil
}

func (m *Memory) GetSandbox(_ context.Context, id string) (types.Sandbox, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sb, ok := m.sandboxes[id]
	if !ok {
		return types.Sandbox{}, fmt.Errorf("sandbox %q not found", id)
	}
	return sb, nil
}

func (m *Memory) DeleteSandbox(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sandboxes, id)
	return nil
}

func (m *Memory) ListSandboxes(_ context.Context) ([]types.Sandbox, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]types.Sandbox, 0, len(m.sandboxes))
	for _, sb := range m.sandboxes {
		out = append(out, sb)
	}
	return out, nil
}

func (m *Memory) PutWorker(_ context.Context, w types.Worker) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workers[w.ID] = w
	return nil
}

func (m *Memory) GetWorker(_ context.Context, id string) (types.Worker, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	w, ok := m.workers[id]
	if !ok {
		return types.Worker{}, fmt.Errorf("worker %q not found", id)
	}
	return w, nil
}

func (m *Memory) DeleteWorker(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.workers, id)
	return nil
}

func (m *Memory) ListWorkers(_ context.Context) ([]types.Worker, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]types.Worker, 0, len(m.workers))
	for _, w := range m.workers {
		out = append(out, w)
	}
	return out, nil
}

func (m *Memory) PutGolden(_ context.Context, objectPrefix string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.golden = objectPrefix
	return nil
}

func (m *Memory) GetGolden(_ context.Context) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.golden == "" {
		return "", fmt.Errorf("golden snapshot not ready")
	}
	return m.golden, nil
}

// SnapshotJSON is a debug helper.
func (m *Memory) SnapshotJSON() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, _ := json.MarshalIndent(struct {
		Sandboxes map[string]types.Sandbox `json:"sandboxes"`
		Workers   map[string]types.Worker  `json:"workers"`
		Golden    string                   `json:"golden"`
	}{m.sandboxes, m.workers, m.golden}, "", "  ")
	return string(b)
}
