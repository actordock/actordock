// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package workerserver

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/actordock/actordock/internal/metrics"
	"github.com/actordock/actordock/internal/runtime"
	"github.com/actordock/actordock/internal/snapshotstore"
)

// Server is the Worker agent HTTP API (1 running sandbox per Worker).
type Server struct {
	workerID       string
	rt             runtime.Runtime
	snaps          snapshotstore.Store
	log            *slog.Logger
	metrics        *metrics.Metrics
	metricsHandler http.Handler

	mu    sync.Mutex
	alive map[string]struct{} // at most one entry when MaxSlots=1
}

func New(workerID string, rt runtime.Runtime, snaps snapshotstore.Store, log *slog.Logger, m *metrics.Metrics, metricsHandler http.Handler) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		workerID:       workerID,
		rt:             rt,
		snaps:          snaps,
		log:            log,
		metrics:        m,
		metricsHandler: metricsHandler,
		alive:          make(map[string]struct{}),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /status", s.handleStatus)
	if s.metricsHandler != nil {
		mux.Handle("GET /metrics", s.metricsHandler)
	}
	mux.HandleFunc("POST /sandboxes/{id}/boot", s.handleBoot)
	mux.HandleFunc("POST /sandboxes/{id}/checkpoint", s.handleCheckpoint)
	mux.HandleFunc("POST /sandboxes/{id}/restore", s.handleRestore)
	mux.HandleFunc("POST /sandboxes/{id}/exec", s.handleExec)
	mux.HandleFunc("GET /local-snapshot", s.handleLocalSnapshot)
	mux.HandleFunc("DELETE /sandboxes/{id}", s.handleDelete)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	used := len(s.alive)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"workerID":  s.workerID,
		"maxSlots":  1,
		"usedSlots": used,
		"healthy":   true,
	})
}

func (s *Server) handleLocalSnapshot(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}
	st, err := os.Stat(path)
	exists := err == nil && st.IsDir()
	writeJSON(w, http.StatusOK, map[string]any{"exists": exists})
}

func (s *Server) claim(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.alive) > 0 {
		if _, ok := s.alive[id]; !ok {
			return errBusy
		}
	}
	s.alive[id] = struct{}{}
	return nil
}

var errBusy = httpError{code: http.StatusConflict, msg: "worker busy"}

type httpError struct {
	code int
	msg  string
}

func (e httpError) Error() string { return e.msg }

func (s *Server) release(id string) {
	s.mu.Lock()
	delete(s.alive, id)
	s.mu.Unlock()
}

func (s *Server) handleBoot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	if err := s.claim(id); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.log.Info("boot begin", "id", id)
	if err := s.rt.Boot(r.Context(), runtime.SandboxSpec{ID: id}); err != nil {
		s.release(id)
		s.log.Error("boot failed", "id", id, "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.log.Info("boot ok", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCheckpoint(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		ImagePath string `json:"imagePath"`
		ObjectKey string `json:"objectKey,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ImagePath == "" {
		http.Error(w, "imagePath required", http.StatusBadRequest)
		return
	}
	if err := s.rt.Checkpoint(r.Context(), id, req.ImagePath); err != nil {
		s.log.Error("checkpoint failed", "id", id, "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if req.ObjectKey != "" {
		if s.snaps == nil {
			http.Error(w, "snapshot store not configured", http.StatusServiceUnavailable)
			return
		}
		xferStart := time.Now()
		n, err := snapshotstore.UploadDir(r.Context(), s.snaps, req.ImagePath, req.ObjectKey)
		if err != nil {
			s.log.Error("upload snapshot failed", "id", id, "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.metrics.RecordTransfer(r.Context(), "upload", time.Since(xferStart), n)
	}
	s.release(id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		ImagePath string `json:"imagePath"`
		ObjectKey string `json:"objectKey,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ImagePath == "" {
		http.Error(w, "imagePath required", http.StatusBadRequest)
		return
	}
	if err := s.claim(id); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	needDownload := true
	if st, err := os.Stat(req.ImagePath); err == nil && st.IsDir() {
		// Local dir exists; still require at least one file for a real checkpoint.
		ents, _ := os.ReadDir(req.ImagePath)
		needDownload = len(ents) == 0
	}
	if needDownload {
		if req.ObjectKey == "" || s.snaps == nil {
			s.release(id)
			http.Error(w, "local snapshot missing and no objectKey", http.StatusConflict)
			return
		}
		xferStart := time.Now()
		n, err := snapshotstore.DownloadDir(r.Context(), s.snaps, req.ObjectKey, req.ImagePath)
		if err != nil {
			s.release(id)
			s.log.Error("download snapshot failed", "id", id, "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.metrics.RecordTransfer(r.Context(), "download", time.Since(xferStart), n)
	}

	if err := s.rt.Restore(r.Context(), id, req.ImagePath); err != nil {
		s.release(id)
		s.log.Error("restore failed", "id", id, "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Argv []string `json:"argv"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Argv) == 0 {
		http.Error(w, "argv required", http.StatusBadRequest)
		return
	}
	out, err := s.rt.Exec(r.Context(), id, req.Argv)
	if err != nil {
		s.log.Error("exec failed", "id", id, "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"stdout": out})
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = s.rt.Delete(r.Context(), id)
	s.release(id)
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
