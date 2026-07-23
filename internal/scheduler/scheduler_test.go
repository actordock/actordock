// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package scheduler

import (
	"errors"
	"fmt"
	"testing"

	"github.com/actordock/actordock/internal/policy"
)

func TestResumeRetryable(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("boom"), false},
		{policy.ErrAllSemanticLocked, true},
		{fmt.Errorf("wrap: %w", policy.ErrAllSemanticLocked), true},
		{policy.ErrNotBestWaiter, true},
		{fmt.Errorf("wrap: %w", policy.ErrNotBestWaiter), true},
		{fmt.Errorf("fifo: no capacity and nothing to suspend"), true},
		{fmt.Errorf("semantic-score: all running sandboxes busy (checkpoint)"), true},
		{fmt.Errorf("evict x: checkpoint failed"), false},
	}
	for _, tc := range cases {
		got := resumeRetryable(tc.err)
		if got != tc.want {
			t.Fatalf("resumeRetryable(%v)=%v want %v", tc.err, got, tc.want)
		}
	}
}
