// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package snapshotstore

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// FS is a directory-backed Store for tests and single-node demos.
type FS struct {
	root string
	mu   sync.Mutex
}

func NewFS(root string) (*FS, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &FS{root: root}, nil
}

func (f *FS) path(key string) string {
	return filepath.Join(f.root, filepath.FromSlash(key))
}

func (f *FS) Put(_ context.Context, key string, r io.Reader, _ int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	out, err := os.Create(p)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, r)
	return err
}

func (f *FS) Get(_ context.Context, key string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.path(key)
	rc, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("snapshot %q: %w", key, err)
	}
	return rc, nil
}

func (f *FS) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return os.Remove(f.path(key))
}
