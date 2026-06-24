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

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/actordock/actordock/internal/config"
	"github.com/actordock/actordock/internal/log"
	"github.com/actordock/actordock/internal/platform"
	"github.com/actordock/actordock/internal/redis"
	"github.com/actordock/actordock/internal/store"
	"github.com/actordock/actordock/internal/runtimeapi"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.PlatformFromEnv()
	if err != nil {
		return err
	}
	logger := log.New(cfg.LogLevel)

	ate, err := runtimeapi.Dial(cfg.RuntimeAPIAddr)
	if err != nil {
		return err
	}
	defer ate.Close()

	waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer waitCancel()
	if err := redis.Wait(waitCtx, cfg.RedisAddr, logger); err != nil {
		return fmt.Errorf("wait for redis: %w", err)
	}

	st, err := store.NewRedis(cfg.RedisAddr)
	if err != nil {
		return err
	}
	defer st.Close()

	catalog, err := platform.NewTemplateCatalogFromCluster(cfg)
	if err != nil {
		logger.Warn("kubernetes template catalog unavailable; using static catalog", "err", err)
		catalog = platform.NewStaticTemplateCatalog(cfg)
	}

	return platform.NewServerWithCatalog(cfg, ate, st, catalog, logger).Run(ctx)
}
