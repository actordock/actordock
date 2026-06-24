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
	"strings"
	"testing"
	"time"

	"github.com/actordock/actordock/internal/store"
)

func testServerWithBuildFiles(t *testing.T) (*Server, *fakeStore) {
	t.Helper()
	cfg := testConfig()
	cfg.TemplateBuildFilesDir = t.TempDir()
	st := newFakeStore()
	srv := NewServerWithCatalog(cfg, &fakeActors{}, st, testTemplateCatalog(cfg), slog.Default())
	return srv, st
}

func TestParseTemplateBuildName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		alias    string
		tags     []string
		wantID   string
		wantTags []string
		wantErr  bool
	}{
		{name: "plain", input: "my-app", wantID: "my-app"},
		{name: "with tag", input: "my-app:v1", wantID: "my-app", wantTags: []string{"v1"}},
		{name: "explicit tags merged", input: "my-app:v1", tags: []string{"latest"}, wantID: "my-app", wantTags: []string{"latest", "v1"}},
		{name: "alias fallback", alias: "legacy", wantID: "legacy"},
		{name: "missing name", wantErr: true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotID, gotTags, err := parseTemplateBuildName(tc.input, tc.alias, tc.tags)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTemplateBuildName: %v", err)
			}
			if gotID != tc.wantID {
				t.Fatalf("templateID = %q, want %q", gotID, tc.wantID)
			}
			if len(gotTags) != len(tc.wantTags) {
				t.Fatalf("tags = %v, want %v", gotTags, tc.wantTags)
			}
			for i := range tc.wantTags {
				if gotTags[i] != tc.wantTags[i] {
					t.Fatalf("tags = %v, want %v", gotTags, tc.wantTags)
				}
			}
		})
	}
}

func TestCreateTemplateV3(t *testing.T) {
	t.Parallel()

	srv, st := testServerWithBuildFiles(t)
	body := `{"name":"custom-app","cpuCount":2,"memoryMB":512}`
	req := httptest.NewRequest(http.MethodPost, "/v3/templates", strings.NewReader(body))
	req.Header.Set("X-API-KEY", "dev")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp templateRequestResponseV3
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TemplateID != "custom-app" || resp.BuildID == "" {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Public || len(resp.Aliases) != 1 || resp.Aliases[0] != "custom-app" {
		t.Fatalf("resp = %+v", resp)
	}

	build, err := st.GetLatestTemplateBuild(context.Background(), "custom-app")
	if err != nil {
		t.Fatalf("GetLatestTemplateBuild: %v", err)
	}
	if build.Status != store.TemplateBuildStatusBuilding {
		t.Fatalf("build status = %q", build.Status)
	}
}

func TestCreateTemplateV3NameTag(t *testing.T) {
	t.Parallel()

	srv, _ := testServerWithBuildFiles(t)
	body := `{"name":"custom-app:v1"}`
	req := httptest.NewRequest(http.MethodPost, "/v3/templates", strings.NewReader(body))
	req.Header.Set("X-API-KEY", "dev")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp templateRequestResponseV3
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TemplateID != "custom-app" || len(resp.Tags) != 1 || resp.Tags[0] != "v1" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestCreateTemplateV3ConflictWithBase(t *testing.T) {
	t.Parallel()

	srv, _ := testServerWithBuildFiles(t)
	body := `{"name":"base"}`
	req := httptest.NewRequest(http.MethodPost, "/v3/templates", strings.NewReader(body))
	req.Header.Set("X-API-KEY", "dev")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestTemplateBuildFileUploadFlow(t *testing.T) {
	t.Parallel()

	srv, st := testServerWithBuildFiles(t)
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	if err := st.PutTemplateBuild(context.Background(), store.TemplateBuild{
		TemplateID:  "custom-app",
		BuildID:     "build-1",
		Status:      store.TemplateBuildStatusBuilding,
		CPUCount:    2,
		MemoryMB:    512,
		Namespace:   "actordock",
		ActorName:   "custom-app",
		Public:      false,
		EnvdVersion: "0.1.0",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("PutTemplateBuild: %v", err)
	}

	const filesHash = "abc123"
	getReq := httptest.NewRequest(http.MethodGet, "/templates/custom-app/files/"+filesHash, nil)
	getReq.Header.Set("X-API-KEY", "dev")
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusCreated {
		t.Fatalf("get status = %d, body = %s", getRec.Code, getRec.Body.String())
	}

	var upload templateBuildFileUploadResponse
	if err := json.NewDecoder(getRec.Body).Decode(&upload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if upload.Present || upload.URL == nil || !strings.Contains(*upload.URL, "/template-build-files/"+filesHash) {
		t.Fatalf("upload = %+v", upload)
	}

	tarBody := []byte("fake-tar-content")
	putReq := httptest.NewRequest(http.MethodPut, "/template-build-files/"+filesHash, bytes.NewReader(tarBody))
	putRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("put status = %d, body = %s", putRec.Code, putRec.Body.String())
	}

	getRec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec2, getReq)
	if getRec2.Code != http.StatusCreated {
		t.Fatalf("get2 status = %d, body = %s", getRec2.Code, getRec2.Body.String())
	}
	var uploadAfter templateBuildFileUploadResponse
	if err := json.NewDecoder(getRec2.Body).Decode(&uploadAfter); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !uploadAfter.Present || uploadAfter.URL != nil {
		t.Fatalf("upload after put = %+v", uploadAfter)
	}
}

func TestGetTemplateFileUploadPresentWithoutDisk(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.TemplateBuildFilesDir = t.TempDir()
	st := newFakeStore()
	srv := NewServerWithCatalog(cfg, &fakeActors{}, st, testTemplateCatalog(cfg), slog.Default())

	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	if err := st.PutTemplateBuildFile(context.Background(), store.TemplateBuildFile{
		FilesHash: "cached-hash",
		ObjectKey: buildFileObjectKey("cached-hash"),
		Present:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("PutTemplateBuildFile: %v", err)
	}
	if err := srv.buildFiles.write("cached-hash", bytes.NewReader([]byte("tar"))); err != nil {
		t.Fatalf("write: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/templates/base/files/cached-hash", nil)
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
