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
	"net/http"
	"strings"

	"github.com/actordock/actordock/internal/store"
	"github.com/google/uuid"
)

type createVolumeRequest struct {
	Name string `json:"name"`
}

type volumeResponse struct {
	VolumeID  string `json:"volumeID"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type volumeAndTokenResponse struct {
	VolumeID  string `json:"volumeID"`
	Name      string `json:"name"`
	Token     string `json:"token"`
	HostPath  string `json:"hostPath,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
}

func (s *Server) handleCreateVolume(w http.ResponseWriter, r *http.Request) {
	var req createVolumeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := store.ValidateVolumeName(req.Name); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid volume name")
		return
	}

	token, err := store.NewVolumeToken()
	if err != nil {
		s.logger.Error("generate volume token", "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create volume")
		return
	}

	volumeID := uuid.NewString()
	now := s.nowFunc().UTC()
	vol := store.Volume{
		VolumeID:  volumeID,
		Name:      req.Name,
		Token:     token,
		HostPath:  store.VolumeHostPath(s.cfg.VolumeRoot, volumeID),
		CreatedAt: now,
	}

	ctx := r.Context()
	if err := s.store.PutVolume(ctx, vol); err != nil {
		if errors.Is(err, store.ErrVolumeNameTaken) {
			writeAPIError(w, http.StatusBadRequest, "volume name already exists")
			return
		}
		s.logger.Error("persist volume", "name", req.Name, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create volume")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(volumeAndTokenResponse{
		VolumeID: volumeID,
		Name:     req.Name,
		Token:    token,
	})
}

func (s *Server) handleListVolumes(w http.ResponseWriter, r *http.Request) {
	volumes, err := s.store.ListVolumes(r.Context())
	if err != nil {
		s.logger.Error("list volumes", "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to list volumes")
		return
	}

	resp := make([]volumeResponse, 0, len(volumes))
	for _, vol := range volumes {
		resp = append(resp, volumeResponse{
			VolumeID:  vol.VolumeID,
			Name:      vol.Name,
			CreatedAt: formatRFC3339(vol.CreatedAt),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleGetVolume(w http.ResponseWriter, r *http.Request) {
	volumeID := r.PathValue("volumeID")
	if volumeID == "" {
		writeAPIError(w, http.StatusBadRequest, "volume id is required")
		return
	}

	vol, err := s.store.GetVolume(r.Context(), volumeID)
	if errors.Is(err, store.ErrVolumeNotFound) {
		writeAPIError(w, http.StatusNotFound, "volume not found")
		return
	}
	if err != nil {
		s.logger.Error("get volume", "volume_id", volumeID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to get volume")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(volumeAndTokenResponse{
		VolumeID:  vol.VolumeID,
		Name:      vol.Name,
		Token:     vol.Token,
		HostPath:  vol.HostPath,
		CreatedAt: formatRFC3339(vol.CreatedAt),
	})
}

func (s *Server) handleDeleteVolume(w http.ResponseWriter, r *http.Request) {
	volumeID := r.PathValue("volumeID")
	if volumeID == "" {
		writeAPIError(w, http.StatusBadRequest, "volume id is required")
		return
	}

	if err := s.store.DeleteVolume(r.Context(), volumeID); errors.Is(err, store.ErrVolumeNotFound) {
		writeAPIError(w, http.StatusNotFound, "volume not found")
		return
	} else if err != nil {
		s.logger.Error("delete volume", "volume_id", volumeID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to delete volume")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) lookupVolume(ctx context.Context, nameOrID string) (store.Volume, error) {
	vol, err := s.store.GetVolumeByName(ctx, nameOrID)
	if err == nil {
		return vol, nil
	}
	if !errors.Is(err, store.ErrVolumeNotFound) {
		return store.Volume{}, err
	}
	return s.store.GetVolume(ctx, nameOrID)
}

func (s *Server) resolveVolumeMounts(ctx context.Context, mounts []store.VolumeMount) ([]store.VolumeMount, error) {
	return store.ValidateVolumeMounts(mounts, func(nameOrID string) (store.Volume, error) {
		return s.lookupVolume(ctx, nameOrID)
	})
}
