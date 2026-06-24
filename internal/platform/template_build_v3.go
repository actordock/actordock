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

type templateBuildRequestV3 struct {
	Name     string   `json:"name,omitempty"`
	Alias    string   `json:"alias,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	CPUCount int      `json:"cpuCount,omitempty"`
	MemoryMB int      `json:"memoryMB,omitempty"`
}

type templateRequestResponseV3 struct {
	TemplateID string   `json:"templateID"`
	BuildID    string   `json:"buildID"`
	Public     bool     `json:"public"`
	Aliases    []string `json:"aliases"`
	Names      []string `json:"names"`
	Tags       []string `json:"tags"`
}

func parseTemplateBuildName(name, alias string, explicitTags []string) (templateID string, tags []string, err error) {
	raw := strings.TrimSpace(name)
	if raw == "" {
		raw = strings.TrimSpace(alias)
	}
	if raw == "" {
		return "", nil, errors.New("name is required")
	}

	tags = append([]string(nil), explicitTags...)
	if idx := strings.LastIndex(raw, ":"); idx > 0 {
		tag := strings.TrimSpace(raw[idx+1:])
		if tag != "" {
			tags = appendUniqueTag(tags, tag)
		}
		raw = raw[:idx]
	}
	templateID = strings.TrimSpace(raw)
	if templateID == "" {
		return "", nil, errors.New("name is required")
	}
	return templateID, tags, nil
}

func appendUniqueTag(tags []string, tag string) []string {
	for _, existing := range tags {
		if existing == tag {
			return tags
		}
	}
	return append(tags, tag)
}

func (s *Server) handleCreateTemplateV3(w http.ResponseWriter, r *http.Request) {
	var req templateBuildRequestV3
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	templateID, tags, err := parseTemplateBuildName(req.Name, req.Alias, req.Tags)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	cpuCount := req.CPUCount
	if cpuCount == 0 {
		cpuCount = defaultCPUCount
	}
	memoryMB := req.MemoryMB
	if memoryMB == 0 {
		memoryMB = defaultMemoryMB
	}
	if cpuCount < 1 {
		writeAPIError(w, http.StatusBadRequest, "cpuCount must be at least 1")
		return
	}
	if memoryMB < 128 {
		writeAPIError(w, http.StatusBadRequest, "memoryMB must be at least 128")
		return
	}

	ctx := r.Context()
	if s.templateIDTaken(ctx, templateID) {
		writeAPIError(w, http.StatusConflict, "template already exists")
		return
	}

	bs, ok := s.store.(templateBuildPersistence)
	if !ok {
		s.logger.Error("create template v3", "err", "template build store unavailable")
		writeAPIError(w, http.StatusInternalServerError, "failed to create template")
		return
	}

	buildID := uuid.NewString()
	now := s.nowFunc().UTC()
	build := store.TemplateBuild{
		TemplateID:  templateID,
		BuildID:     buildID,
		Status:      store.TemplateBuildStatusBuilding,
		CPUCount:    cpuCount,
		MemoryMB:    memoryMB,
		Tags:        tags,
		Namespace:   s.cfg.TemplateNamespace,
		ActorName:   templateID,
		Public:      false,
		EnvdVersion: s.cfg.EnvdVersion,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := bs.PutTemplateBuild(ctx, build); err != nil {
		s.logger.Error("create template v3", "template_id", templateID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create template")
		return
	}

	names := []string{templateID}
	writeJSON(w, http.StatusAccepted, templateRequestResponseV3{
		TemplateID: templateID,
		BuildID:    buildID,
		Public:     false,
		Aliases:    append([]string(nil), names...),
		Names:      names,
		Tags:       tags,
	})
}

func (s *Server) templateIDTaken(ctx context.Context, templateID string) bool {
	_, err := s.templates.Get(ctx, templateID)
	return err == nil
}

func (s *Server) platformPublicURL(r *http.Request) string {
	if u := strings.TrimSpace(s.cfg.PlatformPublicURL); u != "" {
		return strings.TrimRight(u, "/")
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
