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
	"os"
	"strings"
	"time"

	"github.com/actordock/actordock/internal/config"
	"github.com/actordock/actordock/internal/store"
	"github.com/google/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type buildStore interface {
	GetTemplateBuild(ctx context.Context, templateID, buildID string) (store.TemplateBuild, error)
	UpdateTemplateBuild(ctx context.Context, build store.TemplateBuild) error
	AppendBuildLog(ctx context.Context, entry store.BuildLogEntry) error
	DequeueTemplateBuild(ctx context.Context) (store.TemplateBuildJob, error)
}

// Worker processes queued template builds.
type Worker struct {
	cfg       config.TemplateBuilder
	store     buildStore
	k8s       client.Client
	filesDir  string
	workDir   string
	builder   ImageBuilder
	templates *ActorTemplateManager
	waitReady func(ctx context.Context, name string) error
	now       func() time.Time
	logBuild  func(ctx context.Context, templateID, buildID, level, message, step string) error
}

func NewWorker(cfg config.TemplateBuilder, st buildStore, k8s client.Client, builder ImageBuilder) *Worker {
	w := &Worker{
		cfg:       cfg,
		store:     st,
		k8s:       k8s,
		filesDir:  cfg.TemplateBuildFilesDir,
		workDir:   cfg.BuildWorkDir,
		builder:   builder,
		templates: &ActorTemplateManager{Client: k8s, Namespace: cfg.TemplateNamespace},
		now:       time.Now,
		logBuild: func(ctx context.Context, templateID, buildID, level, message, step string) error {
			return st.AppendBuildLog(ctx, store.BuildLogEntry{
				TemplateID: templateID,
				BuildID:    buildID,
				Timestamp:  time.Now().UTC(),
				Level:      level,
				Message:    message,
				Step:       step,
			})
		},
	}
	w.waitReady = w.templates.WaitReady
	return w
}

func (w *Worker) Run(ctx context.Context) error {
	for {
		job, err := w.store.DequeueTemplateBuild(ctx)
		if err != nil {
			return err
		}
		if err := w.Process(ctx, job); err != nil {
			_ = err
		}
	}
}

func (w *Worker) Process(ctx context.Context, job store.TemplateBuildJob) error {
	if strings.TrimSpace(job.SyncTag) != "" {
		return w.processTagSync(ctx, job)
	}

	build, err := w.store.GetTemplateBuild(ctx, job.TemplateID, job.BuildID)
	if err != nil {
		return err
	}

	w.appendLog(ctx, job.TemplateID, job.BuildID, "info", "starting template build", "build")
	if err := w.markStatus(ctx, &build, store.TemplateBuildStatusBuilding, ""); err != nil {
		return err
	}

	pinnedImage, processErr := w.process(ctx, job, build)
	if processErr != nil {
		w.appendLog(ctx, job.TemplateID, job.BuildID, "error", processErr.Error(), "build")
		now := w.now().UTC()
		build.Status = store.TemplateBuildStatusError
		build.ErrorMessage = processErr.Error()
		build.UpdatedAt = now
		build.FinishedAt = &now
		_ = w.store.UpdateTemplateBuild(ctx, build)
		return processErr
	}

	now := w.now().UTC()
	build.Status = store.TemplateBuildStatusReady
	build.ErrorMessage = ""
	build.PinnedImage = pinnedImage
	build.UpdatedAt = now
	build.FinishedAt = &now
	w.appendLog(ctx, job.TemplateID, job.BuildID, "info", "template build ready", "build")
	return w.store.UpdateTemplateBuild(ctx, build)
}

func (w *Worker) process(ctx context.Context, job store.TemplateBuildJob, build store.TemplateBuild) (string, error) {
	spec, err := ParseStartSpec(build.StepsJSON)
	if err != nil {
		return "", err
	}

	baseAT, baseEnvd, err := w.loadBase(ctx, build)
	if err != nil {
		return "", err
	}

	fromImage := strings.TrimSpace(spec.FromImage)
	if fromImage == "" {
		fromImage = baseEnvd.Image
	}

	dockerfile, err := SynthesizeDockerfile(fromImage, spec.Steps)
	if err != nil {
		return "", err
	}

	contextDir := fmt.Sprintf("%s/%s", w.workDir, job.BuildID)
	if err := os.MkdirAll(w.workDir, 0o755); err != nil {
		return "", err
	}
	if err := os.RemoveAll(contextDir); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	defer func() { _ = os.RemoveAll(contextDir) }()

	w.appendLog(ctx, job.TemplateID, job.BuildID, "info", "preparing build context", "build")
	if err := PrepareContextDir(contextDir, dockerfile, w.filesDir, spec.Steps); err != nil {
		return "", err
	}

	imageTag := sanitizeImageTag(job.BuildID)
	if imageTag == "" {
		imageTag = uuid.NewString()
	}
	destination := fmt.Sprintf("%s/actordock/templates/%s:%s", strings.TrimRight(w.cfg.BuildRegistry, "/"), job.TemplateID, imageTag)

	w.appendLog(ctx, job.TemplateID, job.BuildID, "info", "running image build", "build")
	pinnedImage, err := w.builder.Build(ctx, BuildRequest{
		ContextDir:  contextDir,
		Destination: destination,
		BuildID:     job.BuildID,
	})
	if err != nil {
		return "", err
	}

	w.appendLog(ctx, job.TemplateID, job.BuildID, "info", "creating ActorTemplate", "build")
	if err := w.templates.Replace(ctx, job.TemplateID, pinnedImage, baseAT, baseEnvd); err != nil {
		return "", err
	}

	w.appendLog(ctx, job.TemplateID, job.BuildID, "info", "waiting for golden snapshot", "build")
	if err := w.waitReady(ctx, job.TemplateID); err != nil {
		return "", err
	}

	w.appendLog(ctx, job.TemplateID, job.BuildID, "info", "syncing template tags", "build")
	if err := w.syncBuildTags(ctx, job.TemplateID, job.BuildID, build, pinnedImage, baseAT, baseEnvd); err != nil {
		return "", err
	}
	return pinnedImage, nil
}

func (w *Worker) markStatus(ctx context.Context, build *store.TemplateBuild, status store.TemplateBuildStatus, message string) error {
	build.Status = status
	build.ErrorMessage = message
	build.UpdatedAt = w.now().UTC()
	return w.store.UpdateTemplateBuild(ctx, *build)
}

func (w *Worker) appendLog(ctx context.Context, templateID, buildID, level, message, step string) {
	_ = w.logBuild(ctx, templateID, buildID, level, message, step)
}

func sanitizeImageTag(buildID string) string {
	tag := strings.ToLower(strings.TrimSpace(buildID))
	tag = strings.NewReplacer(":", "-", "/", "-", "_", "-").Replace(tag)
	if len(tag) > 128 {
		tag = tag[:128]
	}
	return tag
}
