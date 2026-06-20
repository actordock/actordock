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
	"time"

	"github.com/actordock/actordock/internal/config"
	"github.com/actordock/actordock/internal/store"
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

func TestPaginateSnapshots(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	snapshots := []store.Snapshot{
		{SnapshotID: "a:default", CreatedAt: now},
		{SnapshotID: "b:default", CreatedAt: now.Add(time.Minute)},
		{SnapshotID: "c:default", CreatedAt: now.Add(2 * time.Minute)},
	}

	page, next, err := paginateSnapshots(snapshots, "", 2)
	if err != nil {
		t.Fatalf("paginateSnapshots: %v", err)
	}
	if len(page) != 2 || page[0].SnapshotID != "c:default" || page[1].SnapshotID != "b:default" {
		t.Fatalf("page = %+v", page)
	}
	if next == "" {
		t.Fatal("expected next token")
	}

	page2, next2, err := paginateSnapshots(snapshots, next, 2)
	if err != nil {
		t.Fatalf("paginateSnapshots page2: %v", err)
	}
	if len(page2) != 1 || page2[0].SnapshotID != "a:default" {
		t.Fatalf("page2 = %+v", page2)
	}
	if next2 != "" {
		t.Fatalf("next2 = %q, want empty", next2)
	}
}

func TestFilterSnapshotsBySandboxID(t *testing.T) {
	t.Parallel()
	snapshots := []store.Snapshot{
		{SnapshotID: "a", SandboxID: "sb-1"},
		{SnapshotID: "b", SandboxID: "sb-2"},
	}
	filtered := filterSnapshots(snapshots, "sb-1")
	if len(filtered) != 1 || filtered[0].SnapshotID != "a" {
		t.Fatalf("filtered = %+v", filtered)
	}
}
