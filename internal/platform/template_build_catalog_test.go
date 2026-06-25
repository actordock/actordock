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
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/actordock/actordock/internal/store"
)

func TestWritableCatalogMapsUserTemplateToActorName(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	st := newFakeStore()
	catalog := NewWritableTemplateCatalog(cfg, testTemplateCatalog(cfg), st)
	ctx := context.Background()

	tmpl, err := catalog.Create(ctx, CreateTemplateInput{
		TemplateID: "myapp",
		Alias:      "myapp",
		Dockerfile: "FROM actordock/base",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tmpl.Name != "myapp" {
		t.Fatalf("Name = %q, want myapp", tmpl.Name)
	}
}

func TestWritableCatalogMergesLatestBuildStatus(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	st := newFakeStore()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	if err := st.PutTemplateBuild(context.Background(), store.TemplateBuild{
		TemplateID:  "custom-tpl",
		BuildID:     "build-abc",
		Status:      store.TemplateBuildStatusBuilding,
		CPUCount:    2,
		MemoryMB:    512,
		Namespace:   "actordock",
		ActorName:   "custom-tpl",
		Public:      true,
		EnvdVersion: cfg.EnvdVersion,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("PutTemplateBuild: %v", err)
	}

	catalog := NewWritableTemplateCatalog(cfg, testTemplateCatalog(cfg), st)
	got, err := catalog.Get(context.Background(), "custom-tpl")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.BuildStatus != string(store.TemplateBuildStatusBuilding) || got.Name != "custom-tpl" {
		t.Fatalf("got = %+v", got)
	}

	list, err := catalog.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var found bool
	for _, item := range list {
		if item.TemplateID == "custom-tpl" {
			found = true
			if item.BuildStatus != string(store.TemplateBuildStatusBuilding) {
				t.Fatalf("list item = %+v", item)
			}
		}
	}
	if !found {
		t.Fatalf("custom-tpl missing from list: %+v", list)
	}
}

func TestCreateSandboxUserTemplateUsesActorTemplateName(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	actors := &fakeActors{}
	st := newFakeStore()
	srv := NewServer(cfg, actors, st, slog.Default())

	body := []byte(`{"alias":"myapp","dockerfile":"FROM actordock/base"}`)
	req := httptest.NewRequest(http.MethodPost, "/templates", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create template status = %d, body = %s", rec.Code, rec.Body.String())
	}

	sbxReq := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader([]byte(`{"templateID":"myapp","secure":false}`)))
	sbxReq.Header.Set("Content-Type", "application/json")
	sbxReq.Header.Set("X-API-KEY", "dev")
	sbxRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(sbxRec, sbxReq)
	if sbxRec.Code != http.StatusCreated {
		t.Fatalf("create sandbox status = %d, body = %s", sbxRec.Code, sbxRec.Body.String())
	}
	if actors.lastTemplateName != "myapp" {
		t.Fatalf("template name = %q, want myapp", actors.lastTemplateName)
	}
}

func TestListTemplatesIncludesBuildingStatus(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	st := newFakeStore()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	if err := st.PutTemplateBuild(context.Background(), store.TemplateBuild{
		TemplateID:  "building-tpl",
		BuildID:     "build-1",
		Status:      store.TemplateBuildStatusWaiting,
		CPUCount:    2,
		MemoryMB:    512,
		Namespace:   "actordock",
		ActorName:   "building-tpl",
		Public:      true,
		EnvdVersion: cfg.EnvdVersion,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("PutTemplateBuild: %v", err)
	}

	srv := NewServer(cfg, &fakeActors{}, st, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/templates", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var resp []templateResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, item := range resp {
		if item.TemplateID == "building-tpl" && item.BuildStatus == string(store.TemplateBuildStatusWaiting) {
			return
		}
	}
	t.Fatalf("building-tpl not listed with waiting status: %+v", resp)
}
