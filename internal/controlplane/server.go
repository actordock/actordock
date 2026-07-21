// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/actordock/actordock/internal/scheduler"
	"github.com/actordock/actordock/internal/store"
	"github.com/actordock/actordock/internal/types"
)

// Server is the Actordock control-plane HTTP API (slim Substrate ateapi surface).
type Server struct {
	sched          *scheduler.Scheduler
	store          store.Store
	log            *slog.Logger
	metricsHandler http.Handler
}

func New(sched *scheduler.Scheduler, st store.Store, log *slog.Logger, metricsHandler http.Handler) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{sched: sched, store: st, log: log, metricsHandler: metricsHandler}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	if s.metricsHandler != nil {
		mux.Handle("GET /metrics", s.metricsHandler)
	}
	mux.HandleFunc("GET /v1/policy", s.getPolicy)
	mux.HandleFunc("POST /v1/golden/ensure", s.ensureGolden)
	mux.HandleFunc("GET /v1/golden", s.getGolden)

	mux.HandleFunc("POST /v1/sandboxes", s.createSandbox)
	mux.HandleFunc("GET /v1/sandboxes", s.listSandboxes)
	mux.HandleFunc("GET /v1/sandboxes/{id}", s.getSandbox)
	mux.HandleFunc("DELETE /v1/sandboxes/{id}", s.deleteSandbox)
	mux.HandleFunc("POST /v1/sandboxes/{id}/resume", s.resumeSandbox)
	mux.HandleFunc("POST /v1/sandboxes/{id}/pause", s.pauseSandbox)
	mux.HandleFunc("POST /v1/sandboxes/{id}/suspend", s.suspendSandbox)
	mux.HandleFunc("POST /v1/sandboxes/{id}/exec", s.execSandbox)

	mux.HandleFunc("POST /v1/workers/register", s.registerWorker)
	mux.HandleFunc("GET /v1/workers", s.listWorkers)
	return mux
}

func (s *Server) createSandbox(w http.ResponseWriter, r *http.Request) {
	sb, err := s.sched.Create(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, sb)
}

func (s *Server) listSandboxes(w http.ResponseWriter, r *http.Request) {
	list, err := s.sched.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getSandbox(w http.ResponseWriter, r *http.Request) {
	sb, err := s.sched.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, sb)
}

func (s *Server) deleteSandbox(w http.ResponseWriter, r *http.Request) {
	if err := s.sched.Delete(r.Context(), r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resumeSandbox(w http.ResponseWriter, r *http.Request) {
	sb, err := s.sched.Resume(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, sb)
}

func (s *Server) pauseSandbox(w http.ResponseWriter, r *http.Request) {
	sb, err := s.sched.Pause(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, sb)
}

func (s *Server) suspendSandbox(w http.ResponseWriter, r *http.Request) {
	sb, err := s.sched.Suspend(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, sb)
}

func (s *Server) execSandbox(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Argv []string `json:"argv"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Argv) == 0 {
		http.Error(w, "argv required", http.StatusBadRequest)
		return
	}
	out, err := s.sched.Exec(r.Context(), r.PathValue("id"), req.Argv)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"stdout": out})
}

func (s *Server) ensureGolden(w http.ResponseWriter, r *http.Request) {
	prefix, err := s.sched.EnsureGolden(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"objectKey": prefix})
}

func (s *Server) getGolden(w http.ResponseWriter, r *http.Request) {
	prefix, err := s.store.GetGolden(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"objectKey": prefix})
}

func (s *Server) listWorkers(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListWorkers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) registerWorker(w http.ResponseWriter, r *http.Request) {
	var req types.Worker
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ID == "" || req.Address == "" {
		http.Error(w, "id and address required", http.StatusBadRequest)
		return
	}
	req.MaxSlots = 1
	req.Healthy = true
	if req.RegisteredAt.IsZero() {
		req.RegisteredAt = time.Now().UTC()
	}
	if err := s.store.PutWorker(r.Context(), req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

func (s *Server) getPolicy(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"policy": s.sched.PolicyName()})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
