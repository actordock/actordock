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
	"testing"
)

func TestValidateVolumeName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		wantErr error
	}{
		{name: "valid", input: "my-data_1"},
		{name: "empty", input: "", wantErr: ErrVolumeNameEmpty},
		{name: "invalid chars", input: "bad name", wantErr: ErrInvalidVolumeName},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateVolumeName(tc.input)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateVolumeName = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ValidateVolumeName = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateVolumeMounts(t *testing.T) {
	t.Parallel()

	volumes := map[string]Volume{
		"my-data": {VolumeID: "vol-1", Name: "my-data"},
		"vol-1":   {VolumeID: "vol-1", Name: "my-data"},
	}
	lookup := func(nameOrID string) (Volume, error) {
		vol, ok := volumes[nameOrID]
		if !ok {
			return Volume{}, ErrVolumeNotFound
		}
		return vol, nil
	}

	got, err := ValidateVolumeMounts([]VolumeMount{
		{Name: "my-data", Path: "/mnt/data"},
	}, lookup)
	if err != nil {
		t.Fatalf("ValidateVolumeMounts: %v", err)
	}
	if len(got) != 1 || got[0].Name != "my-data" || got[0].Path != "/mnt/data" {
		t.Fatalf("ValidateVolumeMounts = %+v", got)
	}

	_, err = ValidateVolumeMounts([]VolumeMount{
		{Name: "missing", Path: "/mnt/data"},
	}, lookup)
	if !errors.Is(err, ErrUnknownVolume) {
		t.Fatalf("unknown volume = %v, want ErrUnknownVolume", err)
	}

	_, err = ValidateVolumeMounts([]VolumeMount{
		{Name: "my-data", Path: "relative/path"},
	}, lookup)
	if !errors.Is(err, ErrInvalidMountPath) {
		t.Fatalf("relative path = %v, want ErrInvalidMountPath", err)
	}

	_, err = ValidateVolumeMounts([]VolumeMount{
		{Name: "my-data", Path: "/mnt/data"},
		{Name: "vol-1", Path: "/mnt/data"},
	}, lookup)
	if !errors.Is(err, ErrDuplicateMountPath) {
		t.Fatalf("duplicate path = %v, want ErrDuplicateMountPath", err)
	}
}
