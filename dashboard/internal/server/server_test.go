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

package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testServer(t *testing.T, cfg Config) *Server {
	t.Helper()
	srv, err := New(cfg, slog.Default())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv
}

func TestHealth(t *testing.T) {
	t.Parallel()
	srv := testServer(t, Config{APIKey: "dev"})
	handler, err := srv.Handler()
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %q", body["status"])
	}
}

func TestPlatformProxyForwardsPathAndAPIKey(t *testing.T) {
	t.Parallel()

	var gotPath, gotAPIKey string
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("X-API-KEY")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(platform.Close)

	srv := testServer(t, Config{
		PlatformURL:   platform.URL,
		APIKey:        "test-key",
		ProxyPlatform: true,
	})
	handler, err := srv.Handler()
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/platform/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotPath != "/health" {
		t.Fatalf("proxied path = %q, want /health", gotPath)
	}
	if gotAPIKey != "test-key" {
		t.Fatalf("X-API-KEY = %q, want test-key", gotAPIKey)
	}
}

func TestPlatformProxyMissingAPIKey(t *testing.T) {
	t.Parallel()
	srv := testServer(t, Config{
		PlatformURL:   "http://example.com",
		APIKey:        "",
		ProxyPlatform: true,
	})
	handler, err := srv.Handler()
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/platform/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "API key") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestSPAServesIndex(t *testing.T) {
	t.Parallel()
	srv := testServer(t, Config{APIKey: "dev", ProxyPlatform: false})
	handler, err := srv.Handler()
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/theme-preview", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "Actordock") {
		t.Fatalf("expected SPA index HTML, got %q", truncate(string(body), 120))
	}
}

func TestRouterProxyForwardsPath(t *testing.T) {
	t.Parallel()

	var gotPath string
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(router.Close)

	srv := testServer(t, Config{
		RouterURL:   router.URL,
		ProxyRouter: true,
	})
	handler, err := srv.Handler()
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/router/process.Process/Start", nil)
	req.Header.Set("E2b-Sandbox-Id", "sb-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotPath != "/process.Process/Start" {
		t.Fatalf("proxied path = %q, want /process.Process/Start", gotPath)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
