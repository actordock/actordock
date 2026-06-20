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

package router

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/actordock/actordock/internal/config"
	"github.com/actordock/actordock/internal/envd"
	"github.com/actordock/actordock/internal/substrate"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

const sandboxIDHeader = "E2b-Sandbox-Id"

type sandboxBackendResolver interface {
	ResumeSandboxBackend(ctx context.Context, actorID string, envdPort int) (backend string, waitEnvd bool, err error)
}

type Server struct {
	cfg           config.Router
	actors        sandboxBackendResolver
	logger        *slog.Logger
	envdTransport http.RoundTripper
}

func NewServer(cfg config.Router, actors sandboxBackendResolver, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		cfg:           cfg,
		actors:        actors,
		logger:        logger,
		envdTransport: newEnvdTransport(),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealthOrProxy)
	mux.HandleFunc("/", s.handleProxy)
	return mux
}

func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.cfg.ListenAddr,
		Handler:           h2cHandler(s.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("router listening", "addr", s.cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("router serve: %w", err)
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
	s.logger.Info("router shutting down")
	return srv.Shutdown(shutdownCtx)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleHealthOrProxy(w http.ResponseWriter, r *http.Request) {
	if _, err := parseSandboxID(r, s.cfg.Domain, s.cfg.EnvdPort); err == nil {
		s.handleProxy(w, r)
		return
	}
	s.handleHealth(w, r)
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	// Proxies all envd traffic including Connect RPC (e.g. /process.Process/Connect).
	sandboxID, err := parseSandboxID(r, s.cfg.Domain, s.cfg.EnvdPort)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "sandbox id not found in request")
		return
	}

	backend, waitEnvd, err := s.actors.ResumeSandboxBackend(r.Context(), sandboxID, s.cfg.EnvdPort)
	if err != nil {
		if errors.Is(err, substrate.ErrNotFound) {
			writeAPIError(w, http.StatusNotFound, "sandbox not found")
			return
		}
		s.logger.Error("resolve sandbox backend", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusBadGateway, "failed to reach sandbox")
		return
	}
	if waitEnvd {
		if err := envd.WaitForReady(r.Context(), "http://"+backend, envd.DefaultReadyTimeout); err != nil {
			s.logger.Error("wait for envd", "sandbox_id", sandboxID, "backend", backend, "err", err)
			writeAPIError(w, http.StatusBadGateway, "failed to reach sandbox")
			return
		}
	}

	target, err := url.Parse("http://" + backend)
	if err != nil {
		s.logger.Error("parse backend url", "backend", backend, "err", err)
		writeAPIError(w, http.StatusBadGateway, "failed to reach sandbox")
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = s.envdTransport
	proxy.FlushInterval = 100 * time.Millisecond
	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		s.logger.Error("proxy to envd", "sandbox_id", sandboxID, "backend", backend, "err", err)
		writeAPIError(w, http.StatusBadGateway, "failed to reach sandbox")
	}
	proxy.ServeHTTP(w, r)
}

func parseSandboxID(r *http.Request, domain string, envdPort int) (string, error) {
	if id := strings.TrimSpace(r.Header.Get(sandboxIDHeader)); id != "" {
		return id, nil
	}

	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	if domain == "" {
		return "", errors.New("missing domain")
	}

	suffix := "." + domain
	if !strings.HasSuffix(host, suffix) {
		return "", errors.New("host does not match domain")
	}

	prefix := strings.TrimSuffix(host, suffix)
	portPrefix := fmt.Sprintf("%d-", envdPort)
	if strings.HasPrefix(prefix, portPrefix) {
		prefix = strings.TrimPrefix(prefix, portPrefix)
	}
	if prefix == "" {
		return "", errors.New("missing sandbox id in host")
	}
	return prefix, nil
}

type apiError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiError{
		Message: message,
		Code:    status,
	})
}

func newEnvdTransport() http.RoundTripper {
	h1 := http.DefaultTransport
	h2 := &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	}
	return envdRoundTripper{h1: h1, h2: h2}
}

type envdRoundTripper struct {
	h1 http.RoundTripper
	h2 http.RoundTripper
}

func (t envdRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.ProtoMajor == 2 || strings.HasPrefix(req.Header.Get("Content-Type"), "application/connect") {
		return t.h2.RoundTrip(req)
	}
	return t.h1.RoundTrip(req)
}

func h2cHandler(next http.Handler) http.Handler {
	return h2c.NewHandler(next, &http2.Server{})
}

// Ensure substrate.Client satisfies sandboxBackendResolver.
var _ sandboxBackendResolver = (*substrate.Client)(nil)
