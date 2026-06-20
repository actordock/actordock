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
	"fmt"

	"github.com/actordock/actordock/internal/sandbox"
	"github.com/actordock/actordock/internal/store"
	"github.com/actordock/actordock/internal/substrate"
)

type sandboxStore interface {
	Get(ctx context.Context, sandboxID string) (store.Sandbox, error)
	Put(ctx context.Context, sb store.Sandbox) error
	Delete(ctx context.Context, sandboxID string) error
}

type actorLifecycle interface {
	SuspendSandbox(ctx context.Context, actorID string) error
	DeleteSandbox(ctx context.Context, actorID string) error
}

// Expirer removes sandboxes whose TTL has elapsed.
type Expirer struct {
	store  sandboxStore
	actors actorLifecycle
}

func NewExpirer(st sandboxStore, actors actorLifecycle) *Expirer {
	return &Expirer{store: st, actors: actors}
}

// ExpireSandbox handles TTL expiry: kill+purge when on_timeout is kill,
// suspend+mark paused when on_timeout is pause.
func (e *Expirer) ExpireSandbox(ctx context.Context, sandboxID string) error {
	sb, err := e.store.Get(ctx, sandboxID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get sandbox: %w", err)
	}

	onTimeout, err := store.ResolveOnTimeout(sb.OnTimeout)
	if err != nil {
		return fmt.Errorf("resolve on_timeout: %w", err)
	}
	switch onTimeout {
	case store.OnTimeoutKill:
		if err := sandbox.Purge(ctx, e.actors, e.store, sandboxID); err != nil {
			return fmt.Errorf("purge sandbox: %w", err)
		}
	case store.OnTimeoutPause:
		if sb.Status == store.StatusPaused {
			return nil
		}
		if err := e.actors.SuspendSandbox(ctx, sb.ActorID); err != nil {
			if errors.Is(err, substrate.ErrNotFound) {
				return sandbox.Purge(ctx, e.actors, e.store, sandboxID)
			}
			return fmt.Errorf("suspend sandbox: %w", err)
		}
		sb.Status = store.StatusPaused
		if err := e.store.Put(ctx, sb); err != nil {
			return fmt.Errorf("update sandbox status: %w", err)
		}
	}
	return nil
}

var _ sandboxStore = (*store.Redis)(nil)
var _ actorLifecycle = (*substrate.Client)(nil)
