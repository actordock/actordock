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
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/actordock/actordock/internal/store"
)

func TestProxySecureSandboxRequiresToken(t *testing.T) {
	t.Parallel()

	envdSrv, token := startSecuredEnvdBackend(t)
	actors := &fakeBackend{backend: envdSrv.Listener.Addr().String()}
	policies := newFakePolicyStore()
	policies.records[testSandboxID] = store.Sandbox{
		SandboxID:       testSandboxID,
		Secure:          true,
		EnvdAccessToken: token,
	}

	router := NewServer(testConfig(), actors, policies, slog.Default())
	routerSrv := startH2CTestServer(t, router.Handler())
	t.Cleanup(routerSrv.Close)

	req := httptest.NewRequest(http.MethodGet, routerSrv.URL+"/process.Process/List", nil)
	req.Header.Set(sandboxIDHeader, testSandboxID)
	rec := httptest.NewRecorder()
	routerSrv.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status without token = %d, want 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, routerSrv.URL+"/process.Process/List", nil)
	req.Header.Set(sandboxIDHeader, testSandboxID)
	req.Header.Set(envdAccessTokenHeader, token)
	rec = httptest.NewRecorder()
	routerSrv.Config.Handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("status with token = %d, want proxy success", rec.Code)
	}
}

func startSecuredEnvdBackend(t *testing.T) (*httptest.Server, string) {
	t.Helper()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(backend.Close)
	return backend, "router-test-token"
}

func TestCheckEnvdAccess(t *testing.T) {
	t.Parallel()

	policies := newFakePolicyStore()
	policies.records["sb-1"] = store.Sandbox{
		SandboxID:       "sb-1",
		Secure:          true,
		EnvdAccessToken: "tok",
	}
	srv := NewServer(testConfig(), nil, policies, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "http://example.com/files", nil)
	if err := srv.checkEnvdAccess(req, "sb-1"); err == nil {
		t.Fatal("expected unauthorized without header")
	}

	req.Header.Set(envdAccessTokenHeader, "tok")
	if err := srv.checkEnvdAccess(req, "sb-1"); err != nil {
		t.Fatalf("checkEnvdAccess: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/files", nil)
	if err := srv.checkEnvdAccess(req, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing sandbox err = %v", err)
	}
}
