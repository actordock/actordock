// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/actordock/actordock/internal/metrics"
	"github.com/actordock/actordock/internal/runtime/gvisor"
	"github.com/actordock/actordock/internal/snapshotstore"
	"github.com/actordock/actordock/internal/types"
	"github.com/actordock/actordock/internal/workerresource"
	"github.com/actordock/actordock/internal/workerserver"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	addr := env("LISTEN_ADDR", ":8081")
	workerID := env("WORKER_ID", "worker-0")
	controlplaneURL := env("CONTROLPLANE_URL", "")
	advertise := env("ADVERTISE_URL", "http://127.0.0.1:8081")

	rt, err := gvisor.New(gvisor.Config{
		RunscPath: env("RUNSC_PATH", "runsc"),
		StateDir:  env("RUNSC_ROOT", "/var/lib/actordock/runsc"),
		BundleDir: env("BUNDLE_DIR", "/var/lib/actordock/bundles"),
		Rootfs:    env("ROOTFS", "/opt/actordock/rootfs"),
		Platform:  env("PLATFORM", "systrap"),
		Network:   env("NETWORK", "none"),
	})
	if err != nil {
		log.Error("gvisor runtime", "err", err)
		os.Exit(1)
	}

	// PVC survives pod restarts; wipe ephemeral runsc state so leftover locks
	// cannot wedge the next Boot (Substrate keeps per-actor dirs similarly isolated).
	for _, d := range []string{
		env("RUNSC_ROOT", "/var/lib/actordock/runsc"),
		env("BUNDLE_DIR", "/var/lib/actordock/bundles"),
	} {
		_ = os.RemoveAll(d)
		if err := os.MkdirAll(d, 0o755); err != nil {
			log.Error("reset runtime dir", "dir", d, "err", err)
			os.Exit(1)
		}
	}
	// Legacy shared-rootfs filestores from older workers.
	if matches, _ := filepath.Glob(filepath.Join(env("ROOTFS", "/opt/actordock/rootfs"), ".gvisor.filestore.*")); len(matches) > 0 {
		for _, m := range matches {
			_ = os.Remove(m)
		}
	}

	snaps, err := openSnapshotStore(log)
	if err != nil {
		log.Error("snapshot store", "err", err)
		os.Exit(1)
	}

	metricsHandler, err := metrics.InstallPrometheus()
	if err != nil {
		log.Error("metrics", "err", err)
		os.Exit(1)
	}
	m := metrics.MustNew(env("POLICY", "worker"))

	activity := workerresource.NewActivity()
	srv := workerserver.New(workerID, rt, snaps, log, m, metricsHandler, activity)
	httpSrv := &http.Server{Addr: addr, Handler: srv.Handler()}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if envBool("SIGNAL_RESOURCE_ENABLED", true) && controlplaneURL != "" {
		interval := envDuration("SIGNAL_RESOURCE_INTERVAL", 5*time.Second)
		go (&workerresource.Pusher{
			WorkerID:        workerID,
			ControlPlaneURL: strings.TrimRight(controlplaneURL, "/"),
			Interval:        interval,
			Activity:        activity,
			ListActive:      srv.ActiveSandboxIDs,
			Log:             log,
		}).Run(ctx)
		log.Info("resource signal plugin enabled", "interval", interval.String())
	}

	go func() {
		log.Info("worker listening", "addr", addr, "workerID", workerID)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	if controlplaneURL != "" {
		go registerLoop(log, controlplaneURL, types.Worker{
			ID:           workerID,
			Address:      advertise,
			MaxSlots:     1,
			Healthy:      true,
			RegisteredAt: time.Now().UTC(),
		})
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = httpSrv.Shutdown(shutdownCtx)
}

func openSnapshotStore(log *slog.Logger) (snapshotstore.Store, error) {
	endpoint := env("S3_ENDPOINT", "")
	if endpoint == "" {
		log.Warn("S3_ENDPOINT unset; Suspend upload disabled")
		return nil, nil
	}
	st, err := snapshotstore.NewS3(snapshotstore.S3Config{
		Endpoint:  endpoint,
		Region:    env("S3_REGION", "us-east-1"),
		Bucket:    env("S3_BUCKET", "actordock-snapshots"),
		AccessKey: env("S3_ACCESS_KEY", "rustfsadmin"),
		SecretKey: env("S3_SECRET_KEY", "rustfsadmin"),
		PathStyle: strings.EqualFold(env("S3_PATH_STYLE", "true"), "true"),
	})
	if err != nil {
		return nil, err
	}
	log.Info("snapshot store ready", "endpoint", endpoint, "bucket", env("S3_BUCKET", "actordock-snapshots"))
	return st, nil
}

func registerLoop(log *slog.Logger, base string, w types.Worker) {
	client := &http.Client{Timeout: 5 * time.Second}
	for {
		b, _ := json.Marshal(w)
		req, err := http.NewRequest(http.MethodPost, base+"/v1/workers/register", bytes.NewReader(b))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				log.Warn("register worker failed", "err", err)
			} else {
				_ = resp.Body.Close()
				if resp.StatusCode < 300 {
					log.Info("registered with controlplane", "workerID", w.ID)
				}
			}
		}
		time.Sleep(5 * time.Second)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envDuration(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func envBool(k string, def bool) bool {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
