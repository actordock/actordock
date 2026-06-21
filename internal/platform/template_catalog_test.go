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
	"testing"
	"time"

	"github.com/actordock/actordock/internal/config"
	v1alpha1 "github.com/agent-substrate/substrate/pkg/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStaticTemplateCatalogListGetAlias(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	created := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	catalog := newMemoryTemplateCatalog([]CatalogTemplate{
		catalogTemplateFromConfig(cfg, "base", created),
	})

	ctx := context.Background()
	list, err := catalog.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].TemplateID != "base" {
		t.Fatalf("list = %+v", list)
	}

	got, err := catalog.Get(ctx, "base")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.BuildID != stableTemplateBuildID("actordock", "base") {
		t.Fatalf("buildID = %q", got.BuildID)
	}

	byAlias, err := catalog.ResolveAlias(ctx, "base")
	if err != nil {
		t.Fatalf("ResolveAlias: %v", err)
	}
	if byAlias.TemplateID != "base" {
		t.Fatalf("alias resolve = %+v", byAlias)
	}

	if _, err := catalog.Get(ctx, "missing"); !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("Get missing = %v", err)
	}
	if _, err := catalog.ResolveAlias(ctx, "missing"); !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("ResolveAlias missing = %v", err)
	}
}

func TestMapActorTemplate(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	at := &v1alpha1.ActorTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "base",
			Namespace:         "actordock",
			CreationTimestamp: metav1.NewTime(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)),
		},
		Status: v1alpha1.ActorTemplateStatus{
			Phase: v1alpha1.PhaseReady,
		},
	}
	got := mapActorTemplate(cfg, at)
	if got.TemplateID != "base" || got.EnvdVersion != cfg.EnvdVersion || !got.Public {
		t.Fatalf("mapped = %+v", got)
	}
	if len(got.Aliases) != 1 || got.Aliases[0] != "base" {
		t.Fatalf("aliases = %+v", got.Aliases)
	}
}

func TestBuildTemplateListResponseFields(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	tmpl := catalogTemplateFromConfig(cfg, "base", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	resp := buildTemplateResponse(tmpl)
	if resp.TemplateID != "base" || resp.BuildStatus != "ready" || !resp.Public {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.CreatedBy != nil {
		t.Fatalf("createdBy = %+v, want null", resp.CreatedBy)
	}
	if resp.LastSpawnedAt != nil {
		t.Fatalf("lastSpawnedAt = %+v, want null", resp.LastSpawnedAt)
	}
}

func TestBuildTemplateWithBuildsResponse(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	tmpl := catalogTemplateFromConfig(cfg, "base", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	resp := buildTemplateWithBuildsResponse(tmpl)
	if len(resp.Builds) != 1 || resp.Builds[0].Status != "ready" {
		t.Fatalf("builds = %+v", resp.Builds)
	}
	if resp.LastSpawnedAt != nil {
		t.Fatalf("lastSpawnedAt = %+v, want null", resp.LastSpawnedAt)
	}
}

func testTemplateCatalog(cfg config.Platform) TemplateCatalog {
	return newMemoryTemplateCatalog([]CatalogTemplate{
		catalogTemplateFromConfig(cfg, cfg.TemplateName, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)),
	})
}
