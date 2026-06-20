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

	"github.com/actordock/actordock/internal/logs"
)

func TestFetchLogs(t *testing.T) {
	t.Parallel()

	logBuf := logs.NewBuffer(100, 1<<20)
	logBuf.Append("info", "hello", map[string]string{"stream": "stdout"})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /logs", logs.NewHandler(logBuf))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	entries, err := FetchLogs(context.Background(), srv.URL, "")
	if err != nil {
		t.Fatalf("FetchLogs: %v", err)
	}
	if len(entries) != 1 || entries[0].Message != "hello" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestFetchLogsForwardsQuery(t *testing.T) {
	t.Parallel()

	logBuf := logs.NewBuffer(100, 1<<20)
	logBuf.Append("info", "out", map[string]string{"stream": "stdout"})
	logBuf.Append("error", "err", map[string]string{"stream": "stderr"})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /logs", logs.NewHandler(logBuf))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	entries, err := FetchLogs(context.Background(), srv.URL, "level=error")
	if err != nil {
		t.Fatalf("FetchLogs: %v", err)
	}
	if len(entries) != 1 || entries[0].Message != "err" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestFetchLogsInvalidStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	_, err := FetchLogs(context.Background(), srv.URL, "")
	if err == nil {
		t.Fatal("FetchLogs() = nil, want error")
	}
}

func TestFetchLogsDecode(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(logs.Response{Logs: []logs.Entry{}})
	}))
	t.Cleanup(srv.Close)

	entries, err := FetchLogs(context.Background(), srv.URL, "")
	if err != nil {
		t.Fatalf("FetchLogs: %v", err)
	}
	if entries == nil {
		t.Fatal("entries is nil")
	}
}
