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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/actordock/actordock/internal/store"
)

func seedTemplateBuild(t *testing.T, st *fakeStore, templateID, buildID string) {
	t.Helper()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	if err := st.PutTemplateBuild(context.Background(), store.TemplateBuild{
		TemplateID:  templateID,
		BuildID:     buildID,
		Status:      store.TemplateBuildStatusBuilding,
		CPUCount:    2,
		MemoryMB:    512,
		Namespace:   "actordock",
		ActorName:   templateID,
		Public:      false,
		EnvdVersion: "0.1.0",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("PutTemplateBuild: %v", err)
	}
}

func TestStartTemplateBuildV2Success(t *testing.T) {
	t.Parallel()

	srv, st := testServerWithBuildFiles(t)
	seedTemplateBuild(t, st, "custom-app", "build-1")

	body := `{"fromTemplate":"base","steps":[{"type":"RUN","args":["echo hi"]}]}`
	req := httptest.NewRequest(http.MethodPost, "/v2/templates/custom-app/builds/build-1", strings.NewReader(body))
	req.Header.Set("X-API-KEY", "dev")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(st.templateBuildQueue) != 1 {
		t.Fatalf("queue = %+v", st.templateBuildQueue)
	}

	build, err := st.GetTemplateBuild(context.Background(), "custom-app", "build-1")
	if err != nil {
		t.Fatalf("GetTemplateBuild: %v", err)
	}
	if build.Status != store.TemplateBuildStatusWaiting {
		t.Fatalf("status = %q", build.Status)
	}
	if build.FromTemplate != "base" || len(build.StepsJSON) == 0 {
		t.Fatalf("build = %+v", build)
	}
}

func TestStartTemplateBuildV2RejectNonOfficialBase(t *testing.T) {
	t.Parallel()

	srv, st := testServerWithBuildFiles(t)
	seedTemplateBuild(t, st, "custom-app", "build-1")

	body := `{"fromTemplate":"custom-app","steps":[{"type":"RUN","args":["echo hi"]}]}`
	req := httptest.NewRequest(http.MethodPost, "/v2/templates/custom-app/builds/build-1", strings.NewReader(body))
	req.Header.Set("X-API-KEY", "dev")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	build, err := st.GetTemplateBuild(context.Background(), "custom-app", "build-1")
	if err != nil {
		t.Fatalf("GetTemplateBuild: %v", err)
	}
	if build.Status != store.TemplateBuildStatusError {
		t.Fatalf("status = %q", build.Status)
	}
}

func TestStartTemplateBuildV2RejectUnsupportedStep(t *testing.T) {
	t.Parallel()

	srv, st := testServerWithBuildFiles(t)
	seedTemplateBuild(t, st, "custom-app", "build-1")

	body := `{"fromTemplate":"base","steps":[{"type":"FROM","args":["ubuntu"]}]}`
	req := httptest.NewRequest(http.MethodPost, "/v2/templates/custom-app/builds/build-1", strings.NewReader(body))
	req.Header.Set("X-API-KEY", "dev")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestStartTemplateBuildV2RejectMissingCopyFiles(t *testing.T) {
	t.Parallel()

	srv, st := testServerWithBuildFiles(t)
	seedTemplateBuild(t, st, "custom-app", "build-1")

	body := `{"fromTemplate":"base","steps":[{"type":"COPY","args":[".","/app"],"filesHash":"missing"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v2/templates/custom-app/builds/build-1", strings.NewReader(body))
	req.Header.Set("X-API-KEY", "dev")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestStartTemplateBuildV2AcceptsUploadedCopyFiles(t *testing.T) {
	t.Parallel()

	srv, st := testServerWithBuildFiles(t)
	seedTemplateBuild(t, st, "custom-app", "build-1")
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	if err := st.PutTemplateBuildFile(context.Background(), store.TemplateBuildFile{
		FilesHash: "layer-1",
		ObjectKey: buildFileObjectKey("layer-1"),
		Present:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("PutTemplateBuildFile: %v", err)
	}
	if err := srv.buildFiles.write("layer-1", bytes.NewReader([]byte("tar"))); err != nil {
		t.Fatalf("write: %v", err)
	}

	body := `{"fromTemplate":"base","steps":[{"type":"COPY","args":[".","/app"],"filesHash":"layer-1"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v2/templates/custom-app/builds/build-1", strings.NewReader(body))
	req.Header.Set("X-API-KEY", "dev")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestStartTemplateBuildV2AcceptsUserFromImage(t *testing.T) {
	t.Parallel()

	srv, st := testServerWithBuildFiles(t)
	seedTemplateBuild(t, st, "custom-app", "build-1")

	body := `{"fromImage":"registry.example.com/my-team/envd-base:1.0","steps":[{"type":"RUN","args":["echo hi"]}]}`
	req := httptest.NewRequest(http.MethodPost, "/v2/templates/custom-app/builds/build-1", strings.NewReader(body))
	req.Header.Set("X-API-KEY", "dev")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestStartTemplateBuildV2RejectRegistryAuth(t *testing.T) {
	t.Parallel()

	srv, st := testServerWithBuildFiles(t)
	seedTemplateBuild(t, st, "custom-app", "build-1")

	body := `{"fromImage":"registry.example.com/my-team/envd-base:1.0","fromImageRegistry":{"type":"registry","username":"u","password":"p"},"steps":[]}`
	req := httptest.NewRequest(http.MethodPost, "/v2/templates/custom-app/builds/build-1", strings.NewReader(body))
	req.Header.Set("X-API-KEY", "dev")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestStartTemplateBuildV2BuildNotFound(t *testing.T) {
	t.Parallel()

	srv, _ := testServerWithBuildFiles(t)
	body := `{"fromTemplate":"base","steps":[]}`
	req := httptest.NewRequest(http.MethodPost, "/v2/templates/custom-app/builds/missing", strings.NewReader(body))
	req.Header.Set("X-API-KEY", "dev")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestStartTemplateBuildV2StoresRequestJSON(t *testing.T) {
	t.Parallel()

	srv, st := testServerWithBuildFiles(t)
	seedTemplateBuild(t, st, "custom-app", "build-1")

	body := `{"fromTemplate":"base","startCmd":"python main.py","steps":[{"type":"RUN","args":["pip install numpy"]}]}`
	req := httptest.NewRequest(http.MethodPost, "/v2/templates/custom-app/builds/build-1", strings.NewReader(body))
	req.Header.Set("X-API-KEY", "dev")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	build, err := st.GetTemplateBuild(context.Background(), "custom-app", "build-1")
	if err != nil {
		t.Fatalf("GetTemplateBuild: %v", err)
	}
	var stored templateBuildStartV2
	if err := json.Unmarshal(build.StepsJSON, &stored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if stored.StartCmd != "python main.py" || stored.FromTemplate != "base" {
		t.Fatalf("stored = %+v", stored)
	}
}
