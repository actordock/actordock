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
	"github.com/agent-substrate/substrate/pkg/proto/ateapipb"
)

func TestCreateVolume(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())
	srv.nowFunc = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodPost, "/volumes", bytes.NewReader([]byte(`{"name":"my-data"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp volumeAndTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.VolumeID == "" || resp.Name != "my-data" || resp.Token == "" {
		t.Fatalf("resp = %+v", resp)
	}

	got, err := st.GetVolume(context.Background(), resp.VolumeID)
	if err != nil {
		t.Fatalf("GetVolume: %v", err)
	}
	if got.Name != "my-data" || got.Token != resp.Token {
		t.Fatalf("stored volume = %+v", got)
	}
	if got.HostPath != store.VolumeHostPath("/var/lib/actordock/volumes", resp.VolumeID) {
		t.Fatalf("host path = %q", got.HostPath)
	}
}

func TestCreateVolumeInvalidName(t *testing.T) {
	t.Parallel()

	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/volumes", bytes.NewReader([]byte(`{"name":"bad name"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCreateVolumeDuplicateName(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())
	srv.nowFunc = func() time.Time { return now }

	body := []byte(`{"name":"shared"}`)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/volumes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-KEY", "dev")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if i == 0 && rec.Code != http.StatusCreated {
			t.Fatalf("first status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if i == 1 && rec.Code != http.StatusBadRequest {
			t.Fatalf("second status = %d, want 400", rec.Code)
		}
	}
}

func TestListVolumes(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	if err := st.PutVolume(context.Background(), store.Volume{
		VolumeID:  "vol-1",
		Name:      "data-a",
		Token:     "tok-a",
		HostPath:  "/data/vol-1/",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("PutVolume: %v", err)
	}

	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/volumes", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp []volumeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 1 || resp[0].VolumeID != "vol-1" || resp[0].Name != "data-a" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestGetVolume(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	if err := st.PutVolume(context.Background(), store.Volume{
		VolumeID:  "vol-1",
		Name:      "data-a",
		Token:     "secret",
		HostPath:  "/data/vol-1/",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("PutVolume: %v", err)
	}

	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/volumes/vol-1", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp volumeAndTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.VolumeID != "vol-1" || resp.Name != "data-a" || resp.Token != "secret" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestGetVolumeNotFound(t *testing.T) {
	t.Parallel()

	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/volumes/missing", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDeleteVolume(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	if err := st.PutVolume(context.Background(), store.Volume{
		VolumeID:  "vol-1",
		Name:      "data-a",
		Token:     "secret",
		HostPath:  "/data/vol-1/",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("PutVolume: %v", err)
	}

	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())
	req := httptest.NewRequest(http.MethodDelete, "/volumes/vol-1", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := st.GetVolume(context.Background(), "vol-1"); err == nil {
		t.Fatal("volume still exists after delete")
	}
}

func TestCreateSandboxVolumeMountsRoundTrip(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	if err := st.PutVolume(context.Background(), store.Volume{
		VolumeID:  "vol-1",
		Name:      "my-data",
		Token:     "secret",
		HostPath:  "/data/vol-1/",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("PutVolume: %v", err)
	}

	actors := &fakeActors{defaultStatus: ateapipb.Actor_STATUS_RUNNING}
	srv := NewServer(testConfig(), actors, st, slog.Default())
	srv.nowFunc = func() time.Time { return now }

	body := []byte(`{
		"templateID":"base",
		"secure":false,
		"volumeMounts":[{"name":"my-data","path":"/mnt/data"}]
	}`)
	createReq := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-API-KEY", "dev")
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	var created sandboxResponse
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	got := st.records[created.SandboxID]
	if len(got.VolumeMounts) != 1 || got.VolumeMounts[0].Name != "my-data" || got.VolumeMounts[0].Path != "/mnt/data" {
		t.Fatalf("stored mounts = %+v", got.VolumeMounts)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/sandboxes/"+created.SandboxID, nil)
	getReq.Header.Set("X-API-KEY", "dev")
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRec.Code, getRec.Body.String())
	}

	var detail sandboxDetailResponse
	if err := json.NewDecoder(getRec.Body).Decode(&detail); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if len(detail.VolumeMounts) != 1 || detail.VolumeMounts[0].Name != "my-data" || detail.VolumeMounts[0].Path != "/mnt/data" {
		t.Fatalf("detail mounts = %+v", detail.VolumeMounts)
	}
}

func TestCreateSandboxUnknownVolume(t *testing.T) {
	t.Parallel()

	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	body := []byte(`{"templateID":"base","volumeMounts":[{"name":"missing","path":"/mnt/data"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
