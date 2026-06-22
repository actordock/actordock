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

type templateResponse struct {
	TemplateID    string   `json:"templateID"`
	BuildID       string   `json:"buildID"`
	CPUCount      int      `json:"cpuCount"`
	MemoryMB      int      `json:"memoryMB"`
	DiskSizeMB    int      `json:"diskSizeMB"`
	Public        bool     `json:"public"`
	Aliases       []string `json:"aliases"`
	Names         []string `json:"names"`
	CreatedAt     string   `json:"createdAt"`
	UpdatedAt     string   `json:"updatedAt"`
	CreatedBy     any      `json:"createdBy"`
	LastSpawnedAt *string  `json:"lastSpawnedAt"`
	SpawnCount    int64    `json:"spawnCount"`
	BuildCount    int32    `json:"buildCount"`
	EnvdVersion   string   `json:"envdVersion"`
	BuildStatus   string   `json:"buildStatus"`
}

type templateBuildResponse struct {
	BuildID     string  `json:"buildID"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
	CPUCount    int     `json:"cpuCount"`
	MemoryMB    int     `json:"memoryMB"`
	DiskSizeMB  int     `json:"diskSizeMB,omitempty"`
	EnvdVersion string  `json:"envdVersion,omitempty"`
	FinishedAt  *string `json:"finishedAt,omitempty"`
}

type templateWithBuildsResponse struct {
	TemplateID    string                  `json:"templateID"`
	Public        bool                    `json:"public"`
	Aliases       []string                `json:"aliases"`
	Names         []string                `json:"names"`
	CreatedAt     string                  `json:"createdAt"`
	UpdatedAt     string                  `json:"updatedAt"`
	LastSpawnedAt *string                 `json:"lastSpawnedAt"`
	SpawnCount    int64                   `json:"spawnCount"`
	Builds        []templateBuildResponse `json:"builds"`
}

type templateAliasResponse struct {
	TemplateID string `json:"templateID"`
	Public     bool   `json:"public"`
}

type templateTagResponse struct {
	Tag       string `json:"tag"`
	BuildID   string `json:"buildID"`
	CreatedAt string `json:"createdAt"`
}

type templateBuildFileUploadResponse struct {
	Present bool    `json:"present"`
	URL     *string `json:"url,omitempty"`
}

type createTemplateRequest struct {
	Alias      string `json:"alias,omitempty"`
	Dockerfile string `json:"dockerfile"`
	TeamID     string `json:"teamID,omitempty"`
	StartCmd   string `json:"startCmd,omitempty"`
	ReadyCmd   string `json:"readyCmd,omitempty"`
	CPUCount   int    `json:"cpuCount,omitempty"`
	MemoryMB   int    `json:"memoryMB,omitempty"`
}

type patchTemplateRequest struct {
	Public *bool `json:"public,omitempty"`
}

type templateUpdateResponse struct {
	Names []string `json:"names"`
}

func buildTemplateUpdateResponse(tmpl CatalogTemplate) templateUpdateResponse {
	return templateUpdateResponse{
		Names: append([]string(nil), tmpl.Names...),
	}
}

func buildTemplateResponse(tmpl CatalogTemplate) templateResponse {
	return templateResponse{
		TemplateID:    tmpl.TemplateID,
		BuildID:       tmpl.BuildID,
		CPUCount:      tmpl.CPUCount,
		MemoryMB:      tmpl.MemoryMB,
		DiskSizeMB:    tmpl.DiskSizeMB,
		Public:        tmpl.Public,
		Aliases:       append([]string(nil), tmpl.Aliases...),
		Names:         append([]string(nil), tmpl.Names...),
		CreatedAt:     formatRFC3339(tmpl.CreatedAt),
		UpdatedAt:     formatRFC3339(tmpl.UpdatedAt),
		CreatedBy:     nil,
		LastSpawnedAt: nil,
		SpawnCount:    0,
		BuildCount:    0,
		EnvdVersion:   tmpl.EnvdVersion,
		BuildStatus:   "ready",
	}
}

func buildTemplateBuildResponse(tmpl CatalogTemplate) templateBuildResponse {
	finishedAt := formatRFC3339(tmpl.UpdatedAt)
	return templateBuildResponse{
		BuildID:     tmpl.BuildID,
		Status:      "ready",
		CreatedAt:   formatRFC3339(tmpl.CreatedAt),
		UpdatedAt:   formatRFC3339(tmpl.UpdatedAt),
		CPUCount:    tmpl.CPUCount,
		MemoryMB:    tmpl.MemoryMB,
		DiskSizeMB:  tmpl.DiskSizeMB,
		EnvdVersion: tmpl.EnvdVersion,
		FinishedAt:  &finishedAt,
	}
}

func buildTemplateWithBuildsResponse(tmpl CatalogTemplate) templateWithBuildsResponse {
	return templateWithBuildsResponse{
		TemplateID:    tmpl.TemplateID,
		Public:        tmpl.Public,
		Aliases:       append([]string(nil), tmpl.Aliases...),
		Names:         append([]string(nil), tmpl.Names...),
		CreatedAt:     formatRFC3339(tmpl.CreatedAt),
		UpdatedAt:     formatRFC3339(tmpl.UpdatedAt),
		LastSpawnedAt: nil,
		SpawnCount:    0,
		Builds:        []templateBuildResponse{buildTemplateBuildResponse(tmpl)},
	}
}

func (s *Server) handleTemplatePath(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(r.PathValue("path"), "/")
	if path == "" {
		writeAPIError(w, http.StatusNotFound, "template not found")
		return
	}
	parts := strings.Split(path, "/")

	switch {
	case len(parts) == 2 && parts[0] == "aliases":
		r.SetPathValue("alias", parts[1])
		s.handleGetTemplateAlias(w, r)
	case len(parts) == 2 && parts[1] == "tags":
		r.SetPathValue("templateID", parts[0])
		s.handleGetTemplateTags(w, r)
	case len(parts) == 3 && parts[1] == "files":
		r.SetPathValue("templateID", parts[0])
		r.SetPathValue("hash", parts[2])
		s.handleGetTemplateFileUpload(w, r)
	case len(parts) == 1:
		r.SetPathValue("templateID", parts[0])
		s.handleGetTemplate(w, r)
	default:
		writeAPIError(w, http.StatusNotFound, "template not found")
	}
}

func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	_ = r.URL.Query().Get("teamID")

	ctx := r.Context()
	templates, err := s.templates.List(ctx)
	if err != nil {
		s.logger.Error("list templates", "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to list templates")
		return
	}

	resp := make([]templateResponse, 0, len(templates))
	for _, tmpl := range templates {
		resp = append(resp, buildTemplateResponse(tmpl))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	var req createTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Dockerfile) == "" {
		writeAPIError(w, http.StatusBadRequest, "dockerfile is required")
		return
	}

	templateID := strings.TrimSpace(req.Alias)
	if templateID == "" {
		templateID = "tpl-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	}

	ctx := r.Context()
	tmpl, err := s.templates.Create(ctx, CreateTemplateInput{
		TemplateID: templateID,
		Alias:      req.Alias,
		Dockerfile: req.Dockerfile,
		StartCmd:   req.StartCmd,
		ReadyCmd:   req.ReadyCmd,
		CPUCount:   req.CPUCount,
		MemoryMB:   req.MemoryMB,
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrCatalogTemplateExists):
			writeAPIError(w, http.StatusConflict, "template already exists")
		case errors.Is(err, store.ErrCatalogTemplateDockerfile),
			strings.Contains(err.Error(), "cpuCount"),
			strings.Contains(err.Error(), "memoryMB"):
			writeAPIError(w, http.StatusBadRequest, err.Error())
		default:
			s.logger.Error("create template", "template_id", templateID, "err", err)
			writeAPIError(w, http.StatusInternalServerError, "failed to create template")
		}
		return
	}

	writeJSON(w, http.StatusCreated, buildTemplateResponse(tmpl))
}

func (s *Server) handlePatchTemplate(w http.ResponseWriter, r *http.Request) {
	templateID := strings.TrimSpace(r.PathValue("id"))
	if templateID == "" {
		writeAPIError(w, http.StatusBadRequest, "template id is required")
		return
	}

	var req patchTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	ctx := r.Context()
	tmpl, err := s.templates.Update(ctx, templateID, req.Public)
	if errors.Is(err, ErrTemplateNotFound) {
		writeAPIError(w, http.StatusNotFound, "template not found")
		return
	}
	if err != nil {
		s.logger.Error("update template", "template_id", templateID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to update template")
		return
	}

	writeJSON(w, http.StatusOK, buildTemplateUpdateResponse(tmpl))
}

func (s *Server) handleGetTemplate(w http.ResponseWriter, r *http.Request) {
	templateID := strings.TrimSpace(r.PathValue("templateID"))
	if templateID == "" {
		writeAPIError(w, http.StatusBadRequest, "template id is required")
		return
	}
	_ = r.URL.Query().Get("nextToken")
	_ = r.URL.Query().Get("limit")

	ctx := r.Context()
	tmpl, err := s.templates.Get(ctx, templateID)
	if err != nil {
		if errors.Is(err, ErrTemplateNotFound) {
			writeAPIError(w, http.StatusNotFound, "template not found")
			return
		}
		s.logger.Error("get template", "template_id", templateID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to get template")
		return
	}

	writeJSON(w, http.StatusOK, buildTemplateWithBuildsResponse(tmpl))
}

func (s *Server) handleGetTemplateAlias(w http.ResponseWriter, r *http.Request) {
	alias := strings.TrimSpace(r.PathValue("alias"))
	if alias == "" {
		writeAPIError(w, http.StatusBadRequest, "alias is required")
		return
	}

	ctx := r.Context()
	tmpl, err := s.templates.ResolveAlias(ctx, alias)
	if err != nil {
		if errors.Is(err, ErrTemplateNotFound) {
			writeAPIError(w, http.StatusNotFound, "template not found")
			return
		}
		s.logger.Error("resolve template alias", "alias", alias, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to resolve template alias")
		return
	}

	writeJSON(w, http.StatusOK, templateAliasResponse{
		TemplateID: tmpl.TemplateID,
		Public:     tmpl.Public,
	})
}

func (s *Server) handleGetTemplateTags(w http.ResponseWriter, r *http.Request) {
	templateID := strings.TrimSpace(r.PathValue("templateID"))
	if templateID == "" {
		writeAPIError(w, http.StatusBadRequest, "template id is required")
		return
	}

	ctx := r.Context()
	if _, err := s.templates.Get(ctx, templateID); err != nil {
		if errors.Is(err, ErrTemplateNotFound) {
			writeAPIError(w, http.StatusNotFound, "template not found")
			return
		}
		s.logger.Error("get template tags", "template_id", templateID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to get template tags")
		return
	}

	writeJSON(w, http.StatusOK, []templateTagResponse{})
}

func (s *Server) handleGetTemplateFileUpload(w http.ResponseWriter, r *http.Request) {
	templateID := strings.TrimSpace(r.PathValue("templateID"))
	hash := strings.TrimSpace(r.PathValue("hash"))
	if templateID == "" || hash == "" {
		writeAPIError(w, http.StatusBadRequest, "template id and hash are required")
		return
	}

	ctx := r.Context()
	if _, err := s.templates.Get(ctx, templateID); err != nil {
		if errors.Is(err, ErrTemplateNotFound) {
			writeAPIError(w, http.StatusNotFound, "template not found")
			return
		}
		s.logger.Error("get template file upload", "template_id", templateID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to get template file upload")
		return
	}

	writeJSON(w, http.StatusCreated, templateBuildFileUploadResponse{
		Present: true,
	})
}

func (s *Server) templateAlias(ctx context.Context, templateID string) string {
	tmpl, err := s.templates.Get(ctx, templateID)
	if err != nil || len(tmpl.Aliases) == 0 {
		return ""
	}
	return tmpl.Aliases[0]
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
