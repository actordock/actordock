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
	"testing"
	"time"

	"github.com/actordock/actordock/internal/store"
)

func seedTemplateBuildWithStatus(t *testing.T, st *fakeStore, templateID, buildID string, status store.TemplateBuildStatus) {
	t.Helper()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	build := store.TemplateBuild{
		TemplateID: templateID,
		BuildID:    buildID,
		Status:     status,
		Tags:       []string{"latest"},
		Namespace:  "actordock",
		ActorName:  templateID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := st.PutTemplateBuild(context.Background(), build); err != nil {
		t.Fatalf("PutTemplateBuild: %v", err)
	}
}

func TestGetTemplateBuildStatus(t *testing.T) {
	t.Parallel()

	srv, st := testServerWithBuildFiles(t)
	seedTemplateBuildWithStatus(t, st, "my-app", "build-1", store.TemplateBuildStatusBuilding)
	now := time.Date(2026, 6, 24, 10, 1, 0, 0, time.UTC)
	if err := st.AppendBuildLog(context.Background(), store.BuildLogEntry{
		TemplateID: "my-app",
		BuildID:    "build-1",
		Timestamp:  now,
		Message:    "building image",
		Level:      "info",
	}); err != nil {
		t.Fatalf("AppendBuildLog: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/templates/my-app/builds/build-1/status", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp templateBuildInfoResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "building" || len(resp.Logs) != 1 || resp.Logs[0] != "building image" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestGetTemplateBuildStatusErrorReason(t *testing.T) {
	t.Parallel()

	srv, st := testServerWithBuildFiles(t)
	seedTemplateBuildWithStatus(t, st, "my-app", "build-1", store.TemplateBuildStatusError)
	build, err := st.GetTemplateBuild(context.Background(), "my-app", "build-1")
	if err != nil {
		t.Fatalf("GetTemplateBuild: %v", err)
	}
	build.ErrorMessage = "kaniko failed"
	if err := st.UpdateTemplateBuild(context.Background(), build); err != nil {
		t.Fatalf("UpdateTemplateBuild: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/templates/my-app/builds/build-1/status", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp templateBuildInfoResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Reason == nil || resp.Reason.Message != "kaniko failed" {
		t.Fatalf("reason = %+v", resp.Reason)
	}
}

func TestGetTemplateBuildLogs(t *testing.T) {
	t.Parallel()

	srv, st := testServerWithBuildFiles(t)
	seedTemplateBuildWithStatus(t, st, "my-app", "build-1", store.TemplateBuildStatusBuilding)
	now := time.Date(2026, 6, 24, 10, 1, 0, 0, time.UTC)
	for _, msg := range []string{"step 1", "step 2"} {
		if err := st.AppendBuildLog(context.Background(), store.BuildLogEntry{
			TemplateID: "my-app",
			BuildID:    "build-1",
			Timestamp:  now,
			Message:    msg,
			Level:      "info",
		}); err != nil {
			t.Fatalf("AppendBuildLog: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/templates/my-app/builds/build-1/logs?limit=1", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp templateBuildLogsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Logs) != 1 || resp.Logs[0].Message != "step 1" {
		t.Fatalf("logs = %+v", resp.Logs)
	}
}

func TestAssignAndDeleteTemplateTags(t *testing.T) {
	t.Parallel()

	srv, st := testServerWithBuildFiles(t)
	seedTemplateBuildWithStatus(t, st, "my-app", "build-1", store.TemplateBuildStatusReady)

	body := `{"target":"my-app","tags":["prod","staging"]}`
	req := httptest.NewRequest(http.MethodPost, "/templates/tags", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("assign status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var assigned assignedTemplateTagsResponse
	if err := json.NewDecoder(rec.Body).Decode(&assigned); err != nil {
		t.Fatalf("decode assign: %v", err)
	}
	if assigned.BuildID != "build-1" || len(assigned.Tags) != 2 {
		t.Fatalf("assigned = %+v", assigned)
	}

	req = httptest.NewRequest(http.MethodGet, "/templates/my-app/tags", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get tags status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var tags []templateTagResponse
	if err := json.NewDecoder(rec.Body).Decode(&tags); err != nil {
		t.Fatalf("decode tags: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("tags = %+v", tags)
	}

	delBody := `{"name":"my-app","tags":["prod"]}`
	req = httptest.NewRequest(http.MethodDelete, "/templates/tags", bytes.NewReader([]byte(delBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/templates/my-app/tags", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if err := json.NewDecoder(rec.Body).Decode(&tags); err != nil {
		t.Fatalf("decode tags after delete: %v", err)
	}
	if len(tags) != 1 || tags[0].Tag != "staging" {
		t.Fatalf("tags after delete = %+v", tags)
	}
}

func TestPatchTemplateV2BuildOnly(t *testing.T) {
	t.Parallel()

	srv, st := testServerWithBuildFiles(t)
	seedTemplateBuildWithStatus(t, st, "my-app", "build-1", store.TemplateBuildStatusReady)

	body := `{"public":true}`
	req := httptest.NewRequest(http.MethodPatch, "/v2/templates/my-app", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	build, err := st.GetTemplateBuild(context.Background(), "my-app", "build-1")
	if err != nil {
		t.Fatalf("GetTemplateBuild: %v", err)
	}
	if !build.Public {
		t.Fatalf("public = false, want true")
	}
}
