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
	"errors"
	"testing"
	"time"
)

func TestValidateTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		seconds int
		ok      bool
	}{
		{0, false},
		{14, false},
		{15, true},
		{300, true},
		{MaxTimeoutSeconds, true},
		{MaxTimeoutSeconds + 1, false},
	}
	for _, tc := range tests {
		err := ValidateTimeout(tc.seconds)
		if tc.ok && err != nil {
			t.Fatalf("ValidateTimeout(%d) = %v, want nil", tc.seconds, err)
		}
		if !tc.ok && !errors.Is(err, ErrInvalidTimeout) {
			t.Fatalf("ValidateTimeout(%d) = %v, want ErrInvalidTimeout", tc.seconds, err)
		}
	}
}

func TestResolveTimeout(t *testing.T) {
	t.Parallel()

	got, err := ResolveTimeout(nil, DefaultTimeoutSeconds)
	if err != nil {
		t.Fatalf("ResolveTimeout(nil): %v", err)
	}
	if got != DefaultTimeoutSeconds {
		t.Fatalf("got %d, want %d", got, DefaultTimeoutSeconds)
	}

	timeout := 60
	got, err = ResolveTimeout(&timeout, DefaultTimeoutSeconds)
	if err != nil || got != 60 {
		t.Fatalf("ResolveTimeout(60) = %d, %v", got, err)
	}

	bad := 0
	if _, err := ResolveTimeout(&bad, DefaultTimeoutSeconds); !errors.Is(err, ErrInvalidTimeout) {
		t.Fatalf("ResolveTimeout(0) = %v, want ErrInvalidTimeout", err)
	}
}

func TestExpiresAt(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	got := ExpiresAt(now, 60)
	want := now.Add(60 * time.Second)
	if !got.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v", got, want)
	}
}

func TestValidateRefreshDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		seconds int
		ok      bool
	}{
		{0, false},
		{14, false},
		{15, true},
		{3600, true},
		{3601, false},
		{MaxTimeoutSeconds, false},
	}
	for _, tc := range tests {
		err := ValidateRefreshDuration(tc.seconds)
		if tc.ok && err != nil {
			t.Fatalf("ValidateRefreshDuration(%d) = %v, want nil", tc.seconds, err)
		}
		if !tc.ok && !errors.Is(err, ErrInvalidTimeout) {
			t.Fatalf("ValidateRefreshDuration(%d) = %v, want ErrInvalidTimeout", tc.seconds, err)
		}
	}
}

func TestResolveRefreshDuration(t *testing.T) {
	t.Parallel()

	got, err := ResolveRefreshDuration(nil, DefaultTimeoutSeconds)
	if err != nil {
		t.Fatalf("ResolveRefreshDuration(nil): %v", err)
	}
	if got != DefaultTimeoutSeconds {
		t.Fatalf("got %d, want %d", got, DefaultTimeoutSeconds)
	}

	duration := 120
	got, err = ResolveRefreshDuration(&duration, DefaultTimeoutSeconds)
	if err != nil || got != 120 {
		t.Fatalf("ResolveRefreshDuration(120) = %d, %v", got, err)
	}

	bad := 0
	if _, err := ResolveRefreshDuration(&bad, DefaultTimeoutSeconds); !errors.Is(err, ErrInvalidTimeout) {
		t.Fatalf("ResolveRefreshDuration(0) = %v, want ErrInvalidTimeout", err)
	}
}
