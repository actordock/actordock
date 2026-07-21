// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

// Package runtime defines the pluggable sandbox isolation backend.
package runtime

import "context"

// SandboxSpec describes how to create a sandbox.
type SandboxSpec struct {
	ID     string
	Rootfs string
}

// Runtime isolates sandboxes and supports checkpoint/restore (Substrate ateom-shaped).
type Runtime interface {
	// Boot cold-starts a sandbox (used only to build the golden snapshot).
	Boot(ctx context.Context, spec SandboxSpec) error
	Checkpoint(ctx context.Context, id, imagePath string) error
	Restore(ctx context.Context, id, imagePath string) error
	Delete(ctx context.Context, id string) error
	Exists(ctx context.Context, id string) (bool, error)
	// Exec runs a command inside a running sandbox (e2e / debug).
	Exec(ctx context.Context, id string, argv []string) (stdout string, err error)
}
