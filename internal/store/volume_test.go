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

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestVolumePutGetDeleteRoundTrip(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	s, err := NewRedis(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	createdAt := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	vol := Volume{
		VolumeID:  "vol-1",
		Name:      "my-data",
		Token:     "secret-token",
		HostPath:  VolumeHostPath("/var/lib/actordock/volumes", "vol-1"),
		CreatedAt: createdAt,
	}

	if err := s.PutVolume(ctx, vol); err != nil {
		t.Fatalf("PutVolume: %v", err)
	}

	got, err := s.GetVolume(ctx, "vol-1")
	if err != nil {
		t.Fatalf("GetVolume: %v", err)
	}
	if got != vol {
		t.Fatalf("GetVolume = %+v, want %+v", got, vol)
	}

	byName, err := s.GetVolumeByName(ctx, "my-data")
	if err != nil {
		t.Fatalf("GetVolumeByName: %v", err)
	}
	if byName != vol {
		t.Fatalf("GetVolumeByName = %+v, want %+v", byName, vol)
	}

	listed, err := s.ListVolumes(ctx)
	if err != nil {
		t.Fatalf("ListVolumes: %v", err)
	}
	if len(listed) != 1 || listed[0] != vol {
		t.Fatalf("ListVolumes = %+v, want [%+v]", listed, vol)
	}

	if err := s.DeleteVolume(ctx, "vol-1"); err != nil {
		t.Fatalf("DeleteVolume: %v", err)
	}
	if _, err := s.GetVolume(ctx, "vol-1"); !errors.Is(err, ErrVolumeNotFound) {
		t.Fatalf("GetVolume after delete = %v, want ErrVolumeNotFound", err)
	}
	if _, err := s.GetVolumeByName(ctx, "my-data"); !errors.Is(err, ErrVolumeNotFound) {
		t.Fatalf("GetVolumeByName after delete = %v, want ErrVolumeNotFound", err)
	}
}

func TestVolumeDuplicateNameRejected(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	s, err := NewRedis(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	first := Volume{
		VolumeID:  "vol-a",
		Name:      "shared-name",
		Token:     "token-a",
		HostPath:  VolumeHostPath("/data", "vol-a"),
		CreatedAt: now,
	}
	second := Volume{
		VolumeID:  "vol-b",
		Name:      "shared-name",
		Token:     "token-b",
		HostPath:  VolumeHostPath("/data", "vol-b"),
		CreatedAt: now,
	}

	if err := s.PutVolume(ctx, first); err != nil {
		t.Fatalf("PutVolume first: %v", err)
	}
	if err := s.PutVolume(ctx, second); !errors.Is(err, ErrVolumeNameTaken) {
		t.Fatalf("PutVolume duplicate name = %v, want ErrVolumeNameTaken", err)
	}
}

func TestVolumeNotFound(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	s, err := NewRedis(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	if _, err := s.GetVolume(ctx, "missing"); !errors.Is(err, ErrVolumeNotFound) {
		t.Fatalf("GetVolume = %v, want ErrVolumeNotFound", err)
	}
	if _, err := s.GetVolumeByName(ctx, "missing"); !errors.Is(err, ErrVolumeNotFound) {
		t.Fatalf("GetVolumeByName = %v, want ErrVolumeNotFound", err)
	}
	if err := s.DeleteVolume(ctx, "missing"); !errors.Is(err, ErrVolumeNotFound) {
		t.Fatalf("DeleteVolume = %v, want ErrVolumeNotFound", err)
	}
}

func TestNewVolumeToken(t *testing.T) {
	t.Parallel()

	a, err := NewVolumeToken()
	if err != nil {
		t.Fatalf("NewVolumeToken: %v", err)
	}
	b, err := NewVolumeToken()
	if err != nil {
		t.Fatalf("NewVolumeToken: %v", err)
	}
	if a == "" || b == "" || a == b {
		t.Fatalf("tokens = %q, %q; want non-empty distinct values", a, b)
	}
}
