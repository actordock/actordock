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
	"errors"
	"fmt"

	"github.com/actordock/actordock/internal/store"
	"github.com/actordock/actordock/internal/templateref"
)

var ErrTemplateTagNotReady = errors.New("template tag is not ready")

func (s *Server) resolveSandboxTemplate(ctx context.Context, templateRef string) (CatalogTemplate, error) {
	templateID, tag, err := templateref.Parse(templateRef)
	if err != nil {
		return CatalogTemplate{}, fmt.Errorf("invalid template reference: %w", err)
	}
	if tag == "" {
		return s.templates.Get(ctx, templateID)
	}
	return s.resolveTaggedTemplate(ctx, templateID, tag)
}

func (s *Server) resolveTaggedTemplate(ctx context.Context, templateID, tag string) (CatalogTemplate, error) {
	ts, ok := s.store.(templateTagStore)
	if !ok {
		return CatalogTemplate{}, errors.New("template tag store unavailable")
	}
	bs, ok := s.store.(templateBuildPersistence)
	if !ok {
		return CatalogTemplate{}, errors.New("template build store unavailable")
	}

	sanitized := templateref.SanitizeTagName(tag)
	if sanitized == "" {
		return CatalogTemplate{}, fmt.Errorf("invalid template tag %q", tag)
	}

	rec, err := ts.GetTemplateTag(ctx, templateID, sanitized)
	if errors.Is(err, store.ErrTemplateTagNotFound) {
		// Fall back to tags on the latest ready build metadata.
		build, latestErr := bs.GetLatestTemplateBuild(ctx, templateID)
		if latestErr != nil {
			return CatalogTemplate{}, ErrTemplateNotFound
		}
		if !tagOnBuild(build, sanitized) {
			return CatalogTemplate{}, ErrTemplateNotFound
		}
		rec = store.TemplateTagRecord{TemplateID: templateID, Tag: sanitized, BuildID: build.BuildID}
	} else if err != nil {
		return CatalogTemplate{}, err
	}

	build, err := bs.GetTemplateBuild(ctx, templateID, rec.BuildID)
	if errors.Is(err, store.ErrTemplateBuildNotFound) {
		return CatalogTemplate{}, ErrTemplateNotFound
	}
	if err != nil {
		return CatalogTemplate{}, err
	}
	if build.Status != store.TemplateBuildStatusReady {
		return CatalogTemplate{}, ErrTemplateTagNotReady
	}

	base, err := s.templates.Get(ctx, templateID)
	if errors.Is(err, ErrTemplateNotFound) {
		base = catalogTemplateFromBuild(build)
	} else if err != nil {
		return CatalogTemplate{}, err
	}

	actorName := templateref.ActorNameForTag(templateID, sanitized)
	base.TemplateID = templateID
	base.Name = actorName
	base.Namespace = build.Namespace
	if base.Namespace == "" {
		base.Namespace = s.cfg.TemplateNamespace
	}
	base.BuildID = build.BuildID
	base.BuildStatus = string(build.Status)
	base.CPUCount = build.CPUCount
	base.MemoryMB = build.MemoryMB
	if build.Public {
		base.Public = build.Public
	}
	return base, nil
}

func tagOnBuild(build store.TemplateBuild, tag string) bool {
	for _, item := range build.Tags {
		if templateref.SanitizeTagName(item) == tag {
			return true
		}
	}
	return false
}

func (s *Server) enqueueTemplateTagSync(ctx context.Context, templateID, buildID, tag string) error {
	q, ok := s.store.(interface {
		EnqueueTemplateTagSync(ctx context.Context, templateID, buildID, tag string) error
	})
	if !ok {
		return nil
	}
	return q.EnqueueTemplateTagSync(ctx, templateID, buildID, tag)
}
