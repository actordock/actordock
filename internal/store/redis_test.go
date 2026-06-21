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
	"reflect"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestRedisPutGetDelete(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	s, err := NewRedis(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	createdAt := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	expiresAt := createdAt.Add(300 * time.Second)
	sb := Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
		Status:    StatusRunning,
	}

	if err := s.Put(ctx, sb); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := s.Get(ctx, "sb-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(got, sb) {
		t.Fatalf("Get = %+v, want %+v", got, sb)
	}

	if err := s.Delete(ctx, "sb-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, "sb-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}
}

func TestRedisList(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	s, err := NewRedis(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	for _, id := range []string{"a", "b"} {
		if err := s.Put(ctx, Sandbox{
			SandboxID: id,
			ActorID:   id,
			Template:  "base",
			CreatedAt: now,
			Status:    StatusRunning,
		}); err != nil {
			t.Fatalf("Put %s: %v", id, err)
		}
	}

	list, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List len = %d, want 2", len(list))
	}
}

func TestRedisGetNotFound(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	s, err := NewRedis(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	defer s.Close()

	_, err = s.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get = %v, want ErrNotFound", err)
	}
}

func TestRedisDeleteNotFound(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	s, err := NewRedis(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	defer s.Close()

	err = s.Delete(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete = %v, want ErrNotFound", err)
	}
}
