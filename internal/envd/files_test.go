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
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	filesystemv1 "github.com/actordock/actordock/pkg/envd/filesystem"
	"github.com/actordock/actordock/pkg/envd/filesystem/filesystemv1connect"
)

func newFilesTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	registerFilesHandlers(mux)
	fsPath, fsHandler := filesystemv1connect.NewFilesystemHandler(filesystemService{})
	mux.Handle(fsPath, fsHandler)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestFilesWriteReadRoundTrip(t *testing.T) {
	t.Parallel()

	server := newFilesTestServer(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "nested", "hello.txt")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", target)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write([]byte("hello files")); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	postReq, err := http.NewRequest(http.MethodPost, server.URL+"/files?path="+target, body)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	postReq.Header.Set("Content-Type", writer.FormDataContentType())

	postRec := httptest.NewRecorder()
	server.Config.Handler.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, body = %s", postRec.Code, postRec.Body.String())
	}

	var writeResp []fileWriteInfo
	if err := json.Unmarshal(postRec.Body.Bytes(), &writeResp); err != nil {
		t.Fatalf("unmarshal write response: %v", err)
	}
	if len(writeResp) != 1 || writeResp[0].Path != target || writeResp[0].Type != "file" {
		t.Fatalf("writeResp = %+v", writeResp)
	}

	getReq := httptest.NewRequest(http.MethodGet, server.URL+"/files?path="+target, nil)
	getRec := httptest.NewRecorder()
	server.Config.Handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", getRec.Code, getRec.Body.String())
	}
	if got := getRec.Body.String(); got != "hello files" {
		t.Fatalf("GET body = %q, want %q", got, "hello files")
	}
}

func TestFilesOctetStreamWriteRead(t *testing.T) {
	t.Parallel()

	server := newFilesTestServer(t)
	target := filepath.Join(t.TempDir(), "raw.bin")

	postReq, err := http.NewRequest(
		http.MethodPost,
		server.URL+"/files?path="+target,
		bytes.NewReader([]byte("raw-data")),
	)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	postReq.Header.Set("Content-Type", "application/octet-stream")

	postRec := httptest.NewRecorder()
	server.Config.Handler.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, body = %s", postRec.Code, postRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, server.URL+"/files?path="+target, nil)
	getRec := httptest.NewRecorder()
	server.Config.Handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d", getRec.Code)
	}
	if got := getRec.Body.String(); got != "raw-data" {
		t.Fatalf("GET body = %q, want %q", got, "raw-data")
	}
}

func TestFilesystemListDir(t *testing.T) {
	t.Parallel()

	server := newFilesTestServer(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatalf("write b.txt: %v", err)
	}

	client := filesystemv1connect.NewFilesystemClient(server.Client(), server.URL)
	resp, err := client.ListDir(context.Background(), connect.NewRequest(&filesystemv1.ListDirRequest{
		Path:  dir,
		Depth: 2,
	}))
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}

	names := make(map[string]struct{})
	for _, entry := range resp.Msg.GetEntries() {
		names[entry.GetName()] = struct{}{}
		if entry.GetPath() == "" {
			t.Fatalf("entry missing path: %+v", entry)
		}
	}

	for _, want := range []string{"a.txt", "sub", "b.txt"} {
		if _, ok := names[want]; !ok {
			t.Fatalf("entries = %v, missing %q", names, want)
		}
	}
}

func TestFilesystemStat(t *testing.T) {
	t.Parallel()

	server := newFilesTestServer(t)
	target := filepath.Join(t.TempDir(), "stat.txt")
	if err := os.WriteFile(target, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	client := filesystemv1connect.NewFilesystemClient(server.Client(), server.URL)
	resp, err := client.Stat(context.Background(), connect.NewRequest(&filesystemv1.StatRequest{Path: target}))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	entry := resp.Msg.GetEntry()
	if entry.GetName() != "stat.txt" {
		t.Fatalf("name = %q", entry.GetName())
	}
	if entry.GetType() != filesystemv1.FileType_FILE_TYPE_FILE {
		t.Fatalf("type = %v, want FILE", entry.GetType())
	}
	if entry.GetSize() != int64(len("payload")) {
		t.Fatalf("size = %d", entry.GetSize())
	}
}

func TestDeleteFiles(t *testing.T) {
	t.Parallel()

	server := newFilesTestServer(t)
	target := filepath.Join(t.TempDir(), "remove-me.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	delReq := httptest.NewRequest(http.MethodDelete, server.URL+"/files?path="+target, nil)
	delRec := httptest.NewRecorder()
	server.Config.Handler.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, body = %s", delRec.Code, delRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, server.URL+"/files?path="+target, nil)
	getRec := httptest.NewRecorder()
	server.Config.Handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("GET after delete status = %d", getRec.Code)
	}
}
