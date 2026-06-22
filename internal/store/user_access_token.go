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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

var (
	ErrUserAccessTokenNotFound = errors.New("user access token not found")
	ErrUserAccessTokenName     = errors.New("access token name is required")
)

const userAccessTokenKeyPrefix = "actordock:user-access-token:"

// UserAccessTokenRecord is a dashboard/CLI access token (separate from sandbox envd tokens).
type UserAccessTokenRecord struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
}

func userAccessTokenKey(id string) string {
	return userAccessTokenKeyPrefix + id
}

// NewUserAccessTokenValue returns a new random access token value.
func NewUserAccessTokenValue() (string, error) {
	tok, err := NewVolumeToken()
	if err != nil {
		return "", err
	}
	return "adt_" + tok, nil
}

func (r *Redis) PutUserAccessToken(ctx context.Context, rec UserAccessTokenRecord) error {
	if strings.TrimSpace(rec.ID) == "" {
		return fmt.Errorf("access token id is required")
	}
	if strings.TrimSpace(rec.Name) == "" {
		return ErrUserAccessTokenName
	}
	if strings.TrimSpace(rec.Token) == "" {
		return fmt.Errorf("access token value is required")
	}
	if rec.CreatedAt.IsZero() {
		return fmt.Errorf("access token created_at is required")
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal user access token: %w", err)
	}
	if err := r.client.Set(ctx, userAccessTokenKey(rec.ID), data, 0).Err(); err != nil {
		return fmt.Errorf("redis put user access token %s: %w", rec.ID, err)
	}
	return nil
}

func (r *Redis) GetUserAccessToken(ctx context.Context, id string) (UserAccessTokenRecord, error) {
	if strings.TrimSpace(id) == "" {
		return UserAccessTokenRecord{}, fmt.Errorf("access token id is required")
	}
	data, err := r.client.Get(ctx, userAccessTokenKey(id)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return UserAccessTokenRecord{}, ErrUserAccessTokenNotFound
	}
	if err != nil {
		return UserAccessTokenRecord{}, fmt.Errorf("redis get user access token %s: %w", id, err)
	}
	var rec UserAccessTokenRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return UserAccessTokenRecord{}, fmt.Errorf("unmarshal user access token %s: %w", id, err)
	}
	return rec, nil
}

func (r *Redis) DeleteUserAccessToken(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("access token id is required")
	}
	deleted, err := r.client.Del(ctx, userAccessTokenKey(id)).Result()
	if err != nil {
		return fmt.Errorf("redis delete user access token %s: %w", id, err)
	}
	if deleted == 0 {
		return ErrUserAccessTokenNotFound
	}
	return nil
}
