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
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestTeamAPIKeyRoundTrip(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	s, err := NewRedis(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	ctx := context.Background()
	raw := "adk_test_key_value_123"
	rec := TeamAPIKeyRecord{
		ID:        "key-1",
		Name:      "ci",
		KeyHash:   HashAPIKey(raw),
		CreatedAt: time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC),
	}
	if err := s.PutTeamAPIKey(ctx, rec); err != nil {
		t.Fatalf("PutTeamAPIKey: %v", err)
	}
	ok, err := s.ValidateTeamAPIKey(ctx, raw)
	if err != nil || !ok {
		t.Fatalf("ValidateTeamAPIKey = %v, %v", ok, err)
	}
	list, err := s.ListTeamAPIKeys(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListTeamAPIKeys = %+v, %v", list, err)
	}
}
