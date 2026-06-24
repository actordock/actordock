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

package router

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/actordock/actordock/internal/config"
	"github.com/actordock/actordock/internal/envd"
	"github.com/actordock/actordock/internal/runtimeapi"
)

type fakeBackend struct {
	lastSandboxID string
	backend       string
	waitEnvd      bool
	err           error
}

func (f *fakeBackend) ResumeSandboxBackend(_ context.Context, actorID string, _ int) (string, bool, error) {
	f.lastSandboxID = actorID
	if f.err != nil {
		return "", false, f.err
	}
	return f.backend, f.waitEnvd, nil
}

func TestHealth(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeBackend{}, nil, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestParseSandboxIDFromHost(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "abc-123.localhost"
	id, err := parseSandboxID(req, "localhost", 80)
	if err != nil || id != "abc-123" {
		t.Fatalf("id = %q, err = %v", id, err)
	}
}

func TestParseSandboxIDFromPortHost(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "80-abc-123.localhost"
	id, err := parseSandboxID(req, "localhost", 80)
	if err != nil || id != "abc-123" {
		t.Fatalf("id = %q, err = %v", id, err)
	}
}

func TestParseSandboxIDFromHeader(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "localhost:8081"
	req.Header.Set("E2b-Sandbox-Id", "header-id")
	id, err := parseSandboxID(req, "localhost", 80)
	if err != nil || id != "header-id" {
		t.Fatalf("id = %q, err = %v", id, err)
	}
}

func TestProxyToEnvd(t *testing.T) {
	t.Parallel()
	envd := httptest.NewServer(envd.NewStubHandler())
	t.Cleanup(envd.Close)

	backendHost := envd.Listener.Addr().String()
	actors := &fakeBackend{backend: backendHost}
	srv := NewServer(testConfig(), actors, nil, slog.Default())
	srv.envdTransport = http.DefaultTransport

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Host = "sandbox-1.localhost"
	req.Header.Set("E2b-Sandbox-Id", "sandbox-1")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if actors.lastSandboxID != "sandbox-1" {
		t.Fatalf("sandbox id = %q", actors.lastSandboxID)
	}
}

func TestProxyResumesPausedSandbox(t *testing.T) {
	t.Parallel()
	stub := envd.NewStubHandler()
	envdSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		stub.ServeHTTP(w, r)
	}))
	t.Cleanup(envdSrv.Close)

	actors := &fakeBackend{backend: envdSrv.Listener.Addr().String(), waitEnvd: true}
	srv := NewServer(testConfig(), actors, nil, slog.Default())
	srv.envdTransport = http.DefaultTransport

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "paused-sbx.localhost"
	req.Header.Set("E2b-Sandbox-Id", "paused-sbx")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if actors.lastSandboxID != "paused-sbx" {
		t.Fatalf("ResumeSandboxBackend sandbox id = %q, want paused-sbx", actors.lastSandboxID)
	}
}

func TestProxySandboxNotFound(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeBackend{err: runtimeapi.ErrNotFound}, nil, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("E2b-Sandbox-Id", "missing")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestProxyMissingSandboxID(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeBackend{}, nil, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "localhost:8081"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if len(body) == 0 {
		t.Fatal("expected error body")
	}
}

func testConfig() config.Router {
	return config.Router{
		Server: config.Server{
			ListenAddr: ":8081",
			LogLevel:   "info",
		},
		Domain:   "localhost",
		EnvdPort: 80,
	}
}
