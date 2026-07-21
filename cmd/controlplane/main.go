// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/actordock/actordock/internal/controlplane"
	"github.com/actordock/actordock/internal/policy"
	"github.com/actordock/actordock/internal/scheduler"
	"github.com/actordock/actordock/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	addr := env("LISTEN_ADDR", ":8080")
	redisAddr := env("REDIS_ADDR", "127.0.0.1:6379")
	policyName := env("POLICY", "fifo")
	snapRoot := env("SNAPSHOT_ROOT", "/var/lib/actordock/snapshots")

	pol, err := policy.New(policyName)
	if err != nil {
		log.Error("policy", "err", err)
		os.Exit(1)
	}

	rdb := store.NewRedis(redisAddr)
	if err := waitRedis(rdb, log); err != nil {
		log.Error("redis ping", "addr", redisAddr, "err", err)
		os.Exit(1)
	}

	sched := scheduler.New(rdb, pol, snapRoot, log)
	srv := controlplane.New(sched, rdb, log)

	httpSrv := &http.Server{Addr: addr, Handler: srv.Handler()}
	go func() {
		log.Info("controlplane listening", "addr", addr, "policy", pol.Name())
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	// Build golden once workers register (Substrate template warm path).
	go func() {
		deadline := time.Now().Add(3 * time.Minute)
		for time.Now().Before(deadline) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			_, err := sched.EnsureGolden(ctx)
			cancel()
			if err == nil {
				return
			}
			log.Warn("ensure golden not ready yet", "err", err)
			time.Sleep(5 * time.Second)
		}
		log.Error("ensure golden timed out")
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	fmt.Fprintln(os.Stderr, "controlplane stopped")
}

func waitRedis(rdb *store.Redis, log *slog.Logger) error {
	deadline := time.Now().Add(60 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		last = rdb.Ping(ctx)
		cancel()
		if last == nil {
			return nil
		}
		log.Warn("redis not ready, retrying", "err", last)
		time.Sleep(2 * time.Second)
	}
	return last
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
