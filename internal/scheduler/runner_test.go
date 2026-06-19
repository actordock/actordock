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
	"log/slog"
	"testing"
	"time"

	"github.com/actordock/actordock/internal/config"
	"github.com/actordock/actordock/internal/store"
)

type fakeExpiredLister struct {
	sandboxes []store.Sandbox
	err       error
}

func (f *fakeExpiredLister) ListExpired(_ context.Context, _ time.Time) ([]store.Sandbox, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sandboxes, nil
}

type fakeSandboxExpirer struct {
	attempts []string
	expired  []string
	errs     map[string]error
}

func (f *fakeSandboxExpirer) ExpireSandbox(_ context.Context, sandboxID string) error {
	f.attempts = append(f.attempts, sandboxID)
	if err, ok := f.errs[sandboxID]; ok && err != nil {
		return err
	}
	f.expired = append(f.expired, sandboxID)
	return nil
}

func TestRunnerTickExpiresSandboxes(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	lister := &fakeExpiredLister{
		sandboxes: []store.Sandbox{
			{SandboxID: "a"},
			{SandboxID: "b"},
		},
	}
	expirer := &fakeSandboxExpirer{}
	runner := NewRunner(config.Scheduler{PollInterval: time.Second}, lister, expirer, slog.Default())
	runner.nowFunc = func() time.Time { return now }

	if err := runner.tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(expirer.expired) != 2 || expirer.expired[0] != "a" || expirer.expired[1] != "b" {
		t.Fatalf("expired = %v", expirer.expired)
	}
}

func TestRunnerTickContinuesOnExpireError(t *testing.T) {
	t.Parallel()

	lister := &fakeExpiredLister{
		sandboxes: []store.Sandbox{
			{SandboxID: "fail"},
			{SandboxID: "ok"},
		},
	}
	expirer := &fakeSandboxExpirer{
		errs: map[string]error{"fail": errors.New("boom")},
	}
	runner := NewRunner(config.Scheduler{PollInterval: time.Second}, lister, expirer, slog.Default())

	if err := runner.tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(expirer.attempts) != 2 {
		t.Fatalf("attempts = %v, want both sandboxes", expirer.attempts)
	}
	if len(expirer.expired) != 1 || expirer.expired[0] != "ok" {
		t.Fatalf("expired = %v, want [ok]", expirer.expired)
	}
}

func TestRunnerTickListError(t *testing.T) {
	t.Parallel()

	lister := &fakeExpiredLister{err: errors.New("redis down")}
	runner := NewRunner(config.Scheduler{PollInterval: time.Second}, lister, &fakeSandboxExpirer{}, slog.Default())

	if err := runner.tick(context.Background()); err == nil {
		t.Fatal("tick = nil, want error")
	}
}
