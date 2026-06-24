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
	"github.com/actordock/actordock/internal/redis"
	"github.com/actordock/actordock/internal/store"
	"github.com/actordock/actordock/internal/templatebuild"
	v1alpha1 "github.com/actordock/runtime/pkg/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.TemplateBuilderFromEnv()
	if err != nil {
		return err
	}
	logger := log.New(cfg.LogLevel)

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

	restConfig, err := ctrlconfig.GetConfig()
	if err != nil {
		return fmt.Errorf("kubernetes config: %w", err)
	}
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("add actordock scheme: %w", err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("add batch scheme: %w", err)
	}
	k8s, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("kubernetes client: %w", err)
	}

	builder := templatebuild.NewKanikoImageBuilder(cfg, k8s)
	worker := templatebuild.NewWorker(cfg, st, k8s, builder)
	logger.Info("template-builder listening for build jobs")
	return worker.Run(ctx)
}
