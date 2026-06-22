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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"
	filesystemv1 "github.com/actordock/actordock/pkg/envd/filesystem"
)

type filesystemService struct{}

func (filesystemService) Stat(
	_ context.Context,
	req *connect.Request[filesystemv1.StatRequest],
) (*connect.Response[filesystemv1.StatResponse], error) {
	resolved, err := resolvePath(req.Msg.GetPath())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	entry, err := statEntry(resolved)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&filesystemv1.StatResponse{Entry: entry}), nil
}

func (filesystemService) ListDir(
	_ context.Context,
	req *connect.Request[filesystemv1.ListDirRequest],
) (*connect.Response[filesystemv1.ListDirResponse], error) {
	depth := req.Msg.GetDepth()
	if depth == 0 {
		depth = 1
	}

	requested, err := resolvePath(req.Msg.GetPath())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	resolved, err := filepath.EvalSymlinks(requested)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("directory not found: %w", err))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("resolve symlink: %w", err))
	}

	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("directory not found: %w", err))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("stat directory: %w", err))
	}
	if !info.IsDir() {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path is not a directory: %s", requested))
	}

	entries, err := walkDir(requested, resolved, int(depth))
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&filesystemv1.ListDirResponse{Entries: entries}), nil
}

func (filesystemService) Remove(
	_ context.Context,
	req *connect.Request[filesystemv1.RemoveRequest],
) (*connect.Response[filesystemv1.RemoveResponse], error) {
	resolved, err := resolvePath(req.Msg.GetPath())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := os.RemoveAll(resolved); err != nil {
		if os.IsNotExist(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("file not found: %w", err))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("remove path: %w", err))
	}

	return connect.NewResponse(&filesystemv1.RemoveResponse{}), nil
}

func (filesystemService) MakeDir(
	context.Context,
	*connect.Request[filesystemv1.MakeDirRequest],
) (*connect.Response[filesystemv1.MakeDirResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("MakeDir is not implemented"))
}

func (filesystemService) Move(
	context.Context,
	*connect.Request[filesystemv1.MoveRequest],
) (*connect.Response[filesystemv1.MoveResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("Move is not implemented"))
}

func (filesystemService) WatchDir(
	context.Context,
	*connect.Request[filesystemv1.WatchDirRequest],
	*connect.ServerStream[filesystemv1.WatchDirResponse],
) error {
	return connect.NewError(connect.CodeUnimplemented, errors.New("WatchDir is not implemented"))
}

func (filesystemService) CreateWatcher(
	context.Context,
	*connect.Request[filesystemv1.CreateWatcherRequest],
) (*connect.Response[filesystemv1.CreateWatcherResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("CreateWatcher is not implemented"))
}

func (filesystemService) GetWatcherEvents(
	context.Context,
	*connect.Request[filesystemv1.GetWatcherEventsRequest],
) (*connect.Response[filesystemv1.GetWatcherEventsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("GetWatcherEvents is not implemented"))
}

func (filesystemService) RemoveWatcher(
	context.Context,
	*connect.Request[filesystemv1.RemoveWatcherRequest],
) (*connect.Response[filesystemv1.RemoveWatcherResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("RemoveWatcher is not implemented"))
}

func walkDir(requestedPath, dirPath string, depth int) ([]*filesystemv1.EntryInfo, error) {
	var entries []*filesystemv1.EntryInfo

	err := filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dirPath {
			return nil
		}

		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}
		currentDepth := len(strings.Split(relPath, string(os.PathSeparator)))
		if currentDepth > depth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		displayPath := filepath.Join(requestedPath, relPath)
		entry, err := entryInfoFromStat(displayPath, info)
		if err != nil {
			var connectErr *connect.Error
			if errors.As(err, &connectErr) && connectErr.Code() == connect.CodeNotFound {
				return nil
			}
			return err
		}

		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read directory %s: %w", dirPath, err))
	}

	return entries, nil
}
