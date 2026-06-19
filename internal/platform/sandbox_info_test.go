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

package platform

import (
	"testing"
	"time"

	"github.com/actordock/actordock/internal/config"
	"github.com/actordock/actordock/internal/store"
)

func TestSandboxEndAtFromExpiresAt(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	expiresAt := createdAt.Add(60 * time.Second)
	cfg := config.Platform{DefaultSandboxTimeout: 300}

	got := sandboxEndAt(cfg, store.Sandbox{
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
	})
	if got != expiresAt.Format(time.RFC3339) {
		t.Fatalf("endAt = %q, want %q", got, expiresAt.Format(time.RFC3339))
	}
}

func TestSandboxEndAtFallbackWhenZero(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	cfg := config.Platform{DefaultSandboxTimeout: 300}

	got := sandboxEndAt(cfg, store.Sandbox{CreatedAt: createdAt})
	want := createdAt.Add(300 * time.Second).Format(time.RFC3339)
	if got != want {
		t.Fatalf("endAt = %q, want %q", got, want)
	}
}
