// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package snapshotstore

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/klauspost/compress/zstd"
	"golang.org/x/sys/unix"
)

// Sparse-extent format matches Substrate ategcs (ATESPRSE v2):
//
//	magic[8] | version:u32 | zstd( totalSize:i64 | (off:i64, len:i64, data[len])* | -1:i64 )
//
// Only populated extents are compressed; holes are skipped (critical for guest RAM images).

const sparseMagic = "ATESPRSE"

const sparseVersion uint32 = 2

const sparseEndOffset int64 = -1

func writeSparseZstd(dst io.Writer, src *os.File) (logical, dataBytes int64, err error) {
	fi, err := src.Stat()
	if err != nil {
		return 0, 0, err
	}
	size := fi.Size()

	bw := bufio.NewWriter(dst)
	if _, err := bw.WriteString(sparseMagic); err != nil {
		return 0, 0, err
	}
	if err := binary.Write(bw, binary.LittleEndian, sparseVersion); err != nil {
		return 0, 0, err
	}
	if err := bw.Flush(); err != nil {
		return 0, 0, err
	}

	zw, err := zstd.NewWriter(dst,
		zstd.WithEncoderLevel(zstd.SpeedFastest),
		zstd.WithEncoderConcurrency(runtime.GOMAXPROCS(0)))
	if err != nil {
		return 0, 0, err
	}
	fail := func(e error) (int64, int64, error) {
		_ = zw.Close()
		return 0, 0, e
	}
	if err := binary.Write(zw, binary.LittleEndian, size); err != nil {
		return fail(err)
	}

	fd := int(src.Fd())
	off := int64(0)
	for off < size {
		ds, serr := unix.Seek(fd, off, unix.SEEK_DATA)
		if serr != nil {
			if serr == unix.ENXIO {
				break
			}
			return fail(fmt.Errorf("SEEK_DATA: %w", serr))
		}
		de, serr := unix.Seek(fd, ds, unix.SEEK_HOLE)
		if serr != nil {
			return fail(fmt.Errorf("SEEK_HOLE: %w", serr))
		}
		length := de - ds
		if err := binary.Write(zw, binary.LittleEndian, ds); err != nil {
			return fail(err)
		}
		if err := binary.Write(zw, binary.LittleEndian, length); err != nil {
			return fail(err)
		}
		if _, err := src.Seek(ds, io.SeekStart); err != nil {
			return fail(err)
		}
		n, cerr := io.CopyN(zw, src, length)
		dataBytes += n
		if cerr != nil {
			return fail(fmt.Errorf("reading extent @%d+%d: %w", ds, length, cerr))
		}
		off = de
	}
	if err := binary.Write(zw, binary.LittleEndian, sparseEndOffset); err != nil {
		return fail(err)
	}
	if err := zw.Close(); err != nil {
		return 0, 0, err
	}
	return size, dataBytes, nil
}

// readSparseZstd decodes after the caller has already consumed sparseMagic.
func readSparseZstd(dst *os.File, src io.Reader) (logical int64, err error) {
	var ver uint32
	if err := binary.Read(src, binary.LittleEndian, &ver); err != nil {
		return 0, fmt.Errorf("reading sparse format version: %w", err)
	}
	if ver != sparseVersion {
		return 0, fmt.Errorf("unsupported sparse snapshot format version %d (want %d)", ver, sparseVersion)
	}

	zr, err := zstd.NewReader(src, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return 0, err
	}
	defer zr.Close()

	var size int64
	if err := binary.Read(zr, binary.LittleEndian, &size); err != nil {
		return 0, fmt.Errorf("reading totalSize: %w", err)
	}
	if size < 0 {
		return 0, fmt.Errorf("negative totalSize %d", size)
	}
	if err := dst.Truncate(size); err != nil {
		return 0, err
	}

	for {
		var off int64
		if err := binary.Read(zr, binary.LittleEndian, &off); err != nil {
			return 0, fmt.Errorf("reading extent offset: %w", err)
		}
		if off == sparseEndOffset {
			break
		}
		var length int64
		if err := binary.Read(zr, binary.LittleEndian, &length); err != nil {
			return 0, fmt.Errorf("reading extent length: %w", err)
		}
		if off < 0 || length < 0 || off > size || length > size-off {
			return 0, fmt.Errorf("sparse extent out of range (off=%d len=%d size=%d)", off, length, size)
		}
		if _, err := dst.Seek(off, io.SeekStart); err != nil {
			return 0, err
		}
		if _, err := io.CopyN(dst, zr, length); err != nil {
			return 0, fmt.Errorf("writing extent @%d+%d: %w", off, length, err)
		}
	}
	return size, nil
}
