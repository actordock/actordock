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

import "testing"

func TestRewriteLocalRegistry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ref, replacement, want string
	}{
		{
			ref:          "localhost:5001/envd@sha256:abc",
			replacement:  "kind-registry:5000",
			want:         "kind-registry:5000/envd@sha256:abc",
		},
		{
			ref:          "127.0.0.1:5001/envd@sha256:abc",
			replacement:  "kind-registry:5000",
			want:         "kind-registry:5000/envd@sha256:abc",
		},
		{
			ref:          "kind-registry:5000/envd@sha256:abc",
			replacement:  "kind-registry:5000",
			want:         "kind-registry:5000/envd@sha256:abc",
		},
		{
			ref:          "localhost:5001/envd@sha256:abc",
			replacement:  "",
			want:         "localhost:5001/envd@sha256:abc",
		},
	}
	for _, tc := range tests {
		if got := RewriteLocalRegistry(tc.ref, tc.replacement); got != tc.want {
			t.Fatalf("RewriteLocalRegistry(%q, %q) = %q, want %q", tc.ref, tc.replacement, got, tc.want)
		}
	}
}

func TestRewriteRegistryPrefix(t *testing.T) {
	t.Parallel()

	got := RewriteRegistryPrefix(
		"kind-registry:5000/actordock/templates/app@sha256:abc",
		"kind-registry:5000",
		"localhost:5001",
	)
	want := "localhost:5001/actordock/templates/app@sha256:abc"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
