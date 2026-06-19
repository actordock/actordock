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

	"github.com/actordock/actordock/internal/config"
	"github.com/actordock/actordock/internal/store"
	"github.com/actordock/actordock/internal/substrate"
	"github.com/agent-substrate/substrate/pkg/proto/ateapipb"
)

type fakeActors struct {
	lastActorID   string
	lastDeletedID string
	createErr     error
	deleteErr     error
	getErr        error
	actorStatuses map[string]ateapipb.Actor_Status
	defaultStatus ateapipb.Actor_Status
}

func (f *fakeActors) CreateAndResumeSandbox(_ context.Context, actorID, _, _ string) error {
	f.lastActorID = actorID
	return f.createErr
}

func (f *fakeActors) DeleteSandbox(_ context.Context, actorID string) error {
	f.lastDeletedID = actorID
	return f.deleteErr
}

func (f *fakeActors) GetActor(_ context.Context, actorID string) (ateapipb.Actor_Status, error) {
	if f.getErr != nil {
		return ateapipb.Actor_STATUS_UNSPECIFIED, f.getErr
	}
	if f.actorStatuses != nil {
		if status, ok := f.actorStatuses[actorID]; ok {
			return status, nil
		}
	}
	if f.defaultStatus != ateapipb.Actor_STATUS_UNSPECIFIED {
		return f.defaultStatus, nil
	}
	return ateapipb.Actor_STATUS_RUNNING, nil
}

type fakeStore struct {
	records map[string]store.Sandbox
	putErr  error
	delErr  error
}

func newFakeStore() *fakeStore {
	return &fakeStore{records: make(map[string]store.Sandbox)}
}

func (f *fakeStore) Put(_ context.Context, sb store.Sandbox) error {
	if f.putErr != nil {
		return f.putErr
	}
	f.records[sb.SandboxID] = sb
	return nil
}

func (f *fakeStore) Delete(_ context.Context, sandboxID string) error {
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.records, sandboxID)
	return nil
}

func (f *fakeStore) Get(_ context.Context, sandboxID string) (store.Sandbox, error) {
	sb, ok := f.records[sandboxID]
	if !ok {
		return store.Sandbox{}, store.ErrNotFound
	}
	return sb, nil
}

func (f *fakeStore) List(_ context.Context) ([]store.Sandbox, error) {
	out := make([]store.Sandbox, 0, len(f.records))
	for _, sb := range f.records {
		out = append(out, sb)
	}
	return out, nil
}

func TestHealth(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestCreateSandbox(t *testing.T) {
	t.Parallel()
	actors := &fakeActors{}
	st := newFakeStore()
	srv := NewServer(testConfig(), actors, st, slog.Default())

	body := []byte(`{"templateID":"base","secure":false}`)
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SandboxID == "" || resp.SandboxID != actors.lastActorID {
		t.Fatalf("sandboxID = %q, actor = %q", resp.SandboxID, actors.lastActorID)
	}
	if resp.Domain != "localhost" {
		t.Fatalf("domain = %q", resp.Domain)
	}
	if resp.EnvdVersion != "0.1.0" {
		t.Fatalf("envdVersion = %q", resp.EnvdVersion)
	}
	if resp.EnvdAccessToken != "" {
		t.Fatalf("envdAccessToken = %q, want empty", resp.EnvdAccessToken)
	}
	got, ok := st.records[resp.SandboxID]
	if !ok {
		t.Fatalf("sandbox %q not in store", resp.SandboxID)
	}
	if got.ActorID != resp.SandboxID || got.Template != "base" || got.Status != store.StatusRunning {
		t.Fatalf("stored sandbox = %+v", got)
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("created_at is zero")
	}
}

func TestCreateSandboxUnauthorized(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader([]byte(`{"templateID":"base"}`)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestDeleteSandbox(t *testing.T) {
	t.Parallel()
	actors := &fakeActors{}
	st := newFakeStore()
	st.records["abc-123"] = store.Sandbox{SandboxID: "abc-123", ActorID: "abc-123"}
	srv := NewServer(testConfig(), actors, st, slog.Default())

	req := httptest.NewRequest(http.MethodDelete, "/sandboxes/abc-123", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if actors.lastDeletedID != "abc-123" {
		t.Fatalf("deleted id = %q, want abc-123", actors.lastDeletedID)
	}
	if _, ok := st.records["abc-123"]; ok {
		t.Fatal("sandbox still in store after delete")
	}
}

func TestDeleteSandboxNotFound(t *testing.T) {
	t.Parallel()
	actors := &fakeActors{deleteErr: substrate.ErrNotFound}
	srv := NewServer(testConfig(), actors, newFakeStore(), slog.Default())

	req := httptest.NewRequest(http.MethodDelete, "/sandboxes/missing", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDeleteSandboxUnauthorized(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodDelete, "/sandboxes/abc-123", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestCreateSandboxUnsupportedTemplate(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader([]byte(`{"templateID":"other"}`)))
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetSandbox(t *testing.T) {
	t.Parallel()
	createdAt := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		CreatedAt: createdAt,
		Status:    store.StatusRunning,
	}
	actors := &fakeActors{defaultStatus: ateapipb.Actor_STATUS_RUNNING}
	srv := NewServer(testConfig(), actors, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxDetailResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SandboxID != "sb-1" || resp.State != "running" || resp.TemplateID != "base" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestGetSandboxResuming(t *testing.T) {
	t.Parallel()
	createdAt := time.Now().UTC()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		CreatedAt: createdAt,
		Status:    store.StatusRunning,
	}
	actors := &fakeActors{
		actorStatuses: map[string]ateapipb.Actor_Status{
			"sb-1": ateapipb.Actor_STATUS_RESUMING,
		},
	}
	srv := NewServer(testConfig(), actors, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxDetailResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.State != "running" {
		t.Fatalf("state = %q, want running", resp.State)
	}
	if st.records["sb-1"].Status != store.StatusPending {
		t.Fatalf("stored status = %q, want pending", st.records["sb-1"].Status)
	}
}

func TestGetSandboxNotFound(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/sandboxes/missing", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetSandboxActorGone(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", Template: "base", CreatedAt: time.Now()}
	actors := &fakeActors{getErr: substrate.ErrNotFound}
	srv := NewServer(testConfig(), actors, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if _, ok := st.records["sb-1"]; ok {
		t.Fatal("stale sandbox not purged from store")
	}
}

func TestListSandboxes(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	st := newFakeStore()
	st.records["a"] = store.Sandbox{SandboxID: "a", ActorID: "a", Template: "base", CreatedAt: now, Status: store.StatusRunning}
	st.records["b"] = store.Sandbox{SandboxID: "b", ActorID: "b", Template: "base", CreatedAt: now, Status: store.StatusRunning}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/sandboxes", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp []listedSandboxResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("len = %d, want 2", len(resp))
	}
}

func TestListSandboxesV2(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	st := newFakeStore()
	st.records["a"] = store.Sandbox{SandboxID: "a", ActorID: "a", Template: "base", CreatedAt: now, Status: store.StatusRunning}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/v2/sandboxes", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp []listedSandboxResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 1 || resp[0].SandboxID != "a" {
		t.Fatalf("resp = %+v", resp)
	}
}

func testConfig() config.Platform {
	return config.Platform{
		Server: config.Server{
			ListenAddr: ":8080",
			LogLevel:   "info",
		},
		APIKey:            "dev",
		Domain:            "localhost",
		TemplateNamespace: "actordock",
		TemplateName:      "base",
		EnvdVersion:       "0.1.0",
		ClientID:          "actordock",
	}
}
