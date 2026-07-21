// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/actordock/actordock/internal/types"
	"github.com/redis/go-redis/v9"
)

const (
	keySandboxPrefix = "actordock:sandbox:"
	keySandboxes     = "actordock:sandboxes"
	keyWorkerPrefix  = "actordock:worker:"
	keyWorkers       = "actordock:workers"
	keyGolden        = "actordock:golden:default"
)

// Redis implements Store with Redis/Valkey.
type Redis struct {
	rdb *redis.Client
}

func NewRedis(addr string) *Redis {
	return &Redis{rdb: redis.NewClient(&redis.Options{Addr: addr})}
}

func NewRedisFromClient(rdb *redis.Client) *Redis {
	return &Redis{rdb: rdb}
}

func (r *Redis) Ping(ctx context.Context) error {
	return r.rdb.Ping(ctx).Err()
}

func (r *Redis) PutSandbox(ctx context.Context, sb types.Sandbox) error {
	b, err := json.Marshal(sb)
	if err != nil {
		return err
	}
	pipe := r.rdb.TxPipeline()
	pipe.Set(ctx, keySandboxPrefix+sb.ID, b, 0)
	pipe.SAdd(ctx, keySandboxes, sb.ID)
	_, err = pipe.Exec(ctx)
	return err
}

func (r *Redis) GetSandbox(ctx context.Context, id string) (types.Sandbox, error) {
	b, err := r.rdb.Get(ctx, keySandboxPrefix+id).Bytes()
	if err == redis.Nil {
		return types.Sandbox{}, fmt.Errorf("sandbox %q not found", id)
	}
	if err != nil {
		return types.Sandbox{}, err
	}
	var sb types.Sandbox
	if err := json.Unmarshal(b, &sb); err != nil {
		return types.Sandbox{}, err
	}
	return sb, nil
}

func (r *Redis) DeleteSandbox(ctx context.Context, id string) error {
	pipe := r.rdb.TxPipeline()
	pipe.Del(ctx, keySandboxPrefix+id)
	pipe.SRem(ctx, keySandboxes, id)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *Redis) ListSandboxes(ctx context.Context) ([]types.Sandbox, error) {
	ids, err := r.rdb.SMembers(ctx, keySandboxes).Result()
	if err != nil {
		return nil, err
	}
	out := make([]types.Sandbox, 0, len(ids))
	for _, id := range ids {
		sb, err := r.GetSandbox(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, sb)
	}
	return out, nil
}

func (r *Redis) PutWorker(ctx context.Context, w types.Worker) error {
	b, err := json.Marshal(w)
	if err != nil {
		return err
	}
	pipe := r.rdb.TxPipeline()
	pipe.Set(ctx, keyWorkerPrefix+w.ID, b, 0)
	pipe.SAdd(ctx, keyWorkers, w.ID)
	_, err = pipe.Exec(ctx)
	return err
}

func (r *Redis) GetWorker(ctx context.Context, id string) (types.Worker, error) {
	b, err := r.rdb.Get(ctx, keyWorkerPrefix+id).Bytes()
	if err == redis.Nil {
		return types.Worker{}, fmt.Errorf("worker %q not found", id)
	}
	if err != nil {
		return types.Worker{}, err
	}
	var w types.Worker
	if err := json.Unmarshal(b, &w); err != nil {
		return types.Worker{}, err
	}
	return w, nil
}

func (r *Redis) DeleteWorker(ctx context.Context, id string) error {
	pipe := r.rdb.TxPipeline()
	pipe.Del(ctx, keyWorkerPrefix+id)
	pipe.SRem(ctx, keyWorkers, id)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *Redis) ListWorkers(ctx context.Context) ([]types.Worker, error) {
	ids, err := r.rdb.SMembers(ctx, keyWorkers).Result()
	if err != nil {
		return nil, err
	}
	out := make([]types.Worker, 0, len(ids))
	for _, id := range ids {
		w, err := r.GetWorker(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, w)
	}
	return out, nil
}

func (r *Redis) PutGolden(ctx context.Context, objectPrefix string) error {
	return r.rdb.Set(ctx, keyGolden, objectPrefix, 0).Err()
}

func (r *Redis) GetGolden(ctx context.Context) (string, error) {
	v, err := r.rdb.Get(ctx, keyGolden).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("golden snapshot not ready")
	}
	return v, err
}
