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

package scheduler

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/actordock/actordock/internal/config"
	"github.com/actordock/actordock/internal/store"
)

type expiredLister interface {
	ListExpired(ctx context.Context, now time.Time) ([]store.Sandbox, error)
}

type sandboxExpirer interface {
	ExpireSandbox(ctx context.Context, sandboxID string) error
}

// Runner polls Redis for expired sandboxes and enforces TTL.
type Runner struct {
	cfg     config.Scheduler
	store   expiredLister
	expirer sandboxExpirer
	logger  *slog.Logger
	nowFunc func() time.Time
}

func NewRunner(cfg config.Scheduler, st expiredLister, expirer sandboxExpirer, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{
		cfg:     cfg,
		store:   st,
		expirer: expirer,
		logger:  logger,
		nowFunc: time.Now,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	r.logger.Info("scheduler started", "poll_interval", r.cfg.PollInterval)

	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)

	if err := r.tick(ctx); err != nil {
		r.logger.Error("scheduler tick", "err", err)
	}

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("scheduler shutting down")
			return ctx.Err()
		case <-stop:
			r.logger.Info("scheduler shutting down")
			return nil
		case <-ticker.C:
			if err := r.tick(ctx); err != nil {
				r.logger.Error("scheduler tick", "err", err)
			}
		}
	}
}

func (r *Runner) tick(ctx context.Context) error {
	now := r.nowFunc()
	expired, err := r.store.ListExpired(ctx, now)
	if err != nil {
		return err
	}
	for _, sb := range expired {
		if err := r.expirer.ExpireSandbox(ctx, sb.SandboxID); err != nil {
			r.logger.Error("expire sandbox", "sandbox_id", sb.SandboxID, "err", err)
			continue
		}
		r.logger.Info("expired sandbox", "sandbox_id", sb.SandboxID)
	}
	return nil
}

var _ expiredLister = (*store.Redis)(nil)
