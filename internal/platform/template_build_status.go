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
	"strconv"
	"strings"

	"github.com/actordock/actordock/internal/store"
)

type buildLogEntryResponse struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
	Level     string `json:"level"`
	Step      string `json:"step,omitempty"`
}

type buildStatusReasonResponse struct {
	Message    string                  `json:"message"`
	Step       string                  `json:"step,omitempty"`
	LogEntries []buildLogEntryResponse `json:"logEntries,omitempty"`
}

type templateBuildInfoResponse struct {
	TemplateID string                     `json:"templateID"`
	BuildID    string                     `json:"buildID"`
	Status     string                     `json:"status"`
	Logs       []string                   `json:"logs"`
	LogEntries []buildLogEntryResponse    `json:"logEntries"`
	Reason     *buildStatusReasonResponse `json:"reason,omitempty"`
}

type templateBuildLogsResponse struct {
	Logs []buildLogEntryResponse `json:"logs"`
}

func (s *Server) handleGetTemplateBuildStatus(w http.ResponseWriter, r *http.Request) {
	templateID := strings.TrimSpace(r.PathValue("templateID"))
	buildID := strings.TrimSpace(r.PathValue("buildID"))
	if templateID == "" || buildID == "" {
		writeAPIError(w, http.StatusBadRequest, "template id and build id are required")
		return
	}

	offset, limit := parseLogsPagination(r)
	level := strings.TrimSpace(r.URL.Query().Get("level"))

	bs, ok := s.store.(templateBuildPersistence)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "template build store unavailable")
		return
	}

	ctx := r.Context()
	build, err := bs.GetTemplateBuild(ctx, templateID, buildID)
	if errors.Is(err, store.ErrTemplateBuildNotFound) {
		writeAPIError(w, http.StatusNotFound, "template build not found")
		return
	}
	if err != nil {
		s.logger.Error("get template build status", "template_id", templateID, "build_id", buildID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to get template build status")
		return
	}

	entries, err := bs.ListBuildLogs(ctx, templateID, buildID, offset, limit)
	if err != nil {
		s.logger.Error("list template build logs", "template_id", templateID, "build_id", buildID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to get template build status")
		return
	}
	entries = filterBuildLogsByLevel(entries, level)

	resp := buildTemplateBuildInfoResponse(build, entries)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetTemplateBuildLogs(w http.ResponseWriter, r *http.Request) {
	templateID := strings.TrimSpace(r.PathValue("templateID"))
	buildID := strings.TrimSpace(r.PathValue("buildID"))
	if templateID == "" || buildID == "" {
		writeAPIError(w, http.StatusBadRequest, "template id and build id are required")
		return
	}

	limit := parseLimitQuery(r, 100)
	level := strings.TrimSpace(r.URL.Query().Get("level"))
	_ = r.URL.Query().Get("cursor")
	_ = r.URL.Query().Get("direction")
	_ = r.URL.Query().Get("source")

	bs, ok := s.store.(templateBuildPersistence)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "template build store unavailable")
		return
	}

	ctx := r.Context()
	if _, err := bs.GetTemplateBuild(ctx, templateID, buildID); errors.Is(err, store.ErrTemplateBuildNotFound) {
		writeAPIError(w, http.StatusNotFound, "template build not found")
		return
	} else if err != nil {
		s.logger.Error("get template build logs", "template_id", templateID, "build_id", buildID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to get template build logs")
		return
	}

	entries, err := bs.ListBuildLogs(ctx, templateID, buildID, 0, limit)
	if err != nil {
		s.logger.Error("list template build logs", "template_id", templateID, "build_id", buildID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to get template build logs")
		return
	}
	entries = filterBuildLogsByLevel(entries, level)

	resp := templateBuildLogsResponse{Logs: make([]buildLogEntryResponse, 0, len(entries))}
	for _, entry := range entries {
		resp.Logs = append(resp.Logs, buildLogEntryResponseFromStore(entry))
	}
	writeJSON(w, http.StatusOK, resp)
}

func buildTemplateBuildInfoResponse(build store.TemplateBuild, entries []store.BuildLogEntry) templateBuildInfoResponse {
	resp := templateBuildInfoResponse{
		TemplateID: build.TemplateID,
		BuildID:    build.BuildID,
		Status:     string(build.Status),
		Logs:       make([]string, 0, len(entries)),
		LogEntries: make([]buildLogEntryResponse, 0, len(entries)),
	}
	for _, entry := range entries {
		resp.Logs = append(resp.Logs, entry.Message)
		resp.LogEntries = append(resp.LogEntries, buildLogEntryResponseFromStore(entry))
	}
	if build.Status == store.TemplateBuildStatusError && strings.TrimSpace(build.ErrorMessage) != "" {
		resp.Reason = &buildStatusReasonResponse{Message: build.ErrorMessage}
	}
	return resp
}

func buildLogEntryResponseFromStore(entry store.BuildLogEntry) buildLogEntryResponse {
	return buildLogEntryResponse{
		Timestamp: formatRFC3339(entry.Timestamp),
		Message:   entry.Message,
		Level:     entry.Level,
		Step:      entry.Step,
	}
}

func parseLogsPagination(r *http.Request) (offset, limit int) {
	offset = parseIntQuery(r, "logsOffset", 0)
	limit = parseLimitQuery(r, 100)
	return offset, limit
}

func parseLimitQuery(r *http.Request, defaultLimit int) int {
	limit := parseIntQuery(r, "limit", defaultLimit)
	if limit < 0 {
		return defaultLimit
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func parseIntQuery(r *http.Request, key string, defaultValue int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return defaultValue
	}
	return n
}

func filterBuildLogsByLevel(entries []store.BuildLogEntry, level string) []store.BuildLogEntry {
	level = strings.TrimSpace(strings.ToLower(level))
	if level == "" {
		return entries
	}
	out := make([]store.BuildLogEntry, 0, len(entries))
	for _, entry := range entries {
		if strings.EqualFold(entry.Level, level) {
			out = append(out, entry)
		}
	}
	return out
}

func (s *Server) patchTemplatePublic(ctx context.Context, templateID string, public *bool) (CatalogTemplate, error) {
	if public == nil {
		return s.templates.Get(ctx, templateID)
	}
	tmpl, err := s.templates.Update(ctx, templateID, public)
	if err == nil {
		return tmpl, nil
	}
	if !errors.Is(err, ErrTemplateNotFound) && !errors.Is(err, store.ErrCatalogTemplateNotFound) {
		return CatalogTemplate{}, err
	}
	bs, ok := s.store.(templateBuildPersistence)
	if !ok {
		return CatalogTemplate{}, ErrTemplateNotFound
	}
	build, err := bs.GetLatestTemplateBuild(ctx, templateID)
	if errors.Is(err, store.ErrTemplateBuildNotFound) {
		return CatalogTemplate{}, ErrTemplateNotFound
	}
	if err != nil {
		return CatalogTemplate{}, err
	}
	build.Public = *public
	build.UpdatedAt = s.nowFunc().UTC()
	if err := bs.UpdateTemplateBuild(ctx, build); err != nil {
		return CatalogTemplate{}, err
	}
	return s.templates.Get(ctx, templateID)
}

func (s *Server) handlePatchTemplateV2(w http.ResponseWriter, r *http.Request) {
	templateID := strings.TrimSpace(r.PathValue("templateID"))
	if templateID == "" {
		writeAPIError(w, http.StatusBadRequest, "template id is required")
		return
	}

	var req patchTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	tmpl, err := s.patchTemplatePublic(r.Context(), templateID, req.Public)
	if errors.Is(err, ErrTemplateNotFound) {
		writeAPIError(w, http.StatusNotFound, "template not found")
		return
	}
	if err != nil {
		s.logger.Error("patch template v2", "template_id", templateID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to update template")
		return
	}

	writeJSON(w, http.StatusOK, buildTemplateUpdateResponse(tmpl))
}
