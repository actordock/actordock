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
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/actordock/actordock/internal/config"
	"github.com/actordock/actordock/internal/store"
	"github.com/actordock/actordock/internal/substrate"
	"github.com/google/uuid"
)

const snapshotDefaultTag = ":default"

type createSnapshotRequest struct {
	Name string `json:"name"`
}

type snapshotInfoResponse struct {
	SnapshotID string   `json:"snapshotID"`
	Names      []string `json:"names"`
}

func (s *Server) handleCreateSandboxSnapshot(w http.ResponseWriter, r *http.Request) {
	sandboxID := r.PathValue("id")
	if sandboxID == "" {
		writeAPIError(w, http.StatusBadRequest, "sandbox id is required")
		return
	}

	var req createSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)

	ctx := r.Context()
	sb, err := s.store.Get(ctx, sandboxID)
	if errors.Is(err, store.ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "sandbox not found")
		return
	}
	if err != nil {
		s.logger.Error("get sandbox for snapshot", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create snapshot")
		return
	}

	result, err := s.actors.CreateSnapshot(ctx, sb.ActorID)
	if errors.Is(err, substrate.ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "sandbox not found")
		return
	}
	if errors.Is(err, substrate.ErrInvalidState) {
		writeAPIError(w, http.StatusBadRequest, "sandbox must be running to create a snapshot")
		return
	}
	if err != nil {
		s.logger.Error("create snapshot", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create snapshot")
		return
	}

	snapshotID, names := buildSnapshotIdentity(s.cfg, req.Name)
	now := s.nowFunc().UTC()
	record := store.Snapshot{
		SnapshotID:   snapshotID,
		Names:        names,
		SandboxID:    sandboxID,
		ActorID:      sb.ActorID,
		SnapshotURI:  result.SnapshotURI,
		SnapshotType: result.SnapshotType,
		Name:         req.Name,
		CreatedAt:    now,
	}
	if err := s.snapshots.PutSnapshot(ctx, record); err != nil {
		s.logger.Error("persist snapshot", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create snapshot")
		return
	}

	sb.Status = store.StatusPaused
	if err := s.store.Put(ctx, sb); err != nil {
		s.logger.Error("persist sandbox after snapshot", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create snapshot")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(snapshotInfoResponse{
		SnapshotID: snapshotID,
		Names:      names,
	})
}

func buildSnapshotIdentity(cfg config.Platform, name string) (string, []string) {
	if name != "" {
		id := cfg.ClientID + "/" + name + snapshotDefaultTag
		return id, []string{id}
	}
	id := uuid.NewString() + snapshotDefaultTag
	return id, []string{id}
}
