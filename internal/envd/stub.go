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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/actordock/actordock/internal/logs"
	"github.com/actordock/actordock/pkg/envd/process/processv1connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// NewStubHandler serves /health and the process Connect API for readiness probes.
func NewStubHandler() http.Handler {
	logBuf := logs.NewBuffer(logs.DefaultMaxLines, logs.DefaultMaxBytes)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /logs", logs.NewHandler(logBuf))
	path, handler := processv1connect.NewProcessHandler(&processService{logs: logBuf})
	mux.Handle(path, handler)
	return h2c.NewHandler(mux, &http2.Server{})
}

// StartStubTestBackend starts a local envd stub and returns host:port.
func StartStubTestBackend(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(NewStubHandler())
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().String()
}
