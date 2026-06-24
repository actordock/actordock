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
	"github.com/actordock/actordock/internal/templateref"
)

type assignTemplateTagsRequest struct {
	Target string   `json:"target"`
	Tags   []string `json:"tags"`
}

type assignedTemplateTagsResponse struct {
	Tags    []string `json:"tags"`
	BuildID string   `json:"buildID"`
}

type deleteTemplateTagsRequest struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type templateTagStore interface {
	PutTemplateTag(ctx context.Context, rec store.TemplateTagRecord) error
	GetTemplateTag(ctx context.Context, templateID, tag string) (store.TemplateTagRecord, error)
	ListTemplateTags(ctx context.Context, templateID string) ([]store.TemplateTagRecord, error)
	DeleteTemplateTags(ctx context.Context, templateID string, tags []string) error
}

func (s *Server) handleAssignTemplateTags(w http.ResponseWriter, r *http.Request) {
	var req assignTemplateTagsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	targetID, sourceTag, err := templateref.Parse(req.Target)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Tags) == 0 {
		writeAPIError(w, http.StatusBadRequest, "tags are required")
		return
	}

	ts, ok := s.store.(templateTagStore)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "template tag store unavailable")
		return
	}
	bs, ok := s.store.(templateBuildPersistence)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "template build store unavailable")
		return
	}

	ctx := r.Context()
	buildID, err := s.resolveTagTargetBuild(ctx, ts, bs, targetID, sourceTag)
	if errors.Is(err, store.ErrTemplateBuildNotFound) || errors.Is(err, store.ErrTemplateTagNotFound) {
		writeAPIError(w, http.StatusNotFound, "template build not found")
		return
	}
	if err != nil {
		s.logger.Error("assign template tags", "target", req.Target, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to assign template tags")
		return
	}

	now := s.nowFunc().UTC()
	assigned := make([]string, 0, len(req.Tags))
	for _, tag := range req.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			writeAPIError(w, http.StatusBadRequest, "tags are required")
			return
		}
		sanitized := templateref.SanitizeTagName(tag)
		if sanitized == "" {
			writeAPIError(w, http.StatusBadRequest, "tags are required")
			return
		}
		if err := ts.PutTemplateTag(ctx, store.TemplateTagRecord{
			TemplateID: targetID,
			Tag:        sanitized,
			BuildID:    buildID,
			CreatedAt:  now,
		}); err != nil {
			s.logger.Error("assign template tag", "template_id", targetID, "tag", tag, "err", err)
			writeAPIError(w, http.StatusInternalServerError, "failed to assign template tags")
			return
		}
		build, err := bs.GetTemplateBuild(ctx, targetID, buildID)
		if err == nil && build.Status == store.TemplateBuildStatusReady && strings.TrimSpace(build.PinnedImage) != "" {
			if err := s.enqueueTemplateTagSync(ctx, targetID, buildID, sanitized); err != nil {
				s.logger.Error("enqueue template tag sync", "template_id", targetID, "tag", sanitized, "err", err)
				writeAPIError(w, http.StatusInternalServerError, "failed to assign template tags")
				return
			}
		}
		assigned = append(assigned, sanitized)
	}

	writeJSON(w, http.StatusCreated, assignedTemplateTagsResponse{
		Tags:    assigned,
		BuildID: buildID,
	})
}

func (s *Server) handleDeleteTemplateTags(w http.ResponseWriter, r *http.Request) {
	var req deleteTemplateTagsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	templateID := strings.TrimSpace(req.Name)
	if templateID == "" || len(req.Tags) == 0 {
		writeAPIError(w, http.StatusBadRequest, "name and tags are required")
		return
	}

	ts, ok := s.store.(templateTagStore)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "template tag store unavailable")
		return
	}
	if err := ts.DeleteTemplateTags(r.Context(), templateID, req.Tags); err != nil {
		if errors.Is(err, store.ErrTemplateTagEmpty) {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.logger.Error("delete template tags", "template_id", templateID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to delete template tags")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resolveTagTargetBuild(ctx context.Context, ts templateTagStore, bs templateBuildPersistence, templateID, sourceTag string) (string, error) {
	if sourceTag != "" {
		sourceTag = templateref.SanitizeTagName(sourceTag)
		rec, err := ts.GetTemplateTag(ctx, templateID, sourceTag)
		if err != nil {
			return "", err
		}
		if _, err := bs.GetTemplateBuild(ctx, templateID, rec.BuildID); err != nil {
			return "", err
		}
		return rec.BuildID, nil
	}
	build, err := bs.GetLatestTemplateBuild(ctx, templateID)
	if err != nil {
		return "", err
	}
	return build.BuildID, nil
}

func (s *Server) listTemplateTags(ctx context.Context, templateID string) ([]templateTagResponse, error) {
	ts, ok := s.store.(templateTagStore)
	if !ok {
		return nil, errors.New("template tag store unavailable")
	}
	recs, err := ts.ListTemplateTags(ctx, templateID)
	if err != nil {
		return nil, err
	}
	if len(recs) > 0 {
		return templateTagResponsesFromRecords(recs), nil
	}
	bs, ok := s.store.(templateBuildPersistence)
	if !ok {
		return []templateTagResponse{}, nil
	}
	build, err := bs.GetLatestTemplateBuild(ctx, templateID)
	if errors.Is(err, store.ErrTemplateBuildNotFound) {
		return []templateTagResponse{}, nil
	}
	if err != nil {
		return nil, err
	}
	return templateTagResponsesFromBuild(build), nil
}

func templateTagResponsesFromRecords(recs []store.TemplateTagRecord) []templateTagResponse {
	out := make([]templateTagResponse, 0, len(recs))
	for _, rec := range recs {
		out = append(out, templateTagResponse{
			Tag:       rec.Tag,
			BuildID:   rec.BuildID,
			CreatedAt: formatRFC3339(rec.CreatedAt),
		})
	}
	return out
}

func templateTagResponsesFromBuild(build store.TemplateBuild) []templateTagResponse {
	if len(build.Tags) == 0 {
		return []templateTagResponse{}
	}
	createdAt := formatRFC3339(build.CreatedAt)
	out := make([]templateTagResponse, 0, len(build.Tags))
	for _, tag := range build.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		out = append(out, templateTagResponse{
			Tag:       tag,
			BuildID:   build.BuildID,
			CreatedAt: createdAt,
		})
	}
	return out
}
