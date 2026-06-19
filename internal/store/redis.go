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

	goredis "github.com/redis/go-redis/v9"
)

var ErrNotFound = errors.New("sandbox not found")

const sandboxKeyPrefix = "actordock:sandbox:"

// Redis persists sandbox metadata in Redis.
type Redis struct {
	client *goredis.Client
}

func NewRedis(addr string) (*Redis, error) {
	if addr == "" {
		return nil, fmt.Errorf("redis address is required")
	}
	return &Redis{
		client: goredis.NewClient(&goredis.Options{Addr: addr}),
	}, nil
}

func (r *Redis) Close() error {
	return r.client.Close()
}

func sandboxKey(id string) string {
	return sandboxKeyPrefix + id
}

func (r *Redis) Put(ctx context.Context, sb Sandbox) error {
	if sb.SandboxID == "" {
		return fmt.Errorf("sandbox id is required")
	}
	data, err := json.Marshal(sb)
	if err != nil {
		return fmt.Errorf("marshal sandbox: %w", err)
	}
	if err := r.client.Set(ctx, sandboxKey(sb.SandboxID), data, 0).Err(); err != nil {
		return fmt.Errorf("redis set sandbox %s: %w", sb.SandboxID, err)
	}
	return nil
}

func (r *Redis) Get(ctx context.Context, sandboxID string) (Sandbox, error) {
	if sandboxID == "" {
		return Sandbox{}, fmt.Errorf("sandbox id is required")
	}
	data, err := r.client.Get(ctx, sandboxKey(sandboxID)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return Sandbox{}, ErrNotFound
	}
	if err != nil {
		return Sandbox{}, fmt.Errorf("redis get sandbox %s: %w", sandboxID, err)
	}
	var sb Sandbox
	if err := json.Unmarshal(data, &sb); err != nil {
		return Sandbox{}, fmt.Errorf("unmarshal sandbox %s: %w", sandboxID, err)
	}
	return sb, nil
}

func (r *Redis) Delete(ctx context.Context, sandboxID string) error {
	if sandboxID == "" {
		return fmt.Errorf("sandbox id is required")
	}
	n, err := r.client.Del(ctx, sandboxKey(sandboxID)).Result()
	if err != nil {
		return fmt.Errorf("redis delete sandbox %s: %w", sandboxID, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
