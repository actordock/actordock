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
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/actordock/actordock/internal/store"
)

func TestCreateSandboxWithTemplateTag(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	actors := &fakeActors{}
	st := newFakeStore()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	if err := st.PutTemplateBuild(context.Background(), store.TemplateBuild{
		TemplateID:  "my-app",
		BuildID:     "build-1",
		Status:      store.TemplateBuildStatusReady,
		PinnedImage: "registry.example/app@sha256:abc",
		CPUCount:    2,
		MemoryMB:    512,
		Namespace:   "actordock",
		ActorName:   "my-app",
		EnvdVersion: cfg.EnvdVersion,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("PutTemplateBuild: %v", err)
	}
	if err := st.PutTemplateTag(context.Background(), store.TemplateTagRecord{
		TemplateID: "my-app",
		Tag:        "prod",
		BuildID:    "build-1",
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("PutTemplateTag: %v", err)
	}

	srv := NewServerWithCatalog(cfg, actors, st, testTemplateCatalog(cfg), nil)
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader([]byte(`{"templateID":"my-app:prod","secure":false}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if actors.lastTemplateName != "my-app--prod" {
		t.Fatalf("template name = %q, want my-app--prod", actors.lastTemplateName)
	}
}

func TestAssignTemplateTagEnqueuesSync(t *testing.T) {
	t.Parallel()

	srv, st := testServerWithBuildFiles(t)
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	if err := st.PutTemplateBuild(context.Background(), store.TemplateBuild{
		TemplateID:  "my-app",
		BuildID:     "build-1",
		Status:      store.TemplateBuildStatusReady,
		PinnedImage: "registry.example/app@sha256:abc",
		Namespace:   "actordock",
		ActorName:   "my-app",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("PutTemplateBuild: %v", err)
	}

	body := `{"target":"my-app","tags":["prod"]}`
	req := httptest.NewRequest(http.MethodPost, "/templates/tags", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(st.templateBuildQueue) != 1 || st.templateBuildQueue[0].SyncTag != "prod" {
		t.Fatalf("queue = %+v", st.templateBuildQueue)
	}
}
