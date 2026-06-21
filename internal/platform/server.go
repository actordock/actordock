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
	"github.com/actordock/actordock/internal/envd"
	"github.com/actordock/actordock/internal/logs"
	"github.com/actordock/actordock/internal/metrics"
	"github.com/actordock/actordock/internal/store"
	"github.com/actordock/actordock/internal/substrate"
	"github.com/agent-substrate/substrate/pkg/proto/ateapipb"
	"github.com/google/uuid"
)

type sandboxClient interface {
	CreateAndResumeSandbox(ctx context.Context, actorID, templateNamespace, templateName string) error
	CreateSnapshot(ctx context.Context, actorID string) (substrate.SnapshotResult, error)
	DeleteSandbox(ctx context.Context, actorID string) error
	GetActor(ctx context.Context, actorID string) (ateapipb.Actor_Status, error)
	GetActorBackend(ctx context.Context, actorID string, envdPort int) (string, error)
	SuspendSandbox(ctx context.Context, actorID string) error
	ResumeSandbox(ctx context.Context, actorID string) error
}

type sandboxStore interface {
	Put(ctx context.Context, sb store.Sandbox) error
	Get(ctx context.Context, sandboxID string) (store.Sandbox, error)
	Delete(ctx context.Context, sandboxID string) error
	List(ctx context.Context) ([]store.Sandbox, error)
}

type snapshotStore interface {
	PutSnapshot(ctx context.Context, snap store.Snapshot) error
	GetSnapshot(ctx context.Context, snapshotID string) (store.Snapshot, error)
	ListSnapshots(ctx context.Context) ([]store.Snapshot, error)
}

type volumeStore interface {
	PutVolume(ctx context.Context, vol store.Volume) error
	GetVolume(ctx context.Context, volumeID string) (store.Volume, error)
	GetVolumeByName(ctx context.Context, name string) (store.Volume, error)
	ListVolumes(ctx context.Context) ([]store.Volume, error)
	DeleteVolume(ctx context.Context, volumeID string) error
}

type platformStore interface {
	sandboxStore
	snapshotStore
	volumeStore
}

type Server struct {
	cfg       config.Platform
	actors    sandboxClient
	store     platformStore
	templates TemplateCatalog
	logger    *slog.Logger
	nowFunc   func() time.Time
}

func NewServer(cfg config.Platform, actors sandboxClient, st platformStore, logger *slog.Logger) *Server {
	return NewServerWithCatalog(cfg, actors, st, nil, logger)
}

func NewServerWithCatalog(cfg config.Platform, actors sandboxClient, st platformStore, catalog TemplateCatalog, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if catalog == nil {
		catalog = NewStaticTemplateCatalog(cfg)
	}
	return &Server{
		cfg:       cfg,
		actors:    actors,
		store:     st,
		templates: catalog,
		logger:    logger,
		nowFunc:   time.Now,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.Handle("GET /sandboxes", s.requireAPIKey(http.HandlerFunc(s.handleListSandboxes)))
	mux.Handle("GET /v2/sandboxes", s.requireAPIKey(http.HandlerFunc(s.handleListSandboxes)))
	mux.Handle("GET /sandboxes/metrics", s.requireAPIKey(http.HandlerFunc(s.handleListSandboxMetrics)))
	mux.Handle("GET /sandboxes/{id}/metrics", s.requireAPIKey(http.HandlerFunc(s.handleGetSandboxMetrics)))
	mux.Handle("GET /sandboxes/{id}/logs", s.requireAPIKey(http.HandlerFunc(s.handleGetSandboxLogs)))
	mux.Handle("GET /v2/sandboxes/{id}/logs", s.requireAPIKey(http.HandlerFunc(s.handleGetSandboxLogsV2)))
	mux.Handle("GET /sandboxes/{id}", s.requireAPIKey(http.HandlerFunc(s.handleGetSandbox)))
	mux.Handle("POST /sandboxes", s.requireAPIKey(http.HandlerFunc(s.handleCreateSandbox)))
	mux.Handle("POST /sandboxes/{id}/timeout", s.requireAPIKey(http.HandlerFunc(s.handleSetSandboxTimeout)))
	mux.Handle("POST /sandboxes/{id}/refreshes", s.requireAPIKey(http.HandlerFunc(s.handleRefreshSandbox)))
	mux.Handle("POST /sandboxes/{id}/pause", s.requireAPIKey(http.HandlerFunc(s.handlePauseSandbox)))
	mux.Handle("POST /sandboxes/{id}/resume", s.requireAPIKey(http.HandlerFunc(s.handleResumeSandbox)))
	mux.Handle("POST /sandboxes/{id}/connect", s.requireAPIKey(http.HandlerFunc(s.handleConnectSandbox)))
	mux.Handle("PUT /sandboxes/{id}/network", s.requireAPIKey(http.HandlerFunc(s.handlePutSandboxNetwork)))
	mux.Handle("POST /sandboxes/{id}/snapshots", s.requireAPIKey(http.HandlerFunc(s.handleCreateSandboxSnapshot)))
	mux.Handle("GET /snapshots", s.requireAPIKey(http.HandlerFunc(s.handleListSnapshots)))
	mux.Handle("GET /volumes", s.requireAPIKey(http.HandlerFunc(s.handleListVolumes)))
	mux.Handle("POST /volumes", s.requireAPIKey(http.HandlerFunc(s.handleCreateVolume)))
	mux.Handle("GET /volumes/{volumeID}", s.requireAPIKey(http.HandlerFunc(s.handleGetVolume)))
	mux.Handle("DELETE /volumes/{volumeID}", s.requireAPIKey(http.HandlerFunc(s.handleDeleteVolume)))
	mux.Handle("GET /templates", s.requireAPIKey(http.HandlerFunc(s.handleListTemplates)))
	mux.Handle("GET /templates/{path...}", s.requireAPIKey(http.HandlerFunc(s.handleTemplatePath)))
	mux.Handle("DELETE /sandboxes/{id}", s.requireAPIKey(http.HandlerFunc(s.handleDeleteSandbox)))
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

type createSandboxAutoResume struct {
	Enabled bool `json:"enabled"`
}

type createSandboxLifecycle struct {
	OnTimeout string `json:"onTimeout"`
}

type createSandboxRequest struct {
	TemplateID   string                   `json:"templateID"`
	Secure       *bool                    `json:"secure,omitempty"`
	Timeout      *int                     `json:"timeout,omitempty"`
	AutoPause    *bool                    `json:"autoPause,omitempty"`
	AutoResume   *createSandboxAutoResume `json:"autoResume,omitempty"`
	Lifecycle    *createSandboxLifecycle  `json:"lifecycle,omitempty"`
	VolumeMounts []store.VolumeMount      `json:"volumeMounts,omitempty"`
}

type setSandboxTimeoutRequest struct {
	Timeout int `json:"timeout"`
}

type refreshSandboxRequest struct {
	Duration *int `json:"duration,omitempty"`
}

type resumeSandboxRequest struct {
	Timeout   *int  `json:"timeout,omitempty"`
	AutoPause *bool `json:"autoPause,omitempty"`
}

type connectSandboxRequest struct {
	Timeout int `json:"timeout"`
}

const resumeDefaultTimeoutSeconds = 15

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

	ctx := r.Context()
	tmpl, err := s.templates.Get(ctx, req.TemplateID)
	if err != nil {
		if errors.Is(err, ErrTemplateNotFound) {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("unsupported template %q", req.TemplateID))
			return
		}
		s.logger.Error("resolve template", "template_id", req.TemplateID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create sandbox")
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

	timeoutSeconds, err := store.ResolveTimeout(req.Timeout, s.cfg.DefaultSandboxTimeout)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid timeout")
		return
	}

	onTimeout, autoResume, err := resolveCreateLifecycle(req)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid lifecycle")
		return
	}

	volumeMounts, err := s.resolveVolumeMounts(ctx, req.VolumeMounts)
	if err != nil {
		if errors.Is(err, store.ErrUnknownVolume) {
			writeAPIError(w, http.StatusBadRequest, "volume not found")
			return
		}
		if errors.Is(err, store.ErrInvalidMountPath) || errors.Is(err, store.ErrDuplicateMountPath) {
			writeAPIError(w, http.StatusBadRequest, "invalid volumeMounts")
			return
		}
		writeAPIError(w, http.StatusBadRequest, "invalid volumeMounts")
		return
	}

	actorID, err := newActorID()
	if err != nil {
		s.logger.Error("generate actor id", "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create sandbox")
		return
	}

	if err := s.actors.CreateAndResumeSandbox(ctx, actorID, tmpl.Namespace, tmpl.Name); err != nil {
		s.logger.Error("create sandbox", "actor_id", actorID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create sandbox")
		return
	}

	now := s.nowFunc()
	expiresAt := store.ExpiresAt(now, timeoutSeconds)
	if err := s.store.Put(ctx, store.Sandbox{
		SandboxID:    actorID,
		ActorID:      actorID,
		Template:     tmpl.TemplateID,
		CreatedAt:    now,
		ExpiresAt:    expiresAt,
		OnTimeout:    onTimeout,
		AutoResume:   autoResume,
		Status:       store.StatusRunning,
		VolumeMounts: volumeMounts,
	}); err != nil {
		s.logger.Error("persist sandbox", "actor_id", actorID, "err", err)
		if delErr := s.actors.DeleteSandbox(ctx, actorID); delErr != nil {
			s.logger.Error("rollback sandbox after store failure", "actor_id", actorID, "err", delErr)
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to create sandbox")
		return
	}

	resp := buildSandboxResponse(s.cfg, store.Sandbox{
		SandboxID: actorID,
		Template:  tmpl.TemplateID,
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleConnectSandbox(w http.ResponseWriter, r *http.Request) {
	sandboxID := r.PathValue("id")
	if sandboxID == "" {
		writeAPIError(w, http.StatusBadRequest, "sandbox id is required")
		return
	}

	var req connectSandboxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := store.ValidateTimeout(req.Timeout); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid timeout")
		return
	}

	ctx := r.Context()
	sb, err := s.store.Get(ctx, sandboxID)
	if errors.Is(err, store.ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "sandbox not found")
		return
	}
	if err != nil {
		s.logger.Error("get sandbox for connect", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to connect sandbox")
		return
	}

	wasPaused := sb.Status == store.StatusPaused
	if wasPaused {
		if err := s.actors.ResumeSandbox(ctx, sb.ActorID); err != nil {
			if errors.Is(err, substrate.ErrNotFound) {
				writeAPIError(w, http.StatusNotFound, "sandbox not found")
				return
			}
			s.logger.Error("resume sandbox for connect", "sandbox_id", sandboxID, "err", err)
			writeAPIError(w, http.StatusInternalServerError, "failed to connect sandbox")
			return
		}
		sb.Status = store.StatusRunning
	}

	if err := envd.WaitForBackendReady(ctx, func(ctx context.Context) (string, error) {
		return s.actors.GetActorBackend(ctx, sb.ActorID, s.cfg.EnvdPort)
	}, envd.DefaultReadyTimeout); err != nil {
		s.logger.Error("wait for envd on connect", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to connect sandbox")
		return
	}

	now := s.nowFunc()
	sb.ExpiresAt = resolveConnectExpiresAt(now, sb.ExpiresAt, req.Timeout, wasPaused)
	if err := s.store.Put(ctx, sb); err != nil {
		s.logger.Error("persist sandbox on connect", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to connect sandbox")
		return
	}

	resp := buildSandboxResponse(s.cfg, sb)
	w.Header().Set("Content-Type", "application/json")
	if wasPaused {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func resolveConnectExpiresAt(now, current time.Time, timeoutSeconds int, wasPaused bool) time.Time {
	newExpiry := store.ExpiresAt(now, timeoutSeconds)
	if wasPaused || current.IsZero() || !current.After(newExpiry) {
		return newExpiry
	}
	return current
}

func (s *Server) handlePauseSandbox(w http.ResponseWriter, r *http.Request) {
	sandboxID := r.PathValue("id")
	if sandboxID == "" {
		writeAPIError(w, http.StatusBadRequest, "sandbox id is required")
		return
	}

	ctx := r.Context()
	sb, err := s.store.Get(ctx, sandboxID)
	if errors.Is(err, store.ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "sandbox not found")
		return
	}
	if err != nil {
		s.logger.Error("get sandbox for pause", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to pause sandbox")
		return
	}

	if err := s.actors.SuspendSandbox(ctx, sb.ActorID); err != nil {
		if errors.Is(err, substrate.ErrNotFound) {
			writeAPIError(w, http.StatusNotFound, "sandbox not found")
			return
		}
		s.logger.Error("pause sandbox", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to pause sandbox")
		return
	}

	sb.Status = store.StatusPaused
	if err := s.store.Put(ctx, sb); err != nil {
		s.logger.Error("persist paused sandbox", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to pause sandbox")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResumeSandbox(w http.ResponseWriter, r *http.Request) {
	sandboxID := r.PathValue("id")
	if sandboxID == "" {
		writeAPIError(w, http.StatusBadRequest, "sandbox id is required")
		return
	}

	var req resumeSandboxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	timeoutSeconds, err := store.ResolveTimeout(req.Timeout, resumeDefaultTimeoutSeconds)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid timeout")
		return
	}

	ctx := r.Context()
	sb, err := s.store.Get(ctx, sandboxID)
	if errors.Is(err, store.ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "sandbox not found")
		return
	}
	if err != nil {
		s.logger.Error("get sandbox for resume", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to resume sandbox")
		return
	}

	onTimeout, err := resolveResumeOnTimeout(sb.OnTimeout, req)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid lifecycle")
		return
	}
	if onTimeout == store.OnTimeoutKill {
		sb.AutoResume = false
	} else if err := store.ValidateAutoResume(onTimeout, sb.AutoResume); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid lifecycle")
		return
	}

	if err := s.actors.ResumeSandbox(ctx, sb.ActorID); err != nil {
		if errors.Is(err, substrate.ErrNotFound) {
			writeAPIError(w, http.StatusNotFound, "sandbox not found")
			return
		}
		s.logger.Error("resume sandbox", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to resume sandbox")
		return
	}
	if err := envd.WaitForBackendReady(ctx, func(ctx context.Context) (string, error) {
		return s.actors.GetActorBackend(ctx, sb.ActorID, s.cfg.EnvdPort)
	}, envd.DefaultReadyTimeout); err != nil {
		s.logger.Error("wait for resumed sandbox", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to resume sandbox")
		return
	}

	now := s.nowFunc()
	sb.ExpiresAt = store.ExpiresAt(now, timeoutSeconds)
	sb.OnTimeout = onTimeout
	sb.Status = store.StatusRunning
	if err := s.store.Put(ctx, sb); err != nil {
		s.logger.Error("persist resumed sandbox", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to resume sandbox")
		return
	}

	resp := buildSandboxResponse(s.cfg, sb)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleDeleteSandbox(w http.ResponseWriter, r *http.Request) {
	sandboxID := r.PathValue("id")
	if sandboxID == "" {
		writeAPIError(w, http.StatusBadRequest, "sandbox id is required")
		return
	}

	ctx := r.Context()
	if err := s.actors.DeleteSandbox(ctx, sandboxID); err != nil {
		if errors.Is(err, substrate.ErrNotFound) {
			writeAPIError(w, http.StatusNotFound, "sandbox not found")
			return
		}
		s.logger.Error("delete sandbox", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to delete sandbox")
		return
	}
	if err := s.store.Delete(ctx, sandboxID); err != nil && !errors.Is(err, store.ErrNotFound) {
		s.logger.Error("delete sandbox metadata", "sandbox_id", sandboxID, "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSetSandboxTimeout(w http.ResponseWriter, r *http.Request) {
	sandboxID := r.PathValue("id")
	if sandboxID == "" {
		writeAPIError(w, http.StatusBadRequest, "sandbox id is required")
		return
	}

	var req setSandboxTimeoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := store.ValidateTimeout(req.Timeout); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid timeout")
		return
	}

	ctx := r.Context()
	sb, err := s.store.Get(ctx, sandboxID)
	if errors.Is(err, store.ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "sandbox not found")
		return
	}
	if err != nil {
		s.logger.Error("get sandbox for timeout", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to set sandbox timeout")
		return
	}

	now := s.nowFunc()
	sb.ExpiresAt = store.ExpiresAt(now, req.Timeout)
	if err := s.store.Put(ctx, sb); err != nil {
		s.logger.Error("update sandbox timeout", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to set sandbox timeout")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRefreshSandbox(w http.ResponseWriter, r *http.Request) {
	sandboxID := r.PathValue("id")
	if sandboxID == "" {
		writeAPIError(w, http.StatusBadRequest, "sandbox id is required")
		return
	}

	var req refreshSandboxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	durationSeconds, err := store.ResolveRefreshDuration(req.Duration, s.cfg.DefaultSandboxTimeout)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid duration")
		return
	}

	ctx := r.Context()
	sb, err := s.store.Get(ctx, sandboxID)
	if errors.Is(err, store.ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "sandbox not found")
		return
	}
	if err != nil {
		s.logger.Error("get sandbox for refresh", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to refresh sandbox")
		return
	}

	now := s.nowFunc()
	sb.ExpiresAt = store.ExpiresAt(now, durationSeconds)
	if err := s.store.Put(ctx, sb); err != nil {
		s.logger.Error("update sandbox refresh", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to refresh sandbox")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListSandboxMetrics(w http.ResponseWriter, r *http.Request) {
	ids, err := parseSandboxIDs(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid sandbox_ids")
		return
	}

	sandboxes := make(map[string]sandboxMetricResponse, len(ids))
	for _, id := range ids {
		sb, err := s.store.Get(r.Context(), id)
		if err != nil {
			sandboxes[id] = metrics.FallbackSample(s.nowFunc())
			continue
		}
		sandboxes[id] = s.fetchSandboxMetric(r.Context(), sb)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sandboxesWithMetricsResponse{Sandboxes: sandboxes})
}

func (s *Server) handleGetSandboxMetrics(w http.ResponseWriter, r *http.Request) {
	sandboxID := r.PathValue("id")
	if sandboxID == "" {
		writeAPIError(w, http.StatusBadRequest, "sandbox id is required")
		return
	}
	if err := parseMetricsIntervalQuery(r); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid metrics query")
		return
	}

	sb, err := s.store.Get(r.Context(), sandboxID)
	if errors.Is(err, store.ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "sandbox not found")
		return
	}
	if err != nil {
		s.logger.Error("get sandbox metrics", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to get sandbox metrics")
		return
	}

	var samples []sandboxMetricResponse
	if metricsIntervalRequested(r) {
		samples = s.fetchSandboxMetricsHistory(r.Context(), sb, r.URL.RawQuery)
		if samples == nil {
			samples = []sandboxMetricResponse{}
		}
	} else {
		samples = []sandboxMetricResponse{s.fetchSandboxMetric(r.Context(), sb)}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(samples)
}

func (s *Server) handleGetSandboxLogs(w http.ResponseWriter, r *http.Request) {
	sandboxID := r.PathValue("id")
	if sandboxID == "" {
		writeAPIError(w, http.StatusBadRequest, "sandbox id is required")
		return
	}
	if err := parseLogsV1Query(r); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid logs query")
		return
	}

	sb, err := s.store.Get(r.Context(), sandboxID)
	if errors.Is(err, store.ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "sandbox not found")
		return
	}
	if err != nil {
		s.logger.Error("get sandbox logs", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to get sandbox logs")
		return
	}

	resp := logs.ToV1(s.fetchSandboxLogEntries(r.Context(), sb, r.URL.RawQuery))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleGetSandboxLogsV2(w http.ResponseWriter, r *http.Request) {
	sandboxID := r.PathValue("id")
	if sandboxID == "" {
		writeAPIError(w, http.StatusBadRequest, "sandbox id is required")
		return
	}
	if err := parseLogsV2Query(r); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid logs query")
		return
	}

	sb, err := s.store.Get(r.Context(), sandboxID)
	if errors.Is(err, store.ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "sandbox not found")
		return
	}
	if err != nil {
		s.logger.Error("get sandbox logs v2", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to get sandbox logs")
		return
	}

	resp := logs.ToV2(s.fetchSandboxLogEntries(r.Context(), sb, r.URL.RawQuery))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleGetSandbox(w http.ResponseWriter, r *http.Request) {
	sandboxID := r.PathValue("id")
	if sandboxID == "" {
		writeAPIError(w, http.StatusBadRequest, "sandbox id is required")
		return
	}

	detail, err := s.resolveSandbox(r.Context(), sandboxID)
	if errors.Is(err, store.ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "sandbox not found")
		return
	}
	if err != nil {
		s.logger.Error("get sandbox", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to get sandbox")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(detail)
}

func (s *Server) handleListSandboxes(w http.ResponseWriter, r *http.Request) {
	sandboxes, err := s.store.List(r.Context())
	if err != nil {
		s.logger.Error("list sandboxes", "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to list sandboxes")
		return
	}

	result := make([]listedSandboxResponse, 0, len(sandboxes))
	for _, sb := range sandboxes {
		detail, err := s.syncSandboxRecord(r.Context(), sb)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			s.logger.Error("sync sandbox for list", "sandbox_id", sb.SandboxID, "err", err)
			continue
		}
		result = append(result, listedFromDetail(detail))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) resolveSandbox(ctx context.Context, sandboxID string) (sandboxDetailResponse, error) {
	sb, err := s.store.Get(ctx, sandboxID)
	if err != nil {
		return sandboxDetailResponse{}, err
	}
	return s.syncSandboxRecord(ctx, sb)
}

func (s *Server) syncSandboxRecord(ctx context.Context, sb store.Sandbox) (sandboxDetailResponse, error) {
	actorStatus, err := s.actors.GetActor(ctx, sb.ActorID)
	if errors.Is(err, substrate.ErrNotFound) {
		if delErr := s.store.Delete(ctx, sb.SandboxID); delErr != nil && !errors.Is(delErr, store.ErrNotFound) {
			s.logger.Error("purge stale sandbox", "sandbox_id", sb.SandboxID, "err", delErr)
		}
		return sandboxDetailResponse{}, store.ErrNotFound
	}
	if err != nil {
		return sandboxDetailResponse{}, err
	}

	storeStatus := storeStatusFromActor(actorStatus)
	wasPaused := sb.Status == store.StatusPaused
	origStatus := sb.Status
	origExpiresAt := sb.ExpiresAt

	sb.Status = storeStatus
	if wasPaused && storeStatus == store.StatusRunning && store.IsExpired(sb, s.nowFunc()) {
		sb.ExpiresAt = store.ExpiresAt(s.nowFunc(), s.cfg.DefaultSandboxTimeout)
	}

	if sb.Status != origStatus || !sb.ExpiresAt.Equal(origExpiresAt) {
		if err := s.store.Put(ctx, sb); err != nil {
			s.logger.Error("update sandbox status", "sandbox_id", sb.SandboxID, "err", err)
		}
	}

	return buildSandboxDetail(s.cfg, sb, substrate.ActorStateE2B(actorStatus), s.templateAlias(ctx, sb.Template)), nil
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

// resolveCreateLifecycle maps E2B create body to stored on_timeout and auto_resume.
func resolveCreateLifecycle(req createSandboxRequest) (string, bool, error) {
	onTimeout, err := resolveCreateOnTimeout(req)
	if err != nil {
		return "", false, err
	}
	autoResume := false
	if req.AutoResume != nil {
		autoResume = req.AutoResume.Enabled
	}
	if err := store.ValidateAutoResume(onTimeout, autoResume); err != nil {
		return "", false, err
	}
	return onTimeout, autoResume, nil
}

// resolveCreateOnTimeout maps E2B create body fields to stored on_timeout.
// autoPause=true is equivalent to lifecycle.onTimeout=pause; conflicting values are rejected.
func resolveCreateOnTimeout(req createSandboxRequest) (string, error) {
	lifecycleOnTimeout := ""
	if req.Lifecycle != nil {
		lifecycleOnTimeout = req.Lifecycle.OnTimeout
	}
	resolved, err := store.ResolveOnTimeout(lifecycleOnTimeout)
	if err != nil {
		return "", err
	}

	lifecycleExplicit := req.Lifecycle != nil && req.Lifecycle.OnTimeout != ""
	if req.AutoPause != nil {
		autoPause := *req.AutoPause
		if lifecycleExplicit {
			lifecyclePause := resolved == store.OnTimeoutPause
			if lifecyclePause != autoPause {
				return "", store.ErrInvalidOnTimeout
			}
		} else if autoPause {
			resolved = store.OnTimeoutPause
		}
	}
	return resolved, nil
}

func resolveResumeOnTimeout(currentOnTimeout string, req resumeSandboxRequest) (string, error) {
	if req.AutoPause == nil {
		return store.ResolveOnTimeout(currentOnTimeout)
	}
	if *req.AutoPause {
		return store.OnTimeoutPause, nil
	}
	return store.OnTimeoutKill, nil
}

// Ensure substrate.Client satisfies sandboxClient.
var _ sandboxClient = (*substrate.Client)(nil)

// Ensure store.Redis satisfies platformStore.
var _ platformStore = (*store.Redis)(nil)
