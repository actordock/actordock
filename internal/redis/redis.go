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

package redis

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const defaultRetryInterval = time.Second

// Wait blocks until Redis at addr responds to PING or ctx is canceled.
func Wait(ctx context.Context, addr string, logger *slog.Logger) error {
	if addr == "" {
		return fmt.Errorf("redis address is required")
	}
	if logger == nil {
		logger = slog.Default()
	}

	client := goredis.NewClient(&goredis.Options{Addr: addr})
	defer client.Close()

	ticker := time.NewTicker(defaultRetryInterval)
	defer ticker.Stop()

	for {
		if err := client.Ping(ctx).Err(); err == nil {
			logger.Info("redis ready", "addr", addr)
			return nil
		} else if ctx.Err() != nil {
			return fmt.Errorf("redis %s: %w", addr, ctx.Err())
		} else {
			logger.Debug("waiting for redis", "addr", addr, "err", err)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("redis %s: %w", addr, ctx.Err())
		case <-ticker.C:
		}
	}
}
