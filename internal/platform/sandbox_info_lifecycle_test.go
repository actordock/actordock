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

	"github.com/actordock/actordock/internal/store"
	"github.com/agent-substrate/substrate/pkg/proto/ateapipb"
)

func TestBuildSandboxLifecycle(t *testing.T) {
	t.Parallel()

	got := buildSandboxLifecycle(store.Sandbox{
		OnTimeout:  store.OnTimeoutPause,
		AutoResume: true,
	})
	if got.OnTimeout != store.OnTimeoutPause || !got.AutoResume {
		t.Fatalf("lifecycle = %+v", got)
	}

	got = buildSandboxLifecycle(store.Sandbox{})
	if got.OnTimeout != store.OnTimeoutKill || got.AutoResume {
		t.Fatalf("default lifecycle = %+v", got)
	}
}

func TestStoreStatusFromActorPaused(t *testing.T) {
	t.Parallel()

	if got := storeStatusFromActor(ateapipb.Actor_STATUS_SUSPENDED); got != store.StatusPaused {
		t.Fatalf("status = %q, want paused", got)
	}
	if got := storeStatusFromActor(ateapipb.Actor_STATUS_PAUSED); got != store.StatusPaused {
		t.Fatalf("status = %q, want paused", got)
	}
}
