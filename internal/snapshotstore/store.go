// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

// Package snapshotstore persists Suspend snapshots for cross-Worker resume.
// Layout matches Substrate atelet: object-store prefix + per-file sparse-zstd + manifest.json.
package snapshotstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const manifestName = "manifest.json"

// Store is an object store for sandbox checkpoint objects.
type Store interface {
	Put(ctx context.Context, key string, r io.Reader, size int64) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// Manifest lists checkpoint files under a Suspend prefix (written last, like Substrate).
type Manifest struct {
	Files []string `json:"files"`
}

// ObjectKeyFor returns the object-store prefix for a sandbox checkpoint (not a single blob).
func ObjectKeyFor(sandboxID string) string {
	return fmt.Sprintf("sandboxes/%s", sandboxID)
}

func objectPath(prefix, name string) string {
	return strings.TrimSuffix(prefix, "/") + "/" + name
}

// UploadDir uploads each regular file under localDir as <prefix>/<name>.zstd, then manifest.json.
// It returns total uploaded object bytes (compressed files + manifest).
func UploadDir(ctx context.Context, st Store, localDir, prefix string) (int64, error) {
	files, err := listRegularFiles(localDir)
	if err != nil {
		return 0, err
	}
	if len(files) == 0 {
		return 0, fmt.Errorf("checkpoint dir %q has no files", localDir)
	}

	var total int64
	for _, name := range files {
		local := filepath.Join(localDir, filepath.FromSlash(name))
		key := objectPath(prefix, name+".zstd")
		n, err := uploadFileZstd(ctx, st, key, local)
		if err != nil {
			return total, fmt.Errorf("upload %s: %w", name, err)
		}
		total += n
	}

	man := Manifest{Files: files}
	b, err := json.Marshal(man)
	if err != nil {
		return total, err
	}
	if err := st.Put(ctx, objectPath(prefix, manifestName), bytes.NewReader(b), int64(len(b))); err != nil {
		return total, err
	}
	return total + int64(len(b)), nil
}

// DownloadDir fetches manifest.json then each <file>.zstd into localDir.
// It returns total downloaded object bytes (manifest + compressed blobs).
func DownloadDir(ctx context.Context, st Store, prefix, localDir string) (int64, error) {
	rc, err := st.Get(ctx, objectPath(prefix, manifestName))
	if err != nil {
		return 0, fmt.Errorf("manifest: %w", err)
	}
	b, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		return 0, err
	}
	var man Manifest
	if err := json.Unmarshal(b, &man); err != nil {
		return 0, fmt.Errorf("manifest json: %w", err)
	}
	if len(man.Files) == 0 {
		return 0, fmt.Errorf("empty snapshot manifest under %q", prefix)
	}

	if err := os.RemoveAll(localDir); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return 0, err
	}

	total := int64(len(b))
	for _, name := range man.Files {
		n, err := downloadFileZstd(ctx, st, objectPath(prefix, name+".zstd"), filepath.Join(localDir, filepath.FromSlash(name)))
		if err != nil {
			return total, fmt.Errorf("download %s: %w", name, err)
		}
		total += n
	}
	return total, nil
}

func listRegularFiles(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." || rel == manifestName {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out, err
}

func uploadFileZstd(ctx context.Context, st Store, key, localPath string) (int64, error) {
	src, err := os.Open(localPath)
	if err != nil {
		return 0, err
	}
	defer src.Close()

	tmp, err := os.CreateTemp("", "actordock-snap-*.zstd")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := writeContent(tmp, src); err != nil {
		_ = tmp.Close()
		return 0, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return 0, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		_ = tmp.Close()
		return 0, err
	}
	info, err := tmp.Stat()
	if err != nil {
		_ = tmp.Close()
		return 0, err
	}
	if err := st.Put(ctx, key, tmp, info.Size()); err != nil {
		_ = tmp.Close()
		return 0, err
	}
	return info.Size(), tmp.Close()
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func downloadFileZstd(ctx context.Context, st Store, key, localPath string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return 0, err
	}
	dst, err := os.OpenFile(localPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return 0, err
	}
	defer dst.Close()

	rc, err := st.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	cr := &countingReader{r: rc}
	if _, err := decodeContent(dst, cr); err != nil {
		return cr.n, err
	}
	return cr.n, nil
}
