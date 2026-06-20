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

var ErrSnapshotNotFound = errors.New("snapshot not found")

const snapshotKeyPrefix = "actordock:snapshot:"

// Snapshot is persisted sandbox snapshot metadata for E2B snapshot APIs.
type Snapshot struct {
	SnapshotID   string    `json:"snapshot_id"`
	Names        []string  `json:"names"`
	SandboxID    string    `json:"sandbox_id"`
	ActorID      string    `json:"actor_id"`
	SnapshotURI  string    `json:"snapshot_uri"`
	SnapshotType string    `json:"snapshot_type"`
	Name         string    `json:"name,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func snapshotKey(id string) string {
	return snapshotKeyPrefix + id
}

func (r *Redis) PutSnapshot(ctx context.Context, snap Snapshot) error {
	if snap.SnapshotID == "" {
		return fmt.Errorf("snapshot id is required")
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	if err := r.client.Set(ctx, snapshotKey(snap.SnapshotID), data, 0).Err(); err != nil {
		return fmt.Errorf("redis set snapshot %s: %w", snap.SnapshotID, err)
	}
	return nil
}

func (r *Redis) GetSnapshot(ctx context.Context, snapshotID string) (Snapshot, error) {
	if snapshotID == "" {
		return Snapshot{}, fmt.Errorf("snapshot id is required")
	}
	data, err := r.client.Get(ctx, snapshotKey(snapshotID)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return Snapshot{}, ErrSnapshotNotFound
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("redis get snapshot %s: %w", snapshotID, err)
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return Snapshot{}, fmt.Errorf("unmarshal snapshot %s: %w", snapshotID, err)
	}
	return snap, nil
}

func (r *Redis) ListSnapshots(ctx context.Context) ([]Snapshot, error) {
	var snapshots []Snapshot
	iter := r.client.Scan(ctx, 0, snapshotKeyPrefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		id := strings.TrimPrefix(iter.Val(), snapshotKeyPrefix)
		snap, err := r.GetSnapshot(ctx, id)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snap)
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("redis scan snapshots: %w", err)
	}
	return snapshots, nil
}
