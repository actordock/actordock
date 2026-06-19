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
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	processv1 "github.com/actordock/actordock/pkg/envd/process"
	"github.com/actordock/actordock/pkg/envd/process/processv1connect"
)

func TestHealthReturns204(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestProcessStartEchoHello(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	path, handler := processv1connect.NewProcessHandler(&processService{})
	mux.Handle(path, handler)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := processv1connect.NewProcessClient(server.Client(), server.URL)
	stream, err := client.Start(context.Background(), connect.NewRequest(&processv1.StartRequest{
		Process: &processv1.ProcessConfig{
			Cmd:  "/bin/sh",
			Args: []string{"-c", "echo hello"},
		},
	}))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	var gotStdout string
	for stream.Receive() {
		msg := stream.Msg()
		if data := msg.GetEvent().GetData(); data != nil {
			if stdout := data.GetStdout(); len(stdout) > 0 {
				gotStdout += string(stdout)
			}
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream: %v", err)
	}
	if gotStdout != "hello\n" {
		t.Fatalf("stdout = %q, want %q", gotStdout, "hello\n")
	}
}
