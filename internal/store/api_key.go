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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

var (
	ErrTeamAPIKeyNotFound = errors.New("team api key not found")
	ErrTeamAPIKeyName     = errors.New("team api key name is required")
)

const (
	teamAPIKeyKeyPrefix     = "actordock:team-api-key:"
	teamAPIKeyHashKeyPrefix = "actordock:team-api-key-hash:"
)

// TeamAPIKeyRecord is persisted metadata for a created team API key (hash only).
type TeamAPIKeyRecord struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	KeyHash   string    `json:"key_hash"`
	CreatedAt time.Time `json:"created_at"`
}

func teamAPIKeyKey(id string) string {
	return teamAPIKeyKeyPrefix + id
}

func teamAPIKeyHashKey(hash string) string {
	return teamAPIKeyHashKeyPrefix + hash
}

// HashAPIKey returns a stable SHA-256 hex digest of a raw API key.
func HashAPIKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// NewTeamAPIKeyValue returns a new random API key value.
func NewTeamAPIKeyValue() (string, error) {
	tok, err := NewVolumeToken()
	if err != nil {
		return "", err
	}
	return "adk_" + tok, nil
}

func (r *Redis) PutTeamAPIKey(ctx context.Context, rec TeamAPIKeyRecord) error {
	if strings.TrimSpace(rec.ID) == "" {
		return fmt.Errorf("team api key id is required")
	}
	if strings.TrimSpace(rec.Name) == "" {
		return ErrTeamAPIKeyName
	}
	if rec.KeyHash == "" {
		return fmt.Errorf("team api key hash is required")
	}
	if rec.CreatedAt.IsZero() {
		return fmt.Errorf("team api key created_at is required")
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal team api key: %w", err)
	}
	pipe := r.client.TxPipeline()
	pipe.Set(ctx, teamAPIKeyKey(rec.ID), data, 0)
	pipe.Set(ctx, teamAPIKeyHashKey(rec.KeyHash), rec.ID, 0)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis put team api key %s: %w", rec.ID, err)
	}
	return nil
}

func (r *Redis) ListTeamAPIKeys(ctx context.Context) ([]TeamAPIKeyRecord, error) {
	var out []TeamAPIKeyRecord
	iter := r.client.Scan(ctx, 0, teamAPIKeyKeyPrefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		id := strings.TrimPrefix(iter.Val(), teamAPIKeyKeyPrefix)
		if id == "" {
			continue
		}
		rec, err := r.GetTeamAPIKey(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("redis scan team api keys: %w", err)
	}
	return out, nil
}

func (r *Redis) GetTeamAPIKey(ctx context.Context, id string) (TeamAPIKeyRecord, error) {
	if strings.TrimSpace(id) == "" {
		return TeamAPIKeyRecord{}, fmt.Errorf("team api key id is required")
	}
	data, err := r.client.Get(ctx, teamAPIKeyKey(id)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return TeamAPIKeyRecord{}, ErrTeamAPIKeyNotFound
	}
	if err != nil {
		return TeamAPIKeyRecord{}, fmt.Errorf("redis get team api key %s: %w", id, err)
	}
	var rec TeamAPIKeyRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return TeamAPIKeyRecord{}, fmt.Errorf("unmarshal team api key %s: %w", id, err)
	}
	return rec, nil
}

func (r *Redis) ValidateTeamAPIKey(ctx context.Context, raw string) (bool, error) {
	if strings.TrimSpace(raw) == "" {
		return false, nil
	}
	hash := HashAPIKey(raw)
	_, err := r.client.Get(ctx, teamAPIKeyHashKey(hash)).Result()
	if errors.Is(err, goredis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("redis validate team api key: %w", err)
	}
	return true, nil
}
