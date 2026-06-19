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
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestRedisListExpired(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	s, err := NewRedis(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)

	expired := Sandbox{
		SandboxID: "expired",
		ActorID:   "expired",
		Template:  "base",
		CreatedAt: now.Add(-120 * time.Second),
		ExpiresAt: now.Add(-30 * time.Second),
		OnTimeout: OnTimeoutKill,
		Status:    StatusRunning,
	}
	active := Sandbox{
		SandboxID: "active",
		ActorID:   "active",
		Template:  "base",
		CreatedAt: now.Add(-60 * time.Second),
		ExpiresAt: now.Add(60 * time.Second),
		OnTimeout: OnTimeoutKill,
		Status:    StatusRunning,
	}
	noExpiry := Sandbox{
		SandboxID: "no-expiry",
		ActorID:   "no-expiry",
		Template:  "base",
		CreatedAt: now.Add(-120 * time.Second),
		Status:    StatusRunning,
	}
	for _, sb := range []Sandbox{expired, active, noExpiry} {
		if err := s.Put(ctx, sb); err != nil {
			t.Fatalf("Put %s: %v", sb.SandboxID, err)
		}
	}

	got, err := s.ListExpired(ctx, now)
	if err != nil {
		t.Fatalf("ListExpired: %v", err)
	}
	if len(got) != 1 || got[0].SandboxID != "expired" {
		t.Fatalf("ListExpired = %+v, want [expired]", got)
	}

	extended := active
	extended.ExpiresAt = now.Add(120 * time.Second)
	if err := s.Put(ctx, extended); err != nil {
		t.Fatalf("Put extended: %v", err)
	}
	got, err = s.ListExpired(ctx, now)
	if err != nil {
		t.Fatalf("ListExpired after extend: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListExpired after extend = %+v, want [expired]", got)
	}
}
