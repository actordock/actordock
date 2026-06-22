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
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestUserAccessTokenRoundTrip(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	s, err := NewRedis(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	ctx := context.Background()
	rec := UserAccessTokenRecord{
		ID:        "tok-1",
		Name:      "dashboard",
		Token:     "adt_secret",
		CreatedAt: time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC),
	}
	if err := s.PutUserAccessToken(ctx, rec); err != nil {
		t.Fatalf("PutUserAccessToken: %v", err)
	}
	got, err := s.GetUserAccessToken(ctx, "tok-1")
	if err != nil || got.Name != "dashboard" {
		t.Fatalf("GetUserAccessToken = %+v, %v", got, err)
	}
	if err := s.DeleteUserAccessToken(ctx, "tok-1"); err != nil {
		t.Fatalf("DeleteUserAccessToken: %v", err)
	}
	if err := s.DeleteUserAccessToken(ctx, "tok-1"); !errors.Is(err, ErrUserAccessTokenNotFound) {
		t.Fatalf("second delete = %v", err)
	}
}
