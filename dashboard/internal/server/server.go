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

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	dashboardweb "github.com/actordock/actordock/dashboard/web"
)

type Server struct {
	cfg    Config
	logger *slog.Logger
	static fs.FS
}

func New(cfg Config, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}
	static, err := fs.Sub(dashboardweb.Dist, "dist")
	if err != nil {
		return nil, fmt.Errorf("load embedded SPA: %w", err)
	}
	return &Server{cfg: cfg, logger: logger, static: static}, nil
}

func (s *Server) Handler() (http.Handler, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)

	if s.cfg.ProxyPlatform {
		proxy, err := newPlatformProxy(s.cfg, s.logger)
		if err != nil {
			return nil, err
		}
		mux.Handle("/api/platform/", proxy)
	}

	mux.Handle("/", spaHandler(s.static))
	return mux, nil
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) Run(ctx context.Context) error {
	handler, err := s.Handler()
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              s.cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("dashboard listening", "addr", s.cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("dashboard serve: %w", err)
		}
		close(errCh)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
	case err := <-errCh:
		return err
	case sig := <-sigCh:
		s.logger.Info("shutdown signal received", "signal", sig.String())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("dashboard shutdown: %w", err)
	}
	return nil
}
