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

package templateref

import "testing"

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ref     string
		id      string
		tag     string
		wantErr bool
	}{
		{ref: "my-app", id: "my-app"},
		{ref: "my-app:prod", id: "my-app", tag: "prod"},
		{ref: "  my-app:v1  ", id: "my-app", tag: "v1"},
		{ref: "", wantErr: true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.ref, func(t *testing.T) {
			t.Parallel()
			id, tag, err := Parse(tc.ref)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if id != tc.id || tag != tc.tag {
				t.Fatalf("Parse(%q) = %q,%q", tc.ref, id, tag)
			}
		})
	}
}

func TestActorNameForTag(t *testing.T) {
	t.Parallel()
	if got := ActorNameForTag("my-app", "prod"); got != "my-app--prod" {
		t.Fatalf("got %q", got)
	}
	if got := ActorNameForTag("my-app", ""); got != "my-app" {
		t.Fatalf("got %q", got)
	}
	if got := ActorNameForTag("my-app", "v1.0"); got != "my-app--v1-0" {
		t.Fatalf("got %q", got)
	}
}
