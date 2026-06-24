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
	"strings"
	"testing"
)

func TestSynthesizeDockerfileFromPythonBase(t *testing.T) {
	t.Parallel()

	dockerfile, err := SynthesizeDockerfile("kind-registry:5000/base-envd@sha256:abc", []Step{
		{Type: "RUN", Args: []string{"pip install numpy"}},
		{Type: "WORKDIR", Args: []string{"/app"}},
		{Type: "COPY", Args: []string{".", "/app"}},
		{Type: "ENV", Args: []string{"FOO", "bar"}},
	})
	if err != nil {
		t.Fatalf("SynthesizeDockerfile: %v", err)
	}
	for _, want := range []string{
		"FROM kind-registry:5000/base-envd@sha256:abc",
		"RUN pip install numpy",
		"WORKDIR /app",
		"COPY . /app",
		"ENV FOO=bar",
	} {
		if !strings.Contains(dockerfile, want) {
			t.Fatalf("dockerfile missing %q:\n%s", want, dockerfile)
		}
	}
}

func TestSynthesizeDockerfileRejectsMissingBase(t *testing.T) {
	t.Parallel()
	_, err := SynthesizeDockerfile("", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
