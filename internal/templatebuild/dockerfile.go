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
	"fmt"
	"strings"
)

// SynthesizeDockerfile renders a Dockerfile from a base image and E2B build steps.
func SynthesizeDockerfile(baseImage string, steps []Step) (string, error) {
	baseImage = strings.TrimSpace(baseImage)
	if baseImage == "" {
		return "", fmt.Errorf("base image is required")
	}

	var b strings.Builder
	b.WriteString("FROM ")
	b.WriteString(baseImage)
	b.WriteByte('\n')

	for i, step := range steps {
		line, err := renderStep(step)
		if err != nil {
			return "", fmt.Errorf("step %d: %w", i, err)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func renderStep(step Step) (string, error) {
	stepType := strings.ToUpper(strings.TrimSpace(step.Type))
	args := step.Args
	switch stepType {
	case "RUN":
		if len(args) == 0 {
			return "", fmt.Errorf("RUN requires args")
		}
		cmd := args[0]
		if len(args) > 1 {
			cmd = strings.Join(args, " ")
		}
		return "RUN " + cmd, nil
	case "COPY":
		if len(args) < 2 {
			return "", fmt.Errorf("COPY requires src and dest")
		}
		return fmt.Sprintf("COPY %s %s", args[0], args[1]), nil
	case "ENV":
		if len(args) < 2 || len(args)%2 != 0 {
			return "", fmt.Errorf("ENV requires key/value pairs")
		}
		pairs := make([]string, 0, len(args)/2)
		for i := 0; i < len(args); i += 2 {
			pairs = append(pairs, args[i]+"="+args[i+1])
		}
		return "ENV " + strings.Join(pairs, " "), nil
	case "WORKDIR", "USER":
		if len(args) == 0 {
			return "", fmt.Errorf("%s requires args", stepType)
		}
		return stepType + " " + strings.Join(args, " "), nil
	default:
		return "", fmt.Errorf("unsupported step type %q", step.Type)
	}
}
