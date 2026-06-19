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
	Delete(ctx context.Context, sandboxID string) error
}

type actorDeleter interface {
	DeleteSandbox(ctx context.Context, actorID string) error
}

// Expirer removes sandboxes whose TTL has elapsed.
type Expirer struct {
	store  sandboxStore
	actors actorDeleter
}

func NewExpirer(st sandboxStore, actors actorDeleter) *Expirer {
	return &Expirer{store: st, actors: actors}
}

// ExpireSandbox kills and purges a sandbox when on_timeout is kill.
// Missing records and pause lifecycle are skipped without error.
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
	if onTimeout != store.OnTimeoutKill {
		return nil
	}

	if err := sandbox.Purge(ctx, e.actors, e.store, sandboxID); err != nil {
		return fmt.Errorf("purge sandbox: %w", err)
	}
	return nil
}

var _ sandboxStore = (*store.Redis)(nil)
var _ actorDeleter = (*substrate.Client)(nil)
