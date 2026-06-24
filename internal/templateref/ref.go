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

import (
	"errors"
	"strings"
)

const tagSeparator = "--"

var ErrEmpty = errors.New("template reference is required")

// Parse splits "my-app:prod" into template ID and tag. No tag yields an empty tag.
func Parse(ref string) (templateID, tag string, err error) {
	raw := strings.TrimSpace(ref)
	if raw == "" {
		return "", "", ErrEmpty
	}
	if idx := strings.LastIndex(raw, ":"); idx > 0 {
		tag = strings.TrimSpace(raw[idx+1:])
		raw = raw[:idx]
	}
	templateID = strings.TrimSpace(raw)
	if templateID == "" {
		return "", "", ErrEmpty
	}
	return templateID, tag, nil
}

// ActorNameForTag returns the runtime ActorTemplate name for a tagged template.
func ActorNameForTag(templateID, tag string) string {
	templateID = strings.TrimSpace(templateID)
	tag = SanitizeTagName(tag)
	if tag == "" {
		return templateID
	}
	return templateID + tagSeparator + tag
}

// SanitizeTagName normalizes a tag for use in Kubernetes object names.
func SanitizeTagName(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range tag {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == '_' || r == '.':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return ""
	}
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}
