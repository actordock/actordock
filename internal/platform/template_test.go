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

func TestCreateTemplate(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	st := newFakeStore()
	srv := NewServer(cfg, &fakeActors{}, st, slog.Default())

	body := []byte(`{"alias":"myapp","dockerfile":"FROM ubuntu:22.04","cpuCount":2,"memoryMB":512}`)
	req := httptest.NewRequest(http.MethodPost, "/templates", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp templateResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TemplateID != "myapp" || resp.BuildStatus != "ready" || resp.EnvdVersion != cfg.EnvdVersion {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.CPUCount != 2 || resp.MemoryMB != 512 || !resp.Public {
		t.Fatalf("resource fields = %+v", resp)
	}
	if resp.CreatedBy != nil || resp.LastSpawnedAt != nil {
		t.Fatalf("nullable fields = createdBy %v lastSpawnedAt %v", resp.CreatedBy, resp.LastSpawnedAt)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/templates", nil)
	listReq.Header.Set("X-API-KEY", "dev")
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	var listed []templateResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed len = %d, want 2", len(listed))
	}
}

func TestCreateTemplateDuplicateAlias(t *testing.T) {
	t.Parallel()

	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	body := []byte(`{"alias":"base","dockerfile":"FROM ubuntu"}`)
	req := httptest.NewRequest(http.MethodPost, "/templates", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestCreateTemplateMissingDockerfile(t *testing.T) {
	t.Parallel()

	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/templates", bytes.NewReader([]byte(`{"alias":"x"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestPatchTemplate(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	st := newFakeStore()
	srv := NewServer(cfg, &fakeActors{}, st, slog.Default())

	createBody := []byte(`{"alias":"patchme","dockerfile":"FROM alpine"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/templates", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-API-KEY", "dev")
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d", createRec.Code)
	}

	patchBody := []byte(`{"public":false}`)
	patchReq := httptest.NewRequest(http.MethodPatch, "/templates/patchme", bytes.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq.Header.Set("X-API-KEY", "dev")
	patchRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", patchRec.Code, patchRec.Body.String())
	}
	var patchResp templateUpdateResponse
	if err := json.NewDecoder(patchRec.Body).Decode(&patchResp); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	if len(patchResp.Names) != 1 || patchResp.Names[0] != "patchme" {
		t.Fatalf("names = %+v", patchResp.Names)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/templates", nil)
	getReq.Header.Set("X-API-KEY", "dev")
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	var listed []templateResponse
	if err := json.NewDecoder(getRec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	for _, item := range listed {
		if item.TemplateID == "patchme" && item.Public {
			t.Fatalf("public not updated: %+v", item)
		}
	}
}

func TestPatchTemplateNotFound(t *testing.T) {
	t.Parallel()

	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodPatch, "/templates/missing", bytes.NewReader([]byte(`{"public":false}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

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
