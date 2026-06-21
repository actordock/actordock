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
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

var (
	ErrVolumeNotFound   = errors.New("volume not found")
	ErrVolumeNameTaken  = errors.New("volume name already exists")
	ErrVolumeNameEmpty  = errors.New("volume name is required")
	ErrVolumeIDEmpty    = errors.New("volume id is required")
)

const (
	volumeKeyPrefix     = "actordock:volume:"
	volumeNameKeyPrefix = "actordock:volume-name:"
	volumeTokenBytes    = 32
)

// Volume is persisted E2B volume metadata for Platform volume APIs.
type Volume struct {
	VolumeID  string    `json:"volume_id"`
	Name      string    `json:"name"`
	Token     string    `json:"token"`
	HostPath  string    `json:"host_path"`
	CreatedAt time.Time `json:"created_at"`
}

func volumeKey(id string) string {
	return volumeKeyPrefix + id
}

func volumeNameKey(name string) string {
	return volumeNameKeyPrefix + name
}

// VolumeHostPath returns the on-worker directory for a volume under root.
func VolumeHostPath(root, volumeID string) string {
	return path.Join(root, volumeID) + "/"
}

// NewVolumeToken returns a cryptographically random URL-safe token.
func NewVolumeToken() (string, error) {
	buf := make([]byte, volumeTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate volume token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func validateVolume(vol Volume) error {
	if vol.VolumeID == "" {
		return ErrVolumeIDEmpty
	}
	if vol.Name == "" {
		return ErrVolumeNameEmpty
	}
	return nil
}

func (r *Redis) PutVolume(ctx context.Context, vol Volume) error {
	if err := validateVolume(vol); err != nil {
		return err
	}
	if vol.Token == "" {
		return fmt.Errorf("volume token is required")
	}
	if vol.HostPath == "" {
		return fmt.Errorf("volume host path is required")
	}
	if vol.CreatedAt.IsZero() {
		return fmt.Errorf("volume created_at is required")
	}

	nameKey := volumeNameKey(vol.Name)
	owner, err := r.client.Get(ctx, nameKey).Result()
	if err != nil && !errors.Is(err, goredis.Nil) {
		return fmt.Errorf("redis get volume name %q: %w", vol.Name, err)
	}
	if err == nil && owner != vol.VolumeID {
		return ErrVolumeNameTaken
	}

	data, err := json.Marshal(vol)
	if err != nil {
		return fmt.Errorf("marshal volume: %w", err)
	}

	pipe := r.client.TxPipeline()
	pipe.Set(ctx, nameKey, vol.VolumeID, 0)
	pipe.Set(ctx, volumeKey(vol.VolumeID), data, 0)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis put volume %s: %w", vol.VolumeID, err)
	}
	return nil
}

func (r *Redis) GetVolume(ctx context.Context, volumeID string) (Volume, error) {
	if volumeID == "" {
		return Volume{}, ErrVolumeIDEmpty
	}
	data, err := r.client.Get(ctx, volumeKey(volumeID)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return Volume{}, ErrVolumeNotFound
	}
	if err != nil {
		return Volume{}, fmt.Errorf("redis get volume %s: %w", volumeID, err)
	}
	var vol Volume
	if err := json.Unmarshal(data, &vol); err != nil {
		return Volume{}, fmt.Errorf("unmarshal volume %s: %w", volumeID, err)
	}
	return vol, nil
}

func (r *Redis) GetVolumeByName(ctx context.Context, name string) (Volume, error) {
	if name == "" {
		return Volume{}, ErrVolumeNameEmpty
	}
	volumeID, err := r.client.Get(ctx, volumeNameKey(name)).Result()
	if errors.Is(err, goredis.Nil) {
		return Volume{}, ErrVolumeNotFound
	}
	if err != nil {
		return Volume{}, fmt.Errorf("redis get volume name %q: %w", name, err)
	}
	return r.GetVolume(ctx, volumeID)
}

func (r *Redis) ListVolumes(ctx context.Context) ([]Volume, error) {
	var volumes []Volume
	iter := r.client.Scan(ctx, 0, volumeKeyPrefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		id := strings.TrimPrefix(iter.Val(), volumeKeyPrefix)
		if id == "" {
			continue
		}
		vol, err := r.GetVolume(ctx, id)
		if err != nil {
			return nil, err
		}
		volumes = append(volumes, vol)
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("redis scan volumes: %w", err)
	}
	return volumes, nil
}

func (r *Redis) DeleteVolume(ctx context.Context, volumeID string) error {
	if volumeID == "" {
		return ErrVolumeIDEmpty
	}
	vol, err := r.GetVolume(ctx, volumeID)
	if errors.Is(err, ErrVolumeNotFound) {
		return ErrVolumeNotFound
	}
	if err != nil {
		return err
	}

	pipe := r.client.TxPipeline()
	pipe.Del(ctx, volumeKey(volumeID))
	pipe.Del(ctx, volumeNameKey(vol.Name))
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis delete volume %s: %w", volumeID, err)
	}
	return nil
}
