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
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListTemplates(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	srv := NewServerWithCatalog(cfg, &fakeActors{}, newFakeStore(), testTemplateCatalog(cfg), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/templates", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp []templateResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 1 || resp[0].TemplateID != "base" || resp[0].BuildStatus != "ready" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestGetTemplateWithBuilds(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	srv := NewServerWithCatalog(cfg, &fakeActors{}, newFakeStore(), testTemplateCatalog(cfg), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/templates/base", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp templateWithBuildsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TemplateID != "base" || len(resp.Builds) != 1 {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestGetTemplateNotFound(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	srv := NewServerWithCatalog(cfg, &fakeActors{}, newFakeStore(), testTemplateCatalog(cfg), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/templates/missing", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetTemplateAlias(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	srv := NewServerWithCatalog(cfg, &fakeActors{}, newFakeStore(), testTemplateCatalog(cfg), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/templates/aliases/base", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp templateAliasResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TemplateID != "base" || !resp.Public {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestGetTemplateAliasNotFound(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	srv := NewServerWithCatalog(cfg, &fakeActors{}, newFakeStore(), testTemplateCatalog(cfg), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/templates/aliases/missing", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetTemplateTagsEmpty(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	srv := NewServerWithCatalog(cfg, &fakeActors{}, newFakeStore(), testTemplateCatalog(cfg), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/templates/base/tags", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp []templateTagResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 0 {
		t.Fatalf("tags = %+v, want empty", resp)
	}
}

func TestGetTemplateFileUploadStub(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	srv := NewServerWithCatalog(cfg, &fakeActors{}, newFakeStore(), testTemplateCatalog(cfg), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/templates/base/files/abc123", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp templateBuildFileUploadResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Present {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestCreateSandboxUsesCatalogTemplate(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	actors := &fakeActors{}
	st := newFakeStore()
	srv := NewServerWithCatalog(cfg, actors, st, testTemplateCatalog(cfg), slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader([]byte(`{"templateID":"base","secure":false}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if actors.lastTemplateNamespace != "actordock" || actors.lastTemplateName != "base" {
		t.Fatalf("template ns/name = %q/%q", actors.lastTemplateNamespace, actors.lastTemplateName)
	}
}
