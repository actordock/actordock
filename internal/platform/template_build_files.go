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

package platform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/actordock/actordock/internal/store"
)

type templateBuildPersistence interface {
	PutTemplateBuild(ctx context.Context, build store.TemplateBuild) error
	GetLatestTemplateBuild(ctx context.Context, templateID string) (store.TemplateBuild, error)
	PutTemplateBuildFile(ctx context.Context, file store.TemplateBuildFile) error
	GetTemplateBuildFile(ctx context.Context, filesHash string) (store.TemplateBuildFile, error)
	MarkTemplateBuildFilePresent(ctx context.Context, filesHash string, present bool) error
}

type templateBuildFileStorage struct {
	dir string
}

func newTemplateBuildFileStorage(dir string) (*templateBuildFileStorage, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("template build files dir is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir template build files dir: %w", err)
	}
	return &templateBuildFileStorage{dir: dir}, nil
}

func buildFileObjectKey(filesHash string) string {
	return "build-files/" + filesHash + ".tar"
}

func (f *templateBuildFileStorage) path(filesHash string) string {
	return filepath.Join(f.dir, filesHash+".tar")
}

func (f *templateBuildFileStorage) exists(filesHash string) bool {
	_, err := os.Stat(f.path(filesHash))
	return err == nil
}

func (f *templateBuildFileStorage) write(filesHash string, r io.Reader) error {
	path := f.path(filesHash)
	tmp := path + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create template build file: %w", err)
	}
	if _, err := io.Copy(out, r); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write template build file: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close template build file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename template build file: %w", err)
	}
	return nil
}

func (s *Server) handlePutTemplateBuildFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	filesHash := strings.TrimSpace(r.PathValue("hash"))
	if filesHash == "" {
		writeAPIError(w, http.StatusBadRequest, "hash is required")
		return
	}
	if s.buildFiles == nil {
		writeAPIError(w, http.StatusInternalServerError, "template build file storage unavailable")
		return
	}

	bs, ok := s.store.(templateBuildPersistence)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "template build store unavailable")
		return
	}

	ctx := r.Context()
	if _, err := bs.GetTemplateBuildFile(ctx, filesHash); errors.Is(err, store.ErrTemplateBuildFileNotFound) {
		writeAPIError(w, http.StatusNotFound, "template build file not found")
		return
	} else if err != nil {
		s.logger.Error("put template build file", "hash", filesHash, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to store template build file")
		return
	}

	if err := s.buildFiles.write(filesHash, r.Body); err != nil {
		s.logger.Error("put template build file", "hash", filesHash, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to store template build file")
		return
	}
	if err := bs.MarkTemplateBuildFilePresent(ctx, filesHash, true); err != nil {
		s.logger.Error("mark template build file present", "hash", filesHash, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to store template build file")
		return
	}

	w.WriteHeader(http.StatusOK)
}
