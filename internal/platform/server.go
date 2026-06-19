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

package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/actordock/actordock/internal/config"
	"github.com/actordock/actordock/internal/substrate"
	"github.com/google/uuid"
)

type sandboxCreator interface {
	CreateAndResumeSandbox(ctx context.Context, actorID, templateNamespace, templateName string) error
}

type Server struct {
	cfg     config.Platform
	actors  sandboxCreator
	logger  *slog.Logger
	nowFunc func() time.Time
}

func NewServer(cfg config.Platform, actors sandboxCreator, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		cfg:     cfg,
		actors:  actors,
		logger:  logger,
		nowFunc: time.Now,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.Handle("POST /sandboxes", s.requireAPIKey(http.HandlerFunc(s.handleCreateSandbox)))
	return mux
}

func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.cfg.ListenAddr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("platform listening", "addr", s.cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("platform serve: %w", err)
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
	s.logger.Info("platform shutting down")
	return srv.Shutdown(shutdownCtx)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

type createSandboxRequest struct {
	TemplateID string `json:"templateID"`
	Secure     *bool  `json:"secure,omitempty"`
}

type sandboxResponse struct {
	ClientID        string `json:"clientID"`
	EnvdVersion     string `json:"envdVersion"`
	SandboxID       string `json:"sandboxID"`
	TemplateID      string `json:"templateID"`
	Domain          string `json:"domain"`
	EnvdAccessToken string `json:"envdAccessToken,omitempty"`
}

type apiError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (s *Server) handleCreateSandbox(w http.ResponseWriter, r *http.Request) {
	var req createSandboxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.TemplateID == "" {
		writeAPIError(w, http.StatusBadRequest, "templateID is required")
		return
	}
	if req.TemplateID != s.cfg.TemplateName {
		writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("unsupported template %q (only %q for v0.0.1)", req.TemplateID, s.cfg.TemplateName))
		return
	}

	secure := false
	if req.Secure != nil {
		secure = *req.Secure
	}
	if secure {
		writeAPIError(w, http.StatusBadRequest, "secure sandboxes are not supported in v0.0.1")
		return
	}

	actorID, err := newActorID()
	if err != nil {
		s.logger.Error("generate actor id", "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create sandbox")
		return
	}

	ctx := r.Context()
	if err := s.actors.CreateAndResumeSandbox(ctx, actorID, s.cfg.TemplateNamespace, s.cfg.TemplateName); err != nil {
		s.logger.Error("create sandbox", "actor_id", actorID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create sandbox")
		return
	}

	resp := sandboxResponse{
		ClientID:    s.cfg.ClientID,
		EnvdVersion: s.cfg.EnvdVersion,
		SandboxID:   actorID,
		TemplateID:  req.TemplateID,
		Domain:      s.cfg.Domain,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) requireAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != s.cfg.APIKey {
			writeAPIError(w, http.StatusUnauthorized, "invalid API key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiError{
		Message: message,
		Code:    status,
	})
}

func newActorID() (string, error) {
	id := strings.ToLower(uuid.NewString())
	if len(id) > 63 {
		return "", fmt.Errorf("actor id too long")
	}
	return id, nil
}

// Ensure substrate.Client satisfies sandboxCreator.
var _ sandboxCreator = (*substrate.Client)(nil)
