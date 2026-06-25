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
	"encoding/json"
	"testing"
	"time"

	"github.com/actordock/actordock/internal/config"
	"github.com/actordock/actordock/internal/store"
	v1alpha1 "github.com/actordock/runtime/pkg/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type stubImageBuilder struct {
	pinned string
}

func (s *stubImageBuilder) Build(context.Context, BuildRequest) (string, error) {
	return s.pinned, nil
}

type memoryBuildStore struct {
	builds store.TemplateBuild
	logs   []store.BuildLogEntry
	tags   map[string]store.TemplateTagRecord
}

func tagKey(templateID, tag string) string {
	return templateID + "\x00" + tag
}

func (m *memoryBuildStore) GetTemplateBuild(_ context.Context, templateID, buildID string) (store.TemplateBuild, error) {
	if m.builds.TemplateID == templateID && m.builds.BuildID == buildID {
		return m.builds, nil
	}
	return store.TemplateBuild{}, store.ErrTemplateBuildNotFound
}

func (m *memoryBuildStore) UpdateTemplateBuild(_ context.Context, build store.TemplateBuild) error {
	m.builds = build
	return nil
}

func (m *memoryBuildStore) AppendBuildLog(_ context.Context, entry store.BuildLogEntry) error {
	m.logs = append(m.logs, entry)
	return nil
}

func (m *memoryBuildStore) DequeueTemplateBuild(context.Context) (store.TemplateBuildJob, error) {
	return store.TemplateBuildJob{}, context.Canceled
}

func (m *memoryBuildStore) PutTemplateTag(_ context.Context, rec store.TemplateTagRecord) error {
	if m.tags == nil {
		m.tags = make(map[string]store.TemplateTagRecord)
	}
	m.tags[tagKey(rec.TemplateID, rec.Tag)] = rec
	return nil
}

func (m *memoryBuildStore) ListTemplateTags(_ context.Context, templateID string) ([]store.TemplateTagRecord, error) {
	out := make([]store.TemplateTagRecord, 0)
	for _, rec := range m.tags {
		if rec.TemplateID == templateID {
			out = append(out, rec)
		}
	}
	return out, nil
}

func TestWorkerProcessBuild(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	spec := StartSpec{
		FromTemplate: "base",
		Steps:        []Step{{Type: "RUN", Args: []string{"echo hi"}}},
	}
	stepsJSON, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	mem := &memoryBuildStore{
		builds: store.TemplateBuild{
			TemplateID:   "custom-app",
			BuildID:      "build-1",
			Status:       store.TemplateBuildStatusWaiting,
			StepsJSON:    stepsJSON,
			CPUCount:     2,
			MemoryMB:     512,
			Namespace:    "actordock",
			ActorName:    "custom-app",
			FromTemplate: "base",
			Tags:         []string{"prod"},
			EnvdVersion:  "0.1.0",
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}

	base := &v1alpha1.ActorTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "base", Namespace: "actordock"},
		Spec: v1alpha1.ActorTemplateSpec{
			PauseImage: "registry.k8s.io/pause:3.10.2@sha256:abc",
			Containers: []v1alpha1.Container{{
				Name:    "envd",
				Image:   "kind-registry:5000/envd@sha256:base",
				Command: []string{"/ko-app/envd"},
			}},
			SnapshotsConfig: v1alpha1.SnapshotsConfig{Location: "gs://bucket/actordock/"},
		},
	}
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	k8s := fake.NewClientBuilder().WithScheme(scheme).WithObjects(base).Build()

	dir := t.TempDir()
	cfg := config.TemplateBuilder{
		TemplateNamespace:            "actordock",
		DefaultBaseTemplate:          "base",
		BuildRegistry:                "kind-registry:5000",
		LocalhostRegistryReplacement: "kind-registry:5000",
		LocalRegistryHost:            "localhost:5001",
		BuildDataDir:                 dir,
		TemplateBuildFilesDir:        dir + "/files",
		BuildWorkDir:                 dir + "/work",
	}
	worker := NewWorker(cfg, mem, k8s, &stubImageBuilder{
		pinned: "kind-registry:5000/actordock/templates/custom-app@sha256:deadbeef",
	})
	worker.waitReady = func(context.Context, string) error { return nil }

	if err := worker.Process(context.Background(), store.TemplateBuildJob{
		TemplateID: "custom-app",
		BuildID:    "build-1",
		EnqueuedAt: now,
	}); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if mem.builds.Status != store.TemplateBuildStatusReady {
		t.Fatalf("status = %q", mem.builds.Status)
	}
	if mem.builds.PinnedImage != "localhost:5001/actordock/templates/custom-app@sha256:deadbeef" {
		t.Fatalf("pinned image = %q", mem.builds.PinnedImage)
	}

	var created v1alpha1.ActorTemplate
	if err := k8s.Get(context.Background(), types.NamespacedName{Namespace: "actordock", Name: "custom-app"}, &created); err != nil {
		t.Fatalf("get created actortemplate: %v", err)
	}
	if created.Spec.Containers[0].Image != "localhost:5001/actordock/templates/custom-app@sha256:deadbeef" {
		t.Fatalf("image = %q", created.Spec.Containers[0].Image)
	}

	var tagged v1alpha1.ActorTemplate
	if err := k8s.Get(context.Background(), types.NamespacedName{Namespace: "actordock", Name: "custom-app--prod"}, &tagged); err != nil {
		t.Fatalf("get tagged actortemplate: %v", err)
	}
	if tagged.Spec.Containers[0].Image != "localhost:5001/actordock/templates/custom-app@sha256:deadbeef" {
		t.Fatalf("tagged image = %q", tagged.Spec.Containers[0].Image)
	}
}
