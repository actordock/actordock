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

	"github.com/actordock/actordock/internal/config"
	"github.com/actordock/actordock/internal/store"
	"github.com/actordock/actordock/internal/substrate"
)

type fakeActors struct {
	lastActorID   string
	lastDeletedID string
	createErr     error
	deleteErr     error
}

func (f *fakeActors) CreateAndResumeSandbox(_ context.Context, actorID, _, _ string) error {
	f.lastActorID = actorID
	return f.createErr
}

func (f *fakeActors) DeleteSandbox(_ context.Context, actorID string) error {
	f.lastDeletedID = actorID
	return f.deleteErr
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
