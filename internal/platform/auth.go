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
	"time"

	"github.com/actordock/actordock/internal/store"
	"github.com/google/uuid"
)

type identifierMaskResponse struct {
	Prefix            string `json:"prefix"`
	ValueLength       int    `json:"valueLength"`
	MaskedValuePrefix string `json:"maskedValuePrefix"`
	MaskedValueSuffix string `json:"maskedValueSuffix"`
}

type teamAPIKeyResponse struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Mask      identifierMaskResponse `json:"mask"`
	CreatedAt string                 `json:"createdAt"`
	CreatedBy any                    `json:"createdBy"`
	LastUsed  *string                `json:"lastUsed"`
}

type createdTeamAPIKeyResponse struct {
	ID        string                 `json:"id"`
	Key       string                 `json:"key"`
	Mask      identifierMaskResponse `json:"mask"`
	Name      string                 `json:"name"`
	CreatedAt string                 `json:"createdAt"`
	CreatedBy any                    `json:"createdBy"`
	LastUsed  *string                `json:"lastUsed"`
}

type createTeamAPIKeyRequest struct {
	Name string `json:"name"`
}

type createAccessTokenRequest struct {
	Name string `json:"name"`
}

type createdAccessTokenResponse struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Token     string                 `json:"token"`
	Mask      identifierMaskResponse `json:"mask"`
	CreatedAt string                 `json:"createdAt"`
}

func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	keys, err := s.listTeamAPIKeys(ctx)
	if err != nil {
		s.logger.Error("list api keys", "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to list api keys")
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req createTeamAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeAPIError(w, http.StatusBadRequest, "name is required")
		return
	}

	raw, err := store.NewTeamAPIKeyValue()
	if err != nil {
		s.logger.Error("generate api key", "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create api key")
		return
	}

	now := s.nowFunc().UTC()
	id := uuid.NewString()
	rec := store.TeamAPIKeyRecord{
		ID:        id,
		Name:      strings.TrimSpace(req.Name),
		KeyHash:   store.HashAPIKey(raw),
		CreatedAt: now,
	}
	if err := s.store.PutTeamAPIKey(r.Context(), rec); err != nil {
		s.logger.Error("persist api key", "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create api key")
		return
	}

	writeJSON(w, http.StatusCreated, buildCreatedTeamAPIKeyResponse(rec, raw))
}

func (s *Server) handleCreateAccessToken(w http.ResponseWriter, r *http.Request) {
	var req createAccessTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeAPIError(w, http.StatusBadRequest, "name is required")
		return
	}

	raw, err := store.NewUserAccessTokenValue()
	if err != nil {
		s.logger.Error("generate access token", "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create access token")
		return
	}

	now := s.nowFunc().UTC()
	id := uuid.NewString()
	rec := store.UserAccessTokenRecord{
		ID:        id,
		Name:      strings.TrimSpace(req.Name),
		Token:     raw,
		CreatedAt: now,
	}
	if err := s.store.PutUserAccessToken(r.Context(), rec); err != nil {
		s.logger.Error("persist access token", "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create access token")
		return
	}

	writeJSON(w, http.StatusCreated, buildCreatedAccessTokenResponse(rec))
}

func (s *Server) handleDeleteAccessToken(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeAPIError(w, http.StatusBadRequest, "access token id is required")
		return
	}
	if err := s.store.DeleteUserAccessToken(r.Context(), id); errors.Is(err, store.ErrUserAccessTokenNotFound) {
		writeAPIError(w, http.StatusNotFound, "access token not found")
		return
	} else if err != nil {
		s.logger.Error("delete access token", "access_token_id", id, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to delete access token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listTeamAPIKeys(ctx context.Context) ([]teamAPIKeyResponse, error) {
	bootstrap := bootstrapTeamAPIKeyResponse(s.cfg.APIKey, s.nowFunc())
	stored, err := s.store.ListTeamAPIKeys(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]teamAPIKeyResponse, 0, len(stored)+1)
	out = append(out, bootstrap)
	for _, rec := range stored {
		out = append(out, buildTeamAPIKeyResponse(rec, ""))
	}
	return out, nil
}

func (s *Server) validateAPIKey(ctx context.Context, provided string) (bool, error) {
	if provided == "" {
		return false, nil
	}
	if provided == s.cfg.APIKey {
		return true, nil
	}
	return s.store.ValidateTeamAPIKey(ctx, provided)
}

func bootstrapTeamAPIKeyResponse(cfgKey string, now time.Time) teamAPIKeyResponse {
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !now.IsZero() {
		createdAt = now.UTC()
	}
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte("bootstrap-api-key:"+cfgKey)).String()
	return teamAPIKeyResponse{
		ID:        id,
		Name:      "default",
		Mask:      maskSecret(cfgKey),
		CreatedAt: createdAt.Format(time.RFC3339),
		CreatedBy: nil,
		LastUsed:  nil,
	}
}

func buildTeamAPIKeyResponse(rec store.TeamAPIKeyRecord, raw string) teamAPIKeyResponse {
	mask := listedTeamAPIKeyMask()
	if raw != "" {
		mask = maskSecret(raw)
	}
	return teamAPIKeyResponse{
		ID:        rec.ID,
		Name:      rec.Name,
		Mask:      mask,
		CreatedAt: rec.CreatedAt.UTC().Format(time.RFC3339),
		CreatedBy: nil,
		LastUsed:  nil,
	}
}

func buildCreatedTeamAPIKeyResponse(rec store.TeamAPIKeyRecord, raw string) createdTeamAPIKeyResponse {
	base := buildTeamAPIKeyResponse(rec, raw)
	return createdTeamAPIKeyResponse{
		ID:        base.ID,
		Key:       raw,
		Mask:      base.Mask,
		Name:      base.Name,
		CreatedAt: base.CreatedAt,
		CreatedBy: nil,
		LastUsed:  nil,
	}
}

func buildCreatedAccessTokenResponse(rec store.UserAccessTokenRecord) createdAccessTokenResponse {
	return createdAccessTokenResponse{
		ID:        rec.ID,
		Name:      rec.Name,
		Token:     rec.Token,
		Mask:      maskSecret(rec.Token),
		CreatedAt: rec.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func maskSecret(raw string) identifierMaskResponse {
	n := len(raw)
	if n == 0 {
		return identifierMaskResponse{}
	}
	prefix := ""
	body := raw
	if idx := strings.Index(raw, "_"); idx >= 0 {
		prefix = raw[:idx+1]
		body = raw[idx+1:]
	}
	if n <= 8 {
		return identifierMaskResponse{
			Prefix:            prefix,
			ValueLength:       n,
			MaskedValuePrefix: "****",
			MaskedValueSuffix: "",
		}
	}
	show := 4
	if len(body) < show*2 {
		show = len(body) / 2
		if show < 1 {
			show = 1
		}
	}
	return identifierMaskResponse{
		Prefix:            prefix,
		ValueLength:       n,
		MaskedValuePrefix: prefix + body[:show],
		MaskedValueSuffix: body[len(body)-show:],
	}
}

func listedTeamAPIKeyMask() identifierMaskResponse {
	return identifierMaskResponse{
		Prefix:            "adk_",
		ValueLength:       40,
		MaskedValuePrefix: "adk_",
		MaskedValueSuffix: "****",
	}
}
