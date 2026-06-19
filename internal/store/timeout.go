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
	"fmt"
	"time"
)

const (
	MinTimeoutSeconds     = 15
	MaxTimeoutSeconds     = 24 * 60 * 60
	DefaultTimeoutSeconds = 300
)

var ErrInvalidTimeout = errors.New("invalid timeout")

// ValidateTimeout checks E2B timeout bounds (seconds from now).
func ValidateTimeout(seconds int) error {
	if seconds < MinTimeoutSeconds {
		return fmt.Errorf("%w: must be at least %d seconds", ErrInvalidTimeout, MinTimeoutSeconds)
	}
	if seconds > MaxTimeoutSeconds {
		return fmt.Errorf("%w: must be at most %d seconds", ErrInvalidTimeout, MaxTimeoutSeconds)
	}
	return nil
}

// ResolveTimeout returns an explicit timeout or defaultSeconds when timeout is nil.
func ResolveTimeout(timeout *int, defaultSeconds int) (int, error) {
	if timeout == nil {
		if err := ValidateTimeout(defaultSeconds); err != nil {
			return 0, fmt.Errorf("default timeout: %w", err)
		}
		return defaultSeconds, nil
	}
	if err := ValidateTimeout(*timeout); err != nil {
		return 0, err
	}
	return *timeout, nil
}

// ExpiresAt computes absolute expiry from now and timeout seconds.
func ExpiresAt(now time.Time, timeoutSeconds int) time.Time {
	return now.Add(time.Duration(timeoutSeconds) * time.Second)
}
