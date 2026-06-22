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
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"

	"connectrpc.com/connect"
	filesystemv1 "github.com/actordock/actordock/pkg/envd/filesystem"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func defaultWorkDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return "/"
}

func resolvePath(requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", fmt.Errorf("path is required")
	}

	var resolved string
	if filepath.IsAbs(requested) {
		resolved = filepath.Clean(requested)
	} else {
		resolved = filepath.Clean(filepath.Join(defaultWorkDir(), requested))
	}

	if !filepath.IsAbs(resolved) {
		return "", fmt.Errorf("invalid path %q", requested)
	}

	return resolved, nil
}

func statEntry(path string) (*filesystemv1.EntryInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("file not found: %w", err))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("stat file: %w", err))
	}

	return entryInfoFromStat(path, info)
}

func entryInfoFromStat(path string, info fs.FileInfo) (*filesystemv1.EntryInfo, error) {
	entry := &filesystemv1.EntryInfo{
		Name:         info.Name(),
		Path:         path,
		Size:         info.Size(),
		Mode:         uint32(info.Mode()),
		Permissions:  formatPermissions(info.Mode()),
		ModifiedTime: timestamppb.New(info.ModTime()),
	}

	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read symlink: %w", err))
		}
		entry.SymlinkTarget = &target
		targetInfo, err := os.Stat(path)
		if err != nil {
			entry.Type = filesystemv1.FileType_FILE_TYPE_UNSPECIFIED
		} else if targetInfo.IsDir() {
			entry.Type = filesystemv1.FileType_FILE_TYPE_DIRECTORY
		} else {
			entry.Type = filesystemv1.FileType_FILE_TYPE_FILE
		}
	case info.IsDir():
		entry.Type = filesystemv1.FileType_FILE_TYPE_DIRECTORY
	default:
		entry.Type = filesystemv1.FileType_FILE_TYPE_FILE
	}

	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		entry.Owner, entry.Group = lookupOwnerGroup(stat.Uid, stat.Gid)
	}

	return entry, nil
}

func lookupOwnerGroup(uid, gid uint32) (owner, group string) {
	owner = fmt.Sprintf("%d", uid)
	if u, err := user.LookupId(owner); err == nil {
		owner = u.Username
	}

	group = fmt.Sprintf("%d", gid)
	if g, err := user.LookupGroupId(group); err == nil {
		group = g.Name
	}

	return owner, group
}

func formatPermissions(mode fs.FileMode) string {
	const permBits = "rwxrwxrwx"
	var b strings.Builder
	b.Grow(9)

	fileMode := mode.Perm()
	for i := 0; i < 9; i++ {
		if fileMode&(1<<uint(8-i)) != 0 {
			b.WriteByte(permBits[i])
		} else {
			b.WriteByte('-')
		}
	}

	return b.String()
}

func ensureParentDirs(path string) error {
	parent := filepath.Dir(path)
	if parent == "" || parent == "." {
		return nil
	}
	return os.MkdirAll(parent, 0o755)
}

func writeHTTPFileInfo(path string) fileWriteInfo {
	return fileWriteInfo{
		Name: filepath.Base(path),
		Type: "file",
		Path: path,
	}
}

type fileWriteInfo struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Path     string            `json:"path"`
	Metadata map[string]string `json:"metadata,omitempty"`
}
