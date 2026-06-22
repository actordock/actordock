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

package envd

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	processv1 "github.com/actordock/actordock/pkg/envd/process"
	"github.com/actordock/actordock/pkg/envd/process/processv1connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func newSecuredEnvdServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()

	guard := &accessGuard{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /init", guard.handleInit)
	path, handler := processv1connect.NewProcessHandler(&processService{logger: slog.Default()})
	mux.Handle(path, handler)

	srv := httptest.NewServer(h2c.NewHandler(guard.middleware(mux), &http2.Server{}))
	t.Cleanup(srv.Close)

	const token = "test-envd-token"
	if err := ConfigureAccessToken(context.Background(), srv.URL, token); err != nil {
		t.Fatalf("ConfigureAccessToken: %v", err)
	}
	return srv, token
}

func TestAccessGuardRejectsMissingToken(t *testing.T) {
	t.Parallel()

	srv, token := newSecuredEnvdServer(t)
	_ = token

	client := processv1connect.NewProcessClient(srv.Client(), srv.URL)
	_, err := client.List(context.Background(), connect.NewRequest(&processv1.ListRequest{}))
	if err == nil {
		t.Fatal("List without token: expected error")
	}
	if connect.CodeOf(err) != connect.CodeUnauthenticated && connect.CodeOf(err) != connect.CodePermissionDenied {
		// connect maps 401 to unauthenticated
		if connect.CodeOf(err) != connect.CodeUnknown {
			t.Fatalf("List error code = %v, want auth failure", connect.CodeOf(err))
		}
	}
}

func TestAccessGuardAllowsValidToken(t *testing.T) {
	t.Parallel()

	srv, token := newSecuredEnvdServer(t)

	req := connect.NewRequest(&processv1.ListRequest{})
	req.Header().Set(accessTokenHeader, token)

	client := processv1connect.NewProcessClient(srv.Client(), srv.URL)
	_, err := client.List(context.Background(), req)
	if err != nil {
		t.Fatalf("List with token: %v", err)
	}
}

func TestConfigureAccessToken(t *testing.T) {
	t.Parallel()

	guard := &accessGuard{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /init", guard.handleInit)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(h2c.NewHandler(guard.middleware(mux), &http2.Server{}))
	t.Cleanup(srv.Close)

	if err := ConfigureAccessToken(context.Background(), srv.URL, "secret"); err != nil {
		t.Fatalf("ConfigureAccessToken: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/files?path=/tmp/x", nil)
	guard.middleware(mux).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /files without token status = %d, want 401", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/files?path=/tmp/x", nil)
	req.Header.Set(accessTokenHeader, "secret")
	guard.middleware(mux).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
		// path missing file is fine; auth passed before handler logic
		if rec.Code == http.StatusUnauthorized {
			t.Fatalf("GET /files with token status = %d, want not 401", rec.Code)
		}
	}
}
