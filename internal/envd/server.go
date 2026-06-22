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

package envd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"github.com/actordock/actordock/internal/logs"
	"github.com/actordock/actordock/pkg/envd/filesystem/filesystemv1connect"
	"github.com/actordock/actordock/pkg/envd/process/processv1connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type Options struct {
	Addr   string
	Logger *slog.Logger
}

func Run(ctx context.Context, opts Options) error {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Addr == "" {
		opts.Addr = ":49983"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /init", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	logBuf := logs.NewBuffer(logs.DefaultMaxLines, logs.DefaultMaxBytes)
	metricsCollector := NewCollector(NewProcCgroupReader())
	mux.HandleFunc("GET /logs", logs.NewHandler(logBuf))
	mux.HandleFunc("GET /metrics", NewMetricsHandler(metricsCollector))
	mux.HandleFunc("GET /metrics/history", NewMetricsHistoryHandler(metricsCollector))

	registerFilesHandlers(mux)

	processPath, processHandler := processv1connect.NewProcessHandler(
		&processService{logger: opts.Logger, logs: logBuf},
		connect.WithCompressMinBytes(1024),
	)
	mux.Handle(processPath, processHandler)

	filesystemPath, filesystemHandler := filesystemv1connect.NewFilesystemHandler(
		filesystemService{},
		connect.WithCompressMinBytes(1024),
	)
	mux.Handle(filesystemPath, filesystemHandler)

	srv := &http.Server{
		Addr:              opts.Addr,
		Handler:           h2c.NewHandler(mux, &http2.Server{}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		opts.Logger.Info("envd listening", "addr", opts.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("envd serve: %w", err)
		}
		close(errCh)
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
	case <-stop:
	case err := <-errCh:
		if err != nil {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	opts.Logger.Info("envd shutting down")
	return srv.Shutdown(shutdownCtx)
}
