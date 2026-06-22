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

package logs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseEnvdQuery(t *testing.T) {
	t.Parallel()

	cases := []struct {
		query string
		ok    bool
	}{
		{"", true},
		{"start=0&limit=100&cursor=1&direction=forward&level=info&search=foo", true},
		{"start=-1", false},
		{"cursor=bad", false},
		{"limit=1001", false},
		{"direction=up", false},
		{"level=trace", false},
		{"search=" + strings.Repeat("a", MaxLogSearchLen+1), false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/logs?"+tc.query, nil)
		_, err := ParseEnvdQuery(req)
		if tc.ok && err != nil {
			t.Fatalf("query %q: err = %v, want nil", tc.query, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("query %q: err = nil, want error", tc.query)
		}
	}
}

func TestValidateV1Query(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1/logs?start=0&limit=100", nil)
	if err := ValidateV1Query(req); err != nil {
		t.Fatalf("ValidateV1Query: %v", err)
	}
}

func TestValidateV2Query(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/v2/sandboxes/sb-1/logs?cursor=0&limit=100", nil)
	if err := ValidateV2Query(req); err != nil {
		t.Fatalf("ValidateV2Query: %v", err)
	}
}

func TestBufferAppendAndQuery(t *testing.T) {
	t.Parallel()

	buf := NewBuffer(100, 1<<20)
	buf.Append("info", "hello", map[string]string{"stream": "stdout"})
	buf.Append("error", "boom", map[string]string{"stream": "stderr"})

	all := buf.Query(Query{})
	if len(all) != 2 {
		t.Fatalf("len = %d, want 2", len(all))
	}
	if all[0].Message != "hello" || all[0].Level != "info" {
		t.Fatalf("first = %+v", all[0])
	}

	infoOnly := buf.Query(Query{Level: "info"})
	if len(infoOnly) != 1 || infoOnly[0].Message != "hello" {
		t.Fatalf("infoOnly = %+v", infoOnly)
	}

	search := buf.Query(Query{Search: "boom"})
	if len(search) != 1 || search[0].Message != "boom" {
		t.Fatalf("search = %+v", search)
	}
}

func TestBufferAppendOutput(t *testing.T) {
	t.Parallel()

	buf := NewBuffer(100, 1<<20)
	buf.AppendOutput("stdout", []byte("line1\nline2\n"))
	entries := buf.Query(Query{})
	if len(entries) != 2 {
		t.Fatalf("len = %d, want 2", len(entries))
	}
	if entries[0].Fields["stream"] != "stdout" {
		t.Fatalf("fields = %+v", entries[0].Fields)
	}
}

func TestBufferAppendPTYOutputChunks(t *testing.T) {
	t.Parallel()

	buf := NewBuffer(100, 1<<20)
	buf.AppendOutput("pty", []byte("ec"))
	buf.AppendOutput("pty", []byte("ho hi\n"))
	entries := buf.Query(Query{})
	if len(entries) != 1 {
		t.Fatalf("len = %d, want 1", len(entries))
	}
	if entries[0].Message != "echo hi" {
		t.Fatalf("message = %q, want echo hi", entries[0].Message)
	}
	if entries[0].Fields["stream"] != "pty" {
		t.Fatalf("fields = %+v", entries[0].Fields)
	}
}

func TestBufferAppendPTYOutputSanitizesANSI(t *testing.T) {
	t.Parallel()

	buf := NewBuffer(100, 1<<20)
	buf.AppendOutput("pty", []byte("\x1b[31mhi\x1b[0m\n"))
	entries := buf.Query(Query{})
	if len(entries) != 1 || entries[0].Message != "hi" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestBufferEvictsOldest(t *testing.T) {
	t.Parallel()

	buf := NewBuffer(2, 1<<20)
	buf.Append("info", "a", nil)
	buf.Append("info", "b", nil)
	buf.Append("info", "c", nil)

	entries := buf.Query(Query{})
	if len(entries) != 2 {
		t.Fatalf("len = %d, want 2", len(entries))
	}
	if entries[0].Message != "b" || entries[1].Message != "c" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestBufferQueryStartAndCursor(t *testing.T) {
	t.Parallel()

	buf := NewBuffer(100, 1<<20)
	buf.Append("info", "old", nil)
	time.Sleep(2 * time.Millisecond)
	start := time.Now().UTC().UnixMilli()
	time.Sleep(2 * time.Millisecond)
	buf.Append("info", "new", nil)

	afterStart := buf.Query(Query{Start: &start})
	if len(afterStart) != 1 || afterStart[0].Message != "new" {
		t.Fatalf("afterStart = %+v", afterStart)
	}

	cursor := int64(1)
	forward := buf.Query(Query{Cursor: &cursor, Direction: "forward"})
	if len(forward) != 1 || forward[0].Message != "new" {
		t.Fatalf("forward = %+v", forward)
	}

	cursor = int64(2)
	backward := buf.Query(Query{Cursor: &cursor, Direction: "backward", Limit: 1})
	if len(backward) != 1 || backward[0].Message != "new" {
		t.Fatalf("backward = %+v", backward)
	}
}

func TestHandlerInvalidQuery(t *testing.T) {
	t.Parallel()

	buf := NewBuffer(10, 1<<10)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /logs", NewHandler(buf))

	req := httptest.NewRequest(http.MethodGet, "/logs?cursor=bad", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestToV1V2(t *testing.T) {
	t.Parallel()

	entries := []Entry{{
		Timestamp: "2026-06-20T12:00:00Z",
		Message:   "hello",
		Level:     "info",
		Fields:    map[string]string{"stream": "stdout"},
	}}
	v1 := ToV1(entries)
	if len(v1.Logs) != 1 || v1.Logs[0].Line != "hello" {
		t.Fatalf("v1 logs = %+v", v1.Logs)
	}
	if len(v1.LogEntries) != 1 || v1.LogEntries[0].Message != "hello" {
		t.Fatalf("v1 entries = %+v", v1.LogEntries)
	}
	v2 := ToV2(entries)
	if len(v2.Logs) != 1 || v2.Logs[0].Message != "hello" {
		t.Fatalf("v2 = %+v", v2)
	}
}
