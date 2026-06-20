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

func TestValidateOnTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value string
		ok    bool
	}{
		{OnTimeoutKill, true},
		{OnTimeoutPause, true},
		{"", false},
		{"destroy", false},
	}
	for _, tc := range tests {
		err := ValidateOnTimeout(tc.value)
		if tc.ok && err != nil {
			t.Fatalf("ValidateOnTimeout(%q) = %v, want nil", tc.value, err)
		}
		if !tc.ok && !errors.Is(err, ErrInvalidOnTimeout) {
			t.Fatalf("ValidateOnTimeout(%q) = %v, want ErrInvalidOnTimeout", tc.value, err)
		}
	}
}

func TestResolveOnTimeout(t *testing.T) {
	t.Parallel()

	got, err := ResolveOnTimeout("")
	if err != nil || got != OnTimeoutKill {
		t.Fatalf("ResolveOnTimeout(\"\") = %q, %v, want kill", got, err)
	}

	got, err = ResolveOnTimeout(OnTimeoutPause)
	if err != nil || got != OnTimeoutPause {
		t.Fatalf("ResolveOnTimeout(pause) = %q, %v", got, err)
	}

	if _, err := ResolveOnTimeout("invalid"); !errors.Is(err, ErrInvalidOnTimeout) {
		t.Fatalf("ResolveOnTimeout(invalid) = %v, want ErrInvalidOnTimeout", err)
	}
}

func TestRedisOnTimeoutRoundTrip(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	s, err := NewRedis(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	sb := Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		CreatedAt: now,
		ExpiresAt: now.Add(300 * time.Second),
		Status:    StatusRunning,
		OnTimeout: OnTimeoutPause,
	}

	if err := s.Put(ctx, sb); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := s.Get(ctx, "sb-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.OnTimeout != OnTimeoutPause {
		t.Fatalf("OnTimeout = %q, want %q", got.OnTimeout, OnTimeoutPause)
	}
}

func TestValidateAutoResume(t *testing.T) {
	t.Parallel()

	if err := ValidateAutoResume(OnTimeoutPause, true); err != nil {
		t.Fatalf("ValidateAutoResume(pause, true) = %v, want nil", err)
	}
	if err := ValidateAutoResume(OnTimeoutPause, false); err != nil {
		t.Fatalf("ValidateAutoResume(pause, false) = %v, want nil", err)
	}
	if err := ValidateAutoResume(OnTimeoutKill, false); err != nil {
		t.Fatalf("ValidateAutoResume(kill, false) = %v, want nil", err)
	}
	if err := ValidateAutoResume(OnTimeoutKill, true); !errors.Is(err, ErrInvalidAutoResume) {
		t.Fatalf("ValidateAutoResume(kill, true) = %v, want ErrInvalidAutoResume", err)
	}
}

func TestRedisAutoResumeRoundTrip(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	s, err := NewRedis(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	sb := Sandbox{
		SandboxID:  "sb-1",
		ActorID:    "sb-1",
		Template:   "base",
		CreatedAt:  now,
		ExpiresAt:  now.Add(300 * time.Second),
		Status:     StatusRunning,
		OnTimeout:  OnTimeoutPause,
		AutoResume: true,
	}

	if err := s.Put(ctx, sb); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := s.Get(ctx, "sb-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.AutoResume {
		t.Fatal("AutoResume = false, want true")
	}
}
