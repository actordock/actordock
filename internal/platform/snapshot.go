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
	"sort"
	"strconv"
	"strings"

	"github.com/actordock/actordock/internal/config"
	"github.com/actordock/actordock/internal/runtimeapi"
	"github.com/actordock/actordock/internal/store"
	"github.com/google/uuid"
)

const (
	snapshotDefaultTag       = ":default"
	snapshotListLimitMax     = 100
	snapshotListLimitDefault = 100
	snapshotNextTokenHeader  = "X-Next-Token"
)

type createSnapshotRequest struct {
	Name string `json:"name"`
}

type snapshotInfoResponse struct {
	SnapshotID string   `json:"snapshotID"`
	Names      []string `json:"names"`
	SandboxID  string   `json:"sandboxID,omitempty"`
	CreatedAt  string   `json:"createdAt,omitempty"`
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
	if errors.Is(err, runtimeapi.ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "sandbox not found")
		return
	}
	if errors.Is(err, runtimeapi.ErrInvalidState) {
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
	if err := s.store.PutSnapshot(ctx, record); err != nil {
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

func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	sandboxID := strings.TrimSpace(r.URL.Query().Get("sandboxID"))
	limit, err := parseSnapshotListLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	nextToken := strings.TrimSpace(r.URL.Query().Get("nextToken"))

	snapshots, err := s.store.ListSnapshots(r.Context())
	if err != nil {
		s.logger.Error("list snapshots", "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to list snapshots")
		return
	}

	page, next, err := paginateSnapshots(filterSnapshots(snapshots, sandboxID), nextToken, limit)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid nextToken")
		return
	}

	resp := make([]snapshotInfoResponse, 0, len(page))
	for _, snap := range page {
		resp = append(resp, snapshotInfoFromRecord(snap))
	}

	w.Header().Set("Content-Type", "application/json")
	if next != "" {
		w.Header().Set(snapshotNextTokenHeader, next)
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func snapshotInfoFromRecord(s store.Snapshot) snapshotInfoResponse {
	names := append([]string(nil), s.Names...)
	return snapshotInfoResponse{
		SnapshotID: s.SnapshotID,
		Names:      names,
		SandboxID:  s.SandboxID,
		CreatedAt:  formatRFC3339(s.CreatedAt),
	}
}

func parseSnapshotListLimit(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return snapshotListLimitDefault, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > snapshotListLimitMax {
		return 0, errors.New("invalid limit")
	}
	return limit, nil
}

func filterSnapshots(snapshots []store.Snapshot, sandboxID string) []store.Snapshot {
	if sandboxID == "" {
		return append([]store.Snapshot(nil), snapshots...)
	}
	filtered := make([]store.Snapshot, 0, len(snapshots))
	for _, snap := range snapshots {
		if snap.SandboxID == sandboxID {
			filtered = append(filtered, snap)
		}
	}
	return filtered
}

func paginateSnapshots(snapshots []store.Snapshot, nextToken string, limit int) ([]store.Snapshot, string, error) {
	sorted := append([]store.Snapshot(nil), snapshots...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].CreatedAt.Equal(sorted[j].CreatedAt) {
			return sorted[i].SnapshotID < sorted[j].SnapshotID
		}
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})

	start := 0
	if nextToken != "" {
		found := false
		for i, snap := range sorted {
			if snapshotCursor(snap) == nextToken {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return []store.Snapshot{}, "", nil
		}
	}

	end := start + limit
	if end > len(sorted) {
		end = len(sorted)
	}
	page := sorted[start:end]
	if end < len(sorted) && len(page) > 0 {
		return page, snapshotCursor(page[len(page)-1]), nil
	}
	return page, "", nil
}

func snapshotCursor(s store.Snapshot) string {
	return s.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z") + "|" + s.SnapshotID
}

func buildSnapshotIdentity(cfg config.Platform, name string) (string, []string) {
	if name != "" {
		id := cfg.ClientID + "/" + name + snapshotDefaultTag
		return id, []string{id}
	}
	id := uuid.NewString() + snapshotDefaultTag
	return id, []string{id}
}
