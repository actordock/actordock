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
	"strings"
	"testing"

	"github.com/actordock/actordock/internal/config"
)

func TestBuildSnapshotIdentity(t *testing.T) {
	t.Parallel()
	cfg := config.Platform{ClientID: "actordock"}

	id, names := buildSnapshotIdentity(cfg, "")
	if id == "" || !strings.HasSuffix(id, ":default") {
		t.Fatalf("id = %q", id)
	}
	if len(names) != 1 || names[0] != id {
		t.Fatalf("names = %v", names)
	}

	id, names = buildSnapshotIdentity(cfg, "my-snap")
	want := "actordock/my-snap:default"
	if id != want || len(names) != 1 || names[0] != want {
		t.Fatalf("id = %q names = %v, want %q", id, names, want)
	}
}
