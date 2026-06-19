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

package substrate

import (
	"testing"

	"github.com/agent-substrate/substrate/pkg/proto/ateapipb"
)

func TestActorStateE2B(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status ateapipb.Actor_Status
		want   string
	}{
		{ateapipb.Actor_STATUS_RUNNING, "running"},
		{ateapipb.Actor_STATUS_RESUMING, "running"},
		{ateapipb.Actor_STATUS_SUSPENDED, "paused"},
		{ateapipb.Actor_STATUS_PAUSED, "paused"},
	}
	for _, tc := range tests {
		if got := ActorStateE2B(tc.status); got != tc.want {
			t.Fatalf("ActorStateE2B(%v) = %q, want %q", tc.status, got, tc.want)
		}
	}
}
