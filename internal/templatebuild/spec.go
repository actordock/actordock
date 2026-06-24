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
	"encoding/json"
	"fmt"
	"strings"
)

// StartSpec is the E2B TemplateBuildStartV2 request body stored in TemplateBuild.StepsJSON.
type StartSpec struct {
	FromImage    string `json:"fromImage,omitempty"`
	FromTemplate string `json:"fromTemplate,omitempty"`
	Force        bool   `json:"force,omitempty"`
	Steps        []Step `json:"steps,omitempty"`
	StartCmd     string `json:"startCmd,omitempty"`
	ReadyCmd     string `json:"readyCmd,omitempty"`
}

// Step is a single E2B template build instruction.
type Step struct {
	Type      string   `json:"type"`
	Args      []string `json:"args,omitempty"`
	FilesHash string   `json:"filesHash,omitempty"`
	Force     bool     `json:"force,omitempty"`
}

func ParseStartSpec(raw json.RawMessage) (StartSpec, error) {
	if len(raw) == 0 {
		return StartSpec{}, fmt.Errorf("build steps are required")
	}
	var spec StartSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return StartSpec{}, fmt.Errorf("parse build steps: %w", err)
	}
	fromTemplate := strings.TrimSpace(spec.FromTemplate)
	fromImage := strings.TrimSpace(spec.FromImage)
	if fromTemplate == "" && fromImage == "" {
		return StartSpec{}, fmt.Errorf("fromTemplate or fromImage is required")
	}
	return spec, nil
}
