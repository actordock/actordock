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
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/actordock/actordock/internal/store"
)

type fakePolicyStore struct {
	records map[string]store.Sandbox
}

func newFakePolicyStore() *fakePolicyStore {
	return &fakePolicyStore{records: make(map[string]store.Sandbox)}
}

func (f *fakePolicyStore) Get(_ context.Context, sandboxID string) (store.Sandbox, error) {
	sb, ok := f.records[sandboxID]
	if !ok {
		return store.Sandbox{}, store.ErrNotFound
	}
	return sb, nil
}

func internetDisabled() *bool {
	v := false
	return &v
}

func TestEgressCONNECTDeniedWhenInternetDisabled(t *testing.T) {
	t.Parallel()
	policies := newFakePolicyStore()
	policies.records["sb-1"] = store.Sandbox{
		SandboxID:           "sb-1",
		AllowInternetAccess: internetDisabled(),
	}
	srv := NewServer(testConfig(), &fakeBackend{}, policies, slog.Default())

	req := httptest.NewRequest(http.MethodConnect, "http://example.com:443", nil)
	req.Host = "example.com:443"
	req.Header.Set("E2b-Sandbox-Id", "sb-1")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestEgressCONNECTAllowedWhenInternetEnabled(t *testing.T) {
	t.Parallel()
	policies := newFakePolicyStore()
	policies.records["sb-1"] = store.Sandbox{SandboxID: "sb-1"}
	srv := NewServer(testConfig(), &fakeBackend{}, policies, slog.Default())

	req := httptest.NewRequest(http.MethodConnect, "http://example.com:443", nil)
	req.Host = "example.com:443"
	req.Header.Set("E2b-Sandbox-Id", "sb-1")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code == http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestEgressCONNECTAllowsInternalWhenInternetDisabled(t *testing.T) {
	t.Parallel()
	policies := newFakePolicyStore()
	policies.records["sb-1"] = store.Sandbox{
		SandboxID:           "sb-1",
		AllowInternetAccess: internetDisabled(),
	}
	srv := NewServer(testConfig(), &fakeBackend{}, policies, slog.Default())

	req := httptest.NewRequest(http.MethodConnect, "http://127.0.0.1:8080", nil)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("E2b-Sandbox-Id", "sb-1")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code == http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestEgressHTTPDeniedWhenInternetDisabled(t *testing.T) {
	t.Parallel()
	policies := newFakePolicyStore()
	policies.records["sb-1"] = store.Sandbox{
		SandboxID:           "sb-1",
		AllowInternetAccess: internetDisabled(),
	}
	srv := NewServer(testConfig(), &fakeBackend{}, policies, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.RequestURI = "http://example.com/"
	req.Header.Set("E2b-Sandbox-Id", "sb-1")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestEgressHTTPAllowsInternalWhenInternetDisabled(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(upstream.Close)

	policies := newFakePolicyStore()
	policies.records["sb-1"] = store.Sandbox{
		SandboxID:           "sb-1",
		AllowInternetAccess: internetDisabled(),
	}
	srv := NewServer(testConfig(), &fakeBackend{}, policies, slog.Default())
	srv.egressTransport = http.DefaultTransport

	targetURL := upstream.URL + "/"
	req := httptest.NewRequest(http.MethodGet, targetURL, nil)
	req.RequestURI = targetURL
	req.Header.Set("E2b-Sandbox-Id", "sb-1")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestEnvdProxyUnaffectedWhenInternetDisabled(t *testing.T) {
	t.Parallel()
	envd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(envd.Close)

	policies := newFakePolicyStore()
	policies.records["sandbox-1"] = store.Sandbox{
		SandboxID:           "sandbox-1",
		AllowInternetAccess: internetDisabled(),
	}
	actors := &fakeBackend{backend: envd.Listener.Addr().String()}
	srv := NewServer(testConfig(), actors, policies, slog.Default())
	srv.envdTransport = http.DefaultTransport

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Host = "sandbox-1.localhost"
	req.Header.Set("E2b-Sandbox-Id", "sandbox-1")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
