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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/actordock/actordock/internal/logs"
	processv1 "github.com/actordock/actordock/pkg/envd/process"
	"github.com/actordock/actordock/pkg/envd/process/processv1connect"
)

func TestGetLogsAfterProcessStart(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	logBuf := logs.NewBuffer(logs.DefaultMaxLines, logs.DefaultMaxBytes)
	mux.HandleFunc("GET /logs", logs.NewHandler(logBuf))
	path, handler := processv1connect.NewProcessHandler(&processService{logs: logBuf})
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
	for stream.Receive() {
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, server.URL+"/logs", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("logs status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp logs.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Logs) != 1 {
		t.Fatalf("logs = %+v, want 1 entry", resp.Logs)
	}
	if resp.Logs[0].Message != "hello" {
		t.Fatalf("message = %q, want hello", resp.Logs[0].Message)
	}
	if resp.Logs[0].Level != "info" || resp.Logs[0].Fields["stream"] != "stdout" {
		t.Fatalf("entry = %+v", resp.Logs[0])
	}
}

func TestGetLogsLevelFilter(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	logBuf := logs.NewBuffer(logs.DefaultMaxLines, logs.DefaultMaxBytes)
	mux.HandleFunc("GET /logs", logs.NewHandler(logBuf))
	path, handler := processv1connect.NewProcessHandler(&processService{logs: logBuf})
	mux.Handle(path, handler)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := processv1connect.NewProcessClient(server.Client(), server.URL)
	stream, err := client.Start(context.Background(), connect.NewRequest(&processv1.StartRequest{
		Process: &processv1.ProcessConfig{
			Cmd:  "/bin/sh",
			Args: []string{"-c", "echo out; echo err 1>&2"},
		},
	}))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	for stream.Receive() {
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, server.URL+"/logs?level=error", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var resp logs.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Logs) != 1 || resp.Logs[0].Message != "err" {
		t.Fatalf("logs = %+v", resp.Logs)
	}
}
