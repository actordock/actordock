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

package templatebuild

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// PrepareContextDir writes the Dockerfile and extracts COPY layer tars into dir.
func PrepareContextDir(dir, dockerfile string, filesDir string, steps []Step) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir context: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		return fmt.Errorf("write Dockerfile: %w", err)
	}
	seen := make(map[string]struct{})
	for _, step := range steps {
		if strings.ToUpper(strings.TrimSpace(step.Type)) != "COPY" {
			continue
		}
		hash := strings.TrimSpace(step.FilesHash)
		if hash == "" {
			continue
		}
		if _, ok := seen[hash]; ok {
			continue
		}
		seen[hash] = struct{}{}
		tarPath := filepath.Join(filesDir, hash+".tar")
		if err := extractTarGz(tarPath, dir); err != nil {
			return fmt.Errorf("extract filesHash %q: %w", hash, err)
		}
	}
	return nil
}

func extractTarGz(tarPath, destDir string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		target := filepath.Join(destDir, filepath.Clean(hdr.Name))
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) && target != filepath.Clean(destDir) {
			return fmt.Errorf("tar entry escapes context: %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
}
