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
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func registerFilesHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /files", handleGetFiles)
	mux.HandleFunc("POST /files", handlePostFiles)
	mux.HandleFunc("DELETE /files", handleDeleteFiles)
}

func handleGetFiles(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	resolved, err := resolvePath(path)
	if err != nil {
		writeFilesJSONError(w, http.StatusBadRequest, err)
		return
	}

	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			writeFilesJSONError(w, http.StatusNotFound, fmt.Errorf("file not found: %w", err))
			return
		}
		writeFilesJSONError(w, http.StatusInternalServerError, fmt.Errorf("stat file: %w", err))
		return
	}

	if info.IsDir() {
		writeFilesJSONError(w, http.StatusBadRequest, fmt.Errorf("path %q is a directory", resolved))
		return
	}

	file, err := os.Open(resolved)
	if err != nil {
		writeFilesJSONError(w, http.StatusInternalServerError, fmt.Errorf("open file: %w", err))
		return
	}
	defer file.Close()

	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		contentType := mime.TypeByExtension(filepath.Ext(resolved))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", contentType)

		gw := gzip.NewWriter(w)
		defer gw.Close()
		_, _ = io.Copy(gw, file)
		return
	}

	http.ServeContent(w, r, filepath.Base(resolved), info.ModTime(), file)
}

func handlePostFiles(w http.ResponseWriter, r *http.Request) {
	body := r.Body
	defer body.Close()

	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		gr, err := gzip.NewReader(body)
		if err != nil {
			writeFilesJSONError(w, http.StatusBadRequest, fmt.Errorf("gzip decompress: %w", err))
			return
		}
		defer gr.Close()
		body = gr
	}

	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		writeFilesJSONError(w, http.StatusBadRequest, fmt.Errorf("parse content type: %w", err))
		return
	}

	var results []fileWriteInfo
	switch {
	case mediaType == "application/octet-stream":
		results, err = handleRawUpload(r, body)
	case strings.HasPrefix(mediaType, "multipart/"):
		results, err = handleMultipartUpload(r, body)
	default:
		writeFilesJSONError(
			w,
			http.StatusBadRequest,
			fmt.Errorf("unsupported content type %q, expected multipart/form-data or application/octet-stream", mediaType),
		)
		return
	}
	if err != nil {
		var fileErr *filesHTTPError
		if errors.As(err, &fileErr) {
			writeFilesJSONError(w, fileErr.code, fileErr.err)
			return
		}
		writeFilesJSONError(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		writeFilesJSONError(w, http.StatusInternalServerError, fmt.Errorf("encode response: %w", err))
	}
}

func handleRawUpload(r *http.Request, body io.Reader) ([]fileWriteInfo, error) {
	path := r.URL.Query().Get("path")
	if path == "" {
		return nil, &filesHTTPError{code: http.StatusBadRequest, err: errors.New("path query parameter is required for raw body upload")}
	}

	resolved, err := resolvePath(path)
	if err != nil {
		return nil, &filesHTTPError{code: http.StatusBadRequest, err: err}
	}

	if err := writeFileContents(resolved, body); err != nil {
		return nil, err
	}

	return []fileWriteInfo{writeHTTPFileInfo(resolved)}, nil
}

func handleMultipartUpload(r *http.Request, body io.Reader) ([]fileWriteInfo, error) {
	reader := multipart.NewReader(body, multipartBoundary(r.Header.Get("Content-Type")))
	queryPath := r.URL.Query().Get("path")

	var results []fileWriteInfo
	seen := make(map[string]struct{})

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read multipart: %w", err)
		}

		if part.FormName() != "file" {
			_ = part.Close()
			continue
		}

		targetPath := queryPath
		if targetPath == "" {
			targetPath = part.FileName()
		}
		if targetPath == "" {
			_ = part.Close()
			return nil, &filesHTTPError{code: http.StatusBadRequest, err: errors.New("missing file path")}
		}

		resolved, err := resolvePath(targetPath)
		if err != nil {
			_ = part.Close()
			return nil, &filesHTTPError{code: http.StatusBadRequest, err: err}
		}
		if _, ok := seen[resolved]; ok {
			_ = part.Close()
			return nil, &filesHTTPError{
				code: http.StatusBadRequest,
				err:  fmt.Errorf("duplicate upload path %q in one request", resolved),
			}
		}

		if err := writeFileContents(resolved, part); err != nil {
			_ = part.Close()
			return nil, err
		}
		_ = part.Close()

		seen[resolved] = struct{}{}
		results = append(results, writeHTTPFileInfo(resolved))
	}

	if len(results) == 0 {
		return nil, &filesHTTPError{code: http.StatusBadRequest, err: errors.New("no files in upload")}
	}

	return results, nil
}

func writeFileContents(path string, r io.Reader) error {
	if err := ensureParentDirs(path); err != nil {
		return &filesHTTPError{code: http.StatusInternalServerError, err: fmt.Errorf("create parent dirs: %w", err)}
	}

	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return &filesHTTPError{code: http.StatusBadRequest, err: fmt.Errorf("path is a directory: %s", path)}
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return &filesHTTPError{code: http.StatusInternalServerError, err: fmt.Errorf("open file: %w", err)}
	}
	defer file.Close()

	if _, err := io.Copy(file, r); err != nil {
		return &filesHTTPError{code: http.StatusInternalServerError, err: fmt.Errorf("write file: %w", err)}
	}

	return nil
}

func handleDeleteFiles(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	resolved, err := resolvePath(path)
	if err != nil {
		writeFilesJSONError(w, http.StatusBadRequest, err)
		return
	}

	if err := os.RemoveAll(resolved); err != nil {
		if os.IsNotExist(err) {
			writeFilesJSONError(w, http.StatusNotFound, fmt.Errorf("file not found: %w", err))
			return
		}
		writeFilesJSONError(w, http.StatusInternalServerError, fmt.Errorf("remove path: %w", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type filesHTTPError struct {
	code int
	err  error
}

func (e *filesHTTPError) Error() string {
	return e.err.Error()
}

func multipartBoundary(contentType string) string {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	return params["boundary"]
}

func writeFilesJSONError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    code,
		"message": err.Error(),
	})
}
