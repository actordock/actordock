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
	"time"
)

// IsExpired reports whether sb has a non-zero expires_at before now.
func IsExpired(sb Sandbox, now time.Time) bool {
	return !sb.ExpiresAt.IsZero() && sb.ExpiresAt.Before(now)
}

// ListExpired returns sandboxes whose expires_at is set and before now.
func (r *Redis) ListExpired(ctx context.Context, now time.Time) ([]Sandbox, error) {
	all, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	expired := make([]Sandbox, 0)
	for _, sb := range all {
		if IsExpired(sb, now) {
			expired = append(expired, sb)
		}
	}
	return expired, nil
}
