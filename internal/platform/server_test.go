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
)

type fakeActors struct {
	lastActorID string
	err         error
}

func (f *fakeActors) CreateAndResumeSandbox(_ context.Context, actorID, _, _ string) error {
	f.lastActorID = actorID
	return f.err
}

func TestHealth(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, slog.Default())
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
	srv := NewServer(testConfig(), actors, slog.Default())

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
}

func TestCreateSandboxUnauthorized(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader([]byte(`{"templateID":"base"}`)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestCreateSandboxUnsupportedTemplate(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, slog.Default())
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
