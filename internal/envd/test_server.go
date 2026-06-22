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
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/actordock/actordock/pkg/envd/filesystem/filesystemv1connect"
	"github.com/actordock/actordock/pkg/envd/process/processv1connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// NewTestHandler returns an envd HTTP handler stack for integration tests.
func NewTestHandler(logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	guard := &accessGuard{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /init", guard.handleInit)
	registerFilesHandlers(mux)

	processPath, processHandler := processv1connect.NewProcessHandler(
		&processService{logger: logger},
		connect.WithCompressMinBytes(1024),
	)
	mux.Handle(processPath, processHandler)

	filesystemPath, filesystemHandler := filesystemv1connect.NewFilesystemHandler(
		filesystemService{},
		connect.WithCompressMinBytes(1024),
	)
	mux.Handle(filesystemPath, filesystemHandler)

	return h2c.NewHandler(guard.middleware(mux), &http2.Server{})
}
