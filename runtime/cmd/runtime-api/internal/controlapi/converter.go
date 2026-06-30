// Copyright 2026 Google LLC
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

package controlapi

import (
	"log/slog"

	"github.com/actordock/runtime/internal/proto/runtimeworkerpb"
	runtimev1alpha1 "github.com/actordock/runtime/pkg/api/v1alpha1"
)

// convert runtimev1alpha1.SnapshotScope to runtimeworkerpb.SnapshotScope
func toRuntimeWorkerSnapshotScope(in runtimev1alpha1.SnapshotScope) runtimeworkerpb.SnapshotScope {
	switch in {
	case runtimev1alpha1.SnapshotScopeFull:
		return runtimeworkerpb.SnapshotScope_SNAPSHOT_SCOPE_FULL
	case runtimev1alpha1.SnapshotScopeData:
		return runtimeworkerpb.SnapshotScope_SNAPSHOT_SCOPE_DATA
	default:
		slog.Warn("unknown SnapshotScope; falling back to Full", "scope", string(in))
		return runtimeworkerpb.SnapshotScope_SNAPSHOT_SCOPE_FULL
	}
}
