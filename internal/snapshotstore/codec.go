// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package snapshotstore

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/klauspost/compress/zstd"
)

type writeContentResult struct {
	logicalBytes   int64
	populatedBytes int64
	sparse         bool
}

// writeContent compresses content: sparse-extent for *os.File, plain zstd otherwise.
func writeContent(out io.Writer, content io.Reader) (writeContentResult, error) {
	if f, ok := content.(*os.File); ok {
		logical, populated, err := writeSparseZstd(out, f)
		if err != nil {
			return writeContentResult{}, err
		}
		return writeContentResult{logicalBytes: logical, populatedBytes: populated, sparse: true}, nil
	}
	logical, err := plainZstd(out, content)
	if err != nil {
		return writeContentResult{}, err
	}
	return writeContentResult{logicalBytes: logical, populatedBytes: logical}, nil
}

func plainZstd(w io.Writer, src io.Reader) (int64, error) {
	zw, err := zstd.NewWriter(w,
		zstd.WithEncoderLevel(zstd.SpeedFastest),
		zstd.WithEncoderConcurrency(runtime.GOMAXPROCS(0)))
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(zw, src)
	if err != nil {
		_ = zw.Close()
		return n, err
	}
	return n, zw.Close()
}

type decodeContentResult struct {
	logicalBytes int64
	writtenBytes int64
	sparse       bool
}

// decodeContent auto-detects ATESPRSE vs plain zstd.
func decodeContent(out io.Writer, src io.Reader) (decodeContentResult, error) {
	magic := make([]byte, len(sparseMagic))
	n, rerr := io.ReadFull(src, magic)
	if rerr == nil && string(magic) == sparseMagic {
		f, ok := out.(*os.File)
		if !ok {
			return decodeContentResult{}, fmt.Errorf("sparse-extent snapshot requires a file destination, got %T", out)
		}
		size, derr := readSparseZstd(f, src)
		if derr != nil {
			return decodeContentResult{}, fmt.Errorf("sparse-extent decode: %w", derr)
		}
		return decodeContentResult{logicalBytes: size, sparse: true}, nil
	}
	if rerr != nil && rerr != io.EOF && rerr != io.ErrUnexpectedEOF {
		return decodeContentResult{}, fmt.Errorf("reading object header: %w", rerr)
	}

	r := io.MultiReader(bytes.NewReader(magic[:n]), src)
	zrc, err := zstd.NewReader(r, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return decodeContentResult{}, fmt.Errorf("zstd.NewReader: %w", err)
	}
	defer zrc.Close()
	if f, ok := out.(*os.File); ok {
		size, written, derr := copyZstdSparse(f, zrc)
		if derr != nil {
			return decodeContentResult{}, fmt.Errorf("sparse decompress: %w", derr)
		}
		return decodeContentResult{logicalBytes: size, writtenBytes: written}, nil
	}
	size, cerr := io.Copy(out, zrc)
	if cerr != nil {
		return decodeContentResult{}, fmt.Errorf("io.Copy: %w", cerr)
	}
	return decodeContentResult{logicalBytes: size}, nil
}

func copyZstdSparse(dst *os.File, src io.Reader) (size int64, written int64, err error) {
	if err := dst.Truncate(0); err != nil {
		return 0, 0, fmt.Errorf("truncating dst: %w", err)
	}
	const block = 64 << 10
	buf := make([]byte, block)
	var pos int64
	for {
		n, rerr := io.ReadFull(src, buf)
		if n > 0 {
			chunk := buf[:n]
			if !allZero(chunk) {
				if _, werr := dst.WriteAt(chunk, pos); werr != nil {
					return 0, 0, werr
				}
				written += int64(n)
			}
			pos += int64(n)
		}
		if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
			break
		}
		if rerr != nil {
			return 0, 0, rerr
		}
	}
	if terr := dst.Truncate(pos); terr != nil {
		return 0, 0, terr
	}
	return pos, written, nil
}

func allZero(b []byte) bool {
	i := 0
	for ; i+8 <= len(b); i += 8 {
		if binary.LittleEndian.Uint64(b[i:]) != 0 {
			return false
		}
	}
	for ; i < len(b); i++ {
		if b[i] != 0 {
			return false
		}
	}
	return true
}
