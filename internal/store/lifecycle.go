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
)

const (
	OnTimeoutKill  = "kill"
	OnTimeoutPause = "pause"
)

var ErrInvalidOnTimeout = errors.New("invalid on_timeout")

// ValidateOnTimeout checks E2B lifecycle on_timeout values.
func ValidateOnTimeout(onTimeout string) error {
	switch onTimeout {
	case OnTimeoutKill, OnTimeoutPause:
		return nil
	default:
		return fmt.Errorf("%w: must be %q or %q", ErrInvalidOnTimeout, OnTimeoutKill, OnTimeoutPause)
	}
}

// ResolveOnTimeout returns onTimeout or OnTimeoutKill when empty (E2B default).
func ResolveOnTimeout(onTimeout string) (string, error) {
	if onTimeout == "" {
		return OnTimeoutKill, nil
	}
	if err := ValidateOnTimeout(onTimeout); err != nil {
		return "", err
	}
	return onTimeout, nil
}
