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

package templatebuild

import (
	"context"
	"fmt"
	"strings"

	"github.com/actordock/actordock/internal/store"
	"github.com/actordock/actordock/internal/templateref"
	v1alpha1 "github.com/actordock/runtime/pkg/api/v1alpha1"
)

type tagStore interface {
	PutTemplateTag(ctx context.Context, rec store.TemplateTagRecord) error
	ListTemplateTags(ctx context.Context, templateID string) ([]store.TemplateTagRecord, error)
}

func (w *Worker) processTagSync(ctx context.Context, job store.TemplateBuildJob) error {
	build, err := w.store.GetTemplateBuild(ctx, job.TemplateID, job.BuildID)
	if err != nil {
		return err
	}
	if build.Status != store.TemplateBuildStatusReady {
		return fmt.Errorf("template build %s/%s is not ready", job.TemplateID, job.BuildID)
	}
	pinnedImage := strings.TrimSpace(build.PinnedImage)
	if pinnedImage == "" {
		return fmt.Errorf("template build %s/%s has no pinned image", job.TemplateID, job.BuildID)
	}

	baseAT, baseEnvd, err := w.loadBase(ctx, build)
	if err != nil {
		return err
	}

	tag := strings.TrimSpace(job.SyncTag)
	w.appendLog(ctx, job.TemplateID, job.BuildID, "info", "syncing tag "+tag, "build")
	return w.materializeTag(ctx, job.TemplateID, job.BuildID, tag, pinnedImage, baseAT, baseEnvd)
}

func (w *Worker) syncBuildTags(ctx context.Context, templateID, buildID string, build store.TemplateBuild, pinnedImage string, baseAT *v1alpha1.ActorTemplate, baseEnvd envdContainer) error {
	for _, tag := range w.collectTagsForBuild(ctx, templateID, buildID, build) {
		if err := w.materializeTag(ctx, templateID, buildID, tag, pinnedImage, baseAT, baseEnvd); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) materializeTag(ctx context.Context, templateID, buildID, tag, pinnedImage string, baseAT *v1alpha1.ActorTemplate, baseEnvd envdContainer) error {
	tag = templateref.SanitizeTagName(tag)
	if tag == "" {
		return nil
	}
	now := w.now().UTC()
	if ts, ok := w.store.(tagStore); ok {
		if err := ts.PutTemplateTag(ctx, store.TemplateTagRecord{
			TemplateID: templateID,
			Tag:        tag,
			BuildID:    buildID,
			CreatedAt:  now,
		}); err != nil {
			return err
		}
	}
	actorName := templateref.ActorNameForTag(templateID, tag)
	if err := w.templates.Replace(ctx, actorName, pinnedImage, baseAT, baseEnvd); err != nil {
		return fmt.Errorf("replace actortemplate %s: %w", actorName, err)
	}
	if err := w.waitReady(ctx, actorName); err != nil {
		return fmt.Errorf("wait for actortemplate %s: %w", actorName, err)
	}
	return nil
}

func (w *Worker) collectTagsForBuild(ctx context.Context, templateID, buildID string, build store.TemplateBuild) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(tag string) {
		tag = templateref.SanitizeTagName(tag)
		if tag == "" {
			return
		}
		if _, ok := seen[tag]; ok {
			return
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	for _, tag := range build.Tags {
		add(tag)
	}
	if ts, ok := w.store.(tagStore); ok {
		recs, err := ts.ListTemplateTags(ctx, templateID)
		if err == nil {
			for _, rec := range recs {
				if rec.BuildID == buildID {
					add(rec.Tag)
				}
			}
		}
	}
	return out
}

func (w *Worker) loadBase(ctx context.Context, build store.TemplateBuild) (*v1alpha1.ActorTemplate, envdContainer, error) {
	baseName := strings.TrimSpace(build.FromTemplate)
	if baseName == "" {
		baseName = w.cfg.DefaultBaseTemplate
	}
	baseAT, err := loadActorTemplate(ctx, w.k8s, w.cfg.TemplateNamespace, baseName)
	if err != nil {
		return nil, envdContainer{}, err
	}
	baseEnvd, err := envdFromActorTemplate(baseAT)
	if err != nil {
		return nil, envdContainer{}, err
	}
	return baseAT, baseEnvd, nil
}
