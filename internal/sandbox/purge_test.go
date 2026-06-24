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

package sandbox

import (
	"context"
	"errors"
	"testing"

	"github.com/actordock/actordock/internal/runtimeapi"
	"github.com/actordock/actordock/internal/store"
)

type fakeActorDeleter struct {
	deleted []string
	err     error
}

func (f *fakeActorDeleter) DeleteSandbox(_ context.Context, actorID string) error {
	if f.err != nil {
		return f.err
	}
	f.deleted = append(f.deleted, actorID)
	return nil
}

type fakeMetadataStore struct {
	deleted []string
	err     error
}

func (f *fakeMetadataStore) Delete(_ context.Context, sandboxID string) error {
	if f.err != nil {
		return f.err
	}
	f.deleted = append(f.deleted, sandboxID)
	return nil
}

func TestPurge(t *testing.T) {
	t.Parallel()

	actors := &fakeActorDeleter{}
	st := &fakeMetadataStore{}
	if err := Purge(context.Background(), actors, st, "sb-1"); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if len(actors.deleted) != 1 || actors.deleted[0] != "sb-1" {
		t.Fatalf("deleted actors = %v", actors.deleted)
	}
	if len(st.deleted) != 1 || st.deleted[0] != "sb-1" {
		t.Fatalf("deleted metadata = %v", st.deleted)
	}
}

func TestPurgeActorMissing(t *testing.T) {
	t.Parallel()

	actors := &fakeActorDeleter{err: runtimeapi.ErrNotFound}
	st := &fakeMetadataStore{}
	if err := Purge(context.Background(), actors, st, "sb-1"); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if len(st.deleted) != 1 {
		t.Fatalf("deleted metadata = %v, want sb-1", st.deleted)
	}
}

func TestPurgeActorError(t *testing.T) {
	t.Parallel()

	actors := &fakeActorDeleter{err: errors.New("boom")}
	st := &fakeMetadataStore{}
	if err := Purge(context.Background(), actors, st, "sb-1"); err == nil {
		t.Fatal("Purge = nil, want error")
	}
	if len(st.deleted) != 0 {
		t.Fatalf("deleted metadata = %v, want none", st.deleted)
	}
}

func TestPurgeMetadataMissing(t *testing.T) {
	t.Parallel()

	actors := &fakeActorDeleter{}
	st := &fakeMetadataStore{err: store.ErrNotFound}
	if err := Purge(context.Background(), actors, st, "sb-1"); err != nil {
		t.Fatalf("Purge: %v", err)
	}
}
