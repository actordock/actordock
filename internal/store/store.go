// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"

	"github.com/actordock/actordock/internal/types"
)

// Store persists sandbox/worker metadata and the golden snapshot prefix.
type Store interface {
	PutSandbox(ctx context.Context, sb types.Sandbox) error
	GetSandbox(ctx context.Context, id string) (types.Sandbox, error)
	DeleteSandbox(ctx context.Context, id string) error
	ListSandboxes(ctx context.Context) ([]types.Sandbox, error)

	PutWorker(ctx context.Context, w types.Worker) error
	GetWorker(ctx context.Context, id string) (types.Worker, error)
	DeleteWorker(ctx context.Context, id string) error
	ListWorkers(ctx context.Context) ([]types.Worker, error)

	PutGolden(ctx context.Context, objectPrefix string) error
	GetGolden(ctx context.Context) (objectPrefix string, err error)
}
