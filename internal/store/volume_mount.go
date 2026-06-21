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

package store

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

var (
	ErrInvalidVolumeName  = errors.New("invalid volume name")
	ErrInvalidMountPath   = errors.New("invalid volume mount path")
	ErrDuplicateMountPath = errors.New("duplicate volume mount path")
	ErrUnknownVolume      = errors.New("volume not found")
	volumeNamePattern     = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

// VolumeMount is a persisted sandbox volume mount (OpenAPI SandboxVolumeMount).
type VolumeMount struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ValidateVolumeName checks E2B NewVolume name constraints.
func ValidateVolumeName(name string) error {
	if name == "" {
		return ErrVolumeNameEmpty
	}
	if !volumeNamePattern.MatchString(name) {
		return ErrInvalidVolumeName
	}
	return nil
}

// ValidateVolumeMounts checks mount paths and resolves each name to an existing volume.
func ValidateVolumeMounts(mounts []VolumeMount, lookup func(nameOrID string) (Volume, error)) ([]VolumeMount, error) {
	if len(mounts) == 0 {
		return nil, nil
	}

	seenPaths := make(map[string]struct{}, len(mounts))
	resolved := make([]VolumeMount, 0, len(mounts))
	for i, mount := range mounts {
		if strings.TrimSpace(mount.Name) == "" {
			return nil, fmt.Errorf("volumeMounts[%d]: name is required", i)
		}
		if err := validateMountPath(mount.Path, i); err != nil {
			return nil, err
		}
		if _, ok := seenPaths[mount.Path]; ok {
			return nil, ErrDuplicateMountPath
		}
		seenPaths[mount.Path] = struct{}{}

		vol, err := lookup(mount.Name)
		if errors.Is(err, ErrVolumeNotFound) {
			return nil, ErrUnknownVolume
		}
		if err != nil {
			return nil, err
		}

		resolved = append(resolved, VolumeMount{
			Name: vol.Name,
			Path: mount.Path,
		})
	}
	return resolved, nil
}

func validateMountPath(mountPath string, index int) error {
	if mountPath == "" {
		return fmt.Errorf("volumeMounts[%d]: path is required", index)
	}
	if !path.IsAbs(mountPath) {
		return ErrInvalidMountPath
	}
	clean := path.Clean(mountPath)
	if strings.Contains(clean, "..") {
		return ErrInvalidMountPath
	}
	return nil
}
