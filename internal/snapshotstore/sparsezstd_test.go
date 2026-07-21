// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package snapshotstore

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSparseZstdRoundTrip(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.img")
	src, err := os.Create(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	const size = 4 << 20
	if err := src.Truncate(size); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte{0xAB}, 64<<10)
	if _, err := src.WriteAt(payload, 1<<20); err != nil {
		t.Fatal(err)
	}
	if err := src.Sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	logical, populated, err := writeSparseZstd(&buf, src)
	_ = src.Close()
	if err != nil {
		t.Fatal(err)
	}
	if logical != size {
		t.Fatalf("logical=%d want %d", logical, size)
	}
	if populated < int64(len(payload)) {
		t.Fatalf("populated=%d too small", populated)
	}

	dstPath := filepath.Join(dir, "dst.img")
	dst, err := os.Create(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()

	raw := buf.Bytes()
	if string(raw[:len(sparseMagic)]) != sparseMagic {
		t.Fatal("missing sparse magic")
	}
	got, err := readSparseZstd(dst, bytes.NewReader(raw[len(sparseMagic):]))
	if err != nil {
		t.Fatal(err)
	}
	if got != size {
		t.Fatalf("decoded size=%d want %d", got, size)
	}

	gotPayload := make([]byte, len(payload))
	if _, err := dst.ReadAt(gotPayload, 1<<20); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Fatal("payload mismatch")
	}
}

func TestUploadDownloadDirFS(t *testing.T) {
	ctx := t.Context()
	storeRoot := t.TempDir()
	st, err := NewFS(storeRoot)
	if err != nil {
		t.Fatal(err)
	}

	local := t.TempDir()
	if err := os.WriteFile(filepath.Join(local, "checkpoint.img"), []byte("hello-checkpoint"), 0o644); err != nil {
		t.Fatal(err)
	}
	prefix := ObjectKeyFor("sb-1")
	if _, err := UploadDir(ctx, st, local, prefix); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "restored")
	if _, err := DownloadDir(ctx, st, prefix, out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "checkpoint.img"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello-checkpoint" {
		t.Fatalf("got %q", b)
	}
}
