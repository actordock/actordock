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
	"fmt"
	"net/http"
	"strings"

	"github.com/actordock/actordock/internal/store"
)

type templateBuildStartV2 struct {
	FromImage         string              `json:"fromImage,omitempty"`
	FromTemplate      string              `json:"fromTemplate,omitempty"`
	FromImageRegistry json.RawMessage     `json:"fromImageRegistry,omitempty"`
	Force             bool                `json:"force,omitempty"`
	Steps             []templateBuildStep `json:"steps,omitempty"`
	StartCmd          string              `json:"startCmd,omitempty"`
	ReadyCmd          string              `json:"readyCmd,omitempty"`
}

type templateBuildStep struct {
	Type      string   `json:"type"`
	Args      []string `json:"args,omitempty"`
	FilesHash string   `json:"filesHash,omitempty"`
	Force     bool     `json:"force,omitempty"`
}

var supportedTemplateStepTypes = map[string]struct{}{
	"COPY":    {},
	"ENV":     {},
	"RUN":     {},
	"WORKDIR": {},
	"USER":    {},
}

func (s *Server) handleStartTemplateBuildV2(w http.ResponseWriter, r *http.Request) {
	templateID := strings.TrimSpace(r.PathValue("templateID"))
	buildID := strings.TrimSpace(r.PathValue("buildID"))
	if templateID == "" || buildID == "" {
		writeAPIError(w, http.StatusBadRequest, "template id and build id are required")
		return
	}

	var req templateBuildStartV2
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	bs, ok := s.store.(templateBuildPersistence)
	if !ok {
		s.logger.Error("start template build", "err", "template build store unavailable")
		writeAPIError(w, http.StatusInternalServerError, "failed to start template build")
		return
	}

	ctx := r.Context()
	build, err := bs.GetTemplateBuild(ctx, templateID, buildID)
	if errors.Is(err, store.ErrTemplateBuildNotFound) {
		writeAPIError(w, http.StatusNotFound, "template build not found")
		return
	}
	if err != nil {
		s.logger.Error("start template build", "template_id", templateID, "build_id", buildID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to start template build")
		return
	}

	if build.Status != store.TemplateBuildStatusBuilding && build.Status != store.TemplateBuildStatusWaiting {
		writeAPIError(w, http.StatusConflict, "template build is not startable")
		return
	}

	if err := s.validateTemplateBuildStart(ctx, bs, req); err != nil {
		s.failTemplateBuild(ctx, bs, build, err.Error())
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	stepsJSON, err := json.Marshal(req)
	if err != nil {
		s.logger.Error("marshal template build steps", "template_id", templateID, "build_id", buildID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to start template build")
		return
	}

	now := s.nowFunc().UTC()
	build.StepsJSON = stepsJSON
	build.FromTemplate = strings.TrimSpace(req.FromTemplate)
	build.FromImage = strings.TrimSpace(req.FromImage)
	build.StartCmd = req.StartCmd
	build.ReadyCmd = req.ReadyCmd
	build.Status = store.TemplateBuildStatusWaiting
	build.UpdatedAt = now
	build.ErrorMessage = ""

	if err := bs.UpdateTemplateBuild(ctx, build); err != nil {
		s.logger.Error("update template build", "template_id", templateID, "build_id", buildID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to start template build")
		return
	}

	if err := bs.EnqueueTemplateBuild(ctx, store.TemplateBuildJob{
		TemplateID: templateID,
		BuildID:    buildID,
		EnqueuedAt: now,
	}); err != nil {
		s.logger.Error("enqueue template build", "template_id", templateID, "build_id", buildID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to start template build")
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) validateTemplateBuildStart(ctx context.Context, bs templateBuildPersistence, req templateBuildStartV2) error {
	fromTemplate := strings.TrimSpace(req.FromTemplate)
	fromImage := strings.TrimSpace(req.FromImage)
	if fromTemplate == "" && fromImage == "" {
		return errors.New("fromTemplate or fromImage is required")
	}
	if fromTemplate != "" && fromImage != "" {
		return errors.New("fromTemplate and fromImage are mutually exclusive")
	}
	if len(req.FromImageRegistry) > 0 && string(req.FromImageRegistry) != "null" {
		return errors.New("custom image registries are not supported")
	}

	if fromTemplate != "" {
		if !s.isAllowedFromTemplate(ctx, fromTemplate) {
			return fmt.Errorf("fromTemplate %q is not a cluster base template with envd", fromTemplate)
		}
	}

	for i, step := range req.Steps {
		stepType := strings.ToUpper(strings.TrimSpace(step.Type))
		if _, ok := supportedTemplateStepTypes[stepType]; !ok {
			return fmt.Errorf("unsupported build step type %q at index %d", step.Type, i)
		}
		if stepType == "COPY" {
			hash := strings.TrimSpace(step.FilesHash)
			if hash == "" {
				return fmt.Errorf("COPY step at index %d requires filesHash", i)
			}
			file, err := bs.GetTemplateBuildFile(ctx, hash)
			if err != nil {
				return fmt.Errorf("COPY step at index %d references missing filesHash %q", i, hash)
			}
			if !file.Present {
				return fmt.Errorf("COPY step at index %d references filesHash %q that is not uploaded", i, hash)
			}
			if s.buildFiles != nil && !s.buildFiles.exists(hash) {
				return fmt.Errorf("COPY step at index %d references filesHash %q that is not uploaded", i, hash)
			}
		}
	}
	return nil
}

func (s *Server) isAllowedFromTemplate(ctx context.Context, name string) bool {
	for _, allowed := range s.cfg.OfficialBaseTemplates {
		if strings.EqualFold(strings.TrimSpace(allowed), strings.TrimSpace(name)) {
			_, err := s.templates.Get(ctx, name)
			return err == nil
		}
	}
	return false
}

func (s *Server) failTemplateBuild(ctx context.Context, bs templateBuildPersistence, build store.TemplateBuild, message string) {
	now := s.nowFunc().UTC()
	build.Status = store.TemplateBuildStatusError
	build.ErrorMessage = message
	build.UpdatedAt = now
	build.FinishedAt = &now
	if err := bs.UpdateTemplateBuild(ctx, build); err != nil {
		s.logger.Error("fail template build", "template_id", build.TemplateID, "build_id", build.BuildID, "err", err)
		return
	}
	_ = bs.AppendBuildLog(ctx, store.BuildLogEntry{
		TemplateID: build.TemplateID,
		BuildID:    build.BuildID,
		Timestamp:  now,
		Level:      "error",
		Message:    message,
	})
}
