// Copyright 2026 The Actordock Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/actordock/actordock/internal/runtimeapi"
	"github.com/actordock/actordock/internal/store"
)

type fakeStore struct {
	records map[string]store.Sandbox
	deleted []string
	getErr  error
	delErr  error
}

func newFakeStore() *fakeStore {
	return &fakeStore{records: make(map[string]store.Sandbox)}
}

func (f *fakeStore) Get(_ context.Context, sandboxID string) (store.Sandbox, error) {
	if f.getErr != nil {
		return store.Sandbox{}, f.getErr
	}
	sb, ok := f.records[sandboxID]
	if !ok {
		return store.Sandbox{}, store.ErrNotFound
	}
	return sb, nil
}

func (f *fakeStore) Put(_ context.Context, sb store.Sandbox) error {
	f.records[sb.SandboxID] = sb
	return nil
}

func (f *fakeStore) Delete(_ context.Context, sandboxID string) error {
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.records, sandboxID)
	f.deleted = append(f.deleted, sandboxID)
	return nil
}

type fakeActors struct {
	suspended  []string
	deleted    []string
	suspendErr error
	err        error
}

func (f *fakeActors) SuspendSandbox(_ context.Context, actorID string) error {
	if f.suspendErr != nil {
		return f.suspendErr
	}
	f.suspended = append(f.suspended, actorID)
	return nil
}

func (f *fakeActors) DeleteSandbox(_ context.Context, actorID string) error {
	if f.err != nil {
		return f.err
	}
	f.deleted = append(f.deleted, actorID)
	return nil
}

func TestExpireSandboxKill(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		ExpiresAt: now.Add(-time.Minute),
		OnTimeout: store.OnTimeoutKill,
	}
	actors := &fakeActors{}
	expirer := NewExpirer(st, actors)

	if err := expirer.ExpireSandbox(context.Background(), "sb-1"); err != nil {
		t.Fatalf("ExpireSandbox: %v", err)
	}
	if len(actors.deleted) != 1 || actors.deleted[0] != "sb-1" {
		t.Fatalf("deleted actors = %v", actors.deleted)
	}
	if len(st.deleted) != 1 {
		t.Fatalf("deleted metadata = %v", st.deleted)
	}
	if _, ok := st.records["sb-1"]; ok {
		t.Fatal("record still present")
	}
}

func TestExpireSandboxMissing(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	actors := &fakeActors{}
	expirer := NewExpirer(st, actors)

	if err := expirer.ExpireSandbox(context.Background(), "missing"); err != nil {
		t.Fatalf("ExpireSandbox: %v", err)
	}
	if len(actors.deleted) != 0 {
		t.Fatalf("deleted actors = %v, want none", actors.deleted)
	}
}

func TestExpireSandboxPause(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		ExpiresAt: now.Add(-time.Minute),
		OnTimeout: store.OnTimeoutPause,
		Status:    store.StatusRunning,
	}
	actors := &fakeActors{}
	expirer := NewExpirer(st, actors)

	if err := expirer.ExpireSandbox(context.Background(), "sb-1"); err != nil {
		t.Fatalf("ExpireSandbox: %v", err)
	}
	if len(actors.suspended) != 1 || actors.suspended[0] != "sb-1" {
		t.Fatalf("suspended actors = %v", actors.suspended)
	}
	if len(actors.deleted) != 0 {
		t.Fatalf("deleted actors = %v, want none", actors.deleted)
	}
	if len(st.deleted) != 0 {
		t.Fatalf("deleted metadata = %v, want none", st.deleted)
	}
	got := st.records["sb-1"]
	if got.Status != store.StatusPaused {
		t.Fatalf("status = %q, want %q", got.Status, store.StatusPaused)
	}
}

func TestExpireSandboxPauseAlreadyPaused(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		ExpiresAt: now.Add(-time.Minute),
		OnTimeout: store.OnTimeoutPause,
		Status:    store.StatusPaused,
	}
	actors := &fakeActors{}
	expirer := NewExpirer(st, actors)

	if err := expirer.ExpireSandbox(context.Background(), "sb-1"); err != nil {
		t.Fatalf("ExpireSandbox: %v", err)
	}
	if len(actors.suspended) != 0 {
		t.Fatalf("suspended actors = %v, want none", actors.suspended)
	}
}

func TestExpireSandboxPauseActorMissing(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		ExpiresAt: now.Add(-time.Minute),
		OnTimeout: store.OnTimeoutPause,
		Status:    store.StatusRunning,
	}
	actors := &fakeActors{suspendErr: runtimeapi.ErrNotFound}
	expirer := NewExpirer(st, actors)

	if err := expirer.ExpireSandbox(context.Background(), "sb-1"); err != nil {
		t.Fatalf("ExpireSandbox: %v", err)
	}
	if len(st.deleted) != 1 {
		t.Fatalf("deleted metadata = %v", st.deleted)
	}
}

func TestExpireSandboxPauseActorError(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		ExpiresAt: now.Add(-time.Minute),
		OnTimeout: store.OnTimeoutPause,
		Status:    store.StatusRunning,
	}
	actors := &fakeActors{suspendErr: errors.New("boom")}
	expirer := NewExpirer(st, actors)

	if err := expirer.ExpireSandbox(context.Background(), "sb-1"); err == nil {
		t.Fatal("ExpireSandbox = nil, want error")
	}
	if st.records["sb-1"].Status != store.StatusRunning {
		t.Fatalf("status = %q, want running", st.records["sb-1"].Status)
	}
}

func TestExpireSandboxDefaultOnTimeout(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		ExpiresAt: now.Add(-time.Minute),
	}
	actors := &fakeActors{}
	expirer := NewExpirer(st, actors)

	if err := expirer.ExpireSandbox(context.Background(), "sb-1"); err != nil {
		t.Fatalf("ExpireSandbox: %v", err)
	}
	if len(actors.deleted) != 1 {
		t.Fatalf("deleted actors = %v", actors.deleted)
	}
}

func TestExpireSandboxActorMissing(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		ExpiresAt: now.Add(-time.Minute),
		OnTimeout: store.OnTimeoutKill,
	}
	actors := &fakeActors{err: runtimeapi.ErrNotFound}
	expirer := NewExpirer(st, actors)

	if err := expirer.ExpireSandbox(context.Background(), "sb-1"); err != nil {
		t.Fatalf("ExpireSandbox: %v", err)
	}
	if len(st.deleted) != 1 {
		t.Fatalf("deleted metadata = %v", st.deleted)
	}
}

func TestExpireSandboxActorError(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		ExpiresAt: now.Add(-time.Minute),
		OnTimeout: store.OnTimeoutKill,
	}
	actors := &fakeActors{err: errors.New("boom")}
	expirer := NewExpirer(st, actors)

	if err := expirer.ExpireSandbox(context.Background(), "sb-1"); err == nil {
		t.Fatal("ExpireSandbox = nil, want error")
	}
	if _, ok := st.records["sb-1"]; !ok {
		t.Fatal("record should remain on actor error")
	}
}
