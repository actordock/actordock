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

package sandbox

import (
	"context"
	"errors"
	"fmt"

	"github.com/actordock/actordock/internal/store"
	"github.com/actordock/actordock/internal/runtimeapi"
)

type metadataStore interface {
	Delete(ctx context.Context, sandboxID string) error
}

type actorDeleter interface {
	DeleteSandbox(ctx context.Context, actorID string) error
}

// Purge deletes the runtime actor when present and removes Redis metadata.
// Missing actor or metadata is ignored (idempotent).
func Purge(ctx context.Context, actors actorDeleter, st metadataStore, sandboxID string) error {
	if err := actors.DeleteSandbox(ctx, sandboxID); err != nil && !errors.Is(err, runtimeapi.ErrNotFound) {
		return fmt.Errorf("delete actor: %w", err)
	}
	if err := st.Delete(ctx, sandboxID); err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("delete metadata: %w", err)
	}
	return nil
}

var _ actorDeleter = (*runtimeapi.Client)(nil)
