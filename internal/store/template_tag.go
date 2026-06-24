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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

var (
	ErrTemplateTagNotFound = errors.New("template tag not found")
	ErrTemplateTagEmpty    = errors.New("template tag is required")
)

const (
	templateTagKeyPrefix    = "actordock:template-tag:"
	templateTagIndexPrefix  = "actordock:template-tags:"
)

// TemplateTagRecord maps a template tag to a build.
type TemplateTagRecord struct {
	TemplateID string    `json:"template_id"`
	Tag        string    `json:"tag"`
	BuildID    string    `json:"build_id"`
	CreatedAt  time.Time `json:"created_at"`
}

func templateTagKey(templateID, tag string) string {
	return templateTagKeyPrefix + templateID + ":" + tag
}

func templateTagIndexKey(templateID string) string {
	return templateTagIndexPrefix + templateID
}

func (r *Redis) PutTemplateTag(ctx context.Context, rec TemplateTagRecord) error {
	if strings.TrimSpace(rec.TemplateID) == "" {
		return ErrCatalogTemplateIDEmpty
	}
	if strings.TrimSpace(rec.Tag) == "" {
		return ErrTemplateTagEmpty
	}
	if strings.TrimSpace(rec.BuildID) == "" {
		return ErrTemplateBuildIDEmpty
	}
	if rec.CreatedAt.IsZero() {
		return fmt.Errorf("template tag created_at is required")
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal template tag: %w", err)
	}
	pipe := r.client.TxPipeline()
	pipe.Set(ctx, templateTagKey(rec.TemplateID, rec.Tag), data, 0)
	pipe.SAdd(ctx, templateTagIndexKey(rec.TemplateID), rec.Tag)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis put template tag %s/%s: %w", rec.TemplateID, rec.Tag, err)
	}
	return nil
}

func (r *Redis) GetTemplateTag(ctx context.Context, templateID, tag string) (TemplateTagRecord, error) {
	if strings.TrimSpace(templateID) == "" {
		return TemplateTagRecord{}, ErrCatalogTemplateIDEmpty
	}
	if strings.TrimSpace(tag) == "" {
		return TemplateTagRecord{}, ErrTemplateTagEmpty
	}
	data, err := r.client.Get(ctx, templateTagKey(templateID, tag)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return TemplateTagRecord{}, ErrTemplateTagNotFound
	}
	if err != nil {
		return TemplateTagRecord{}, fmt.Errorf("redis get template tag %s/%s: %w", templateID, tag, err)
	}
	var rec TemplateTagRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return TemplateTagRecord{}, fmt.Errorf("unmarshal template tag %s/%s: %w", templateID, tag, err)
	}
	return rec, nil
}

func (r *Redis) ListTemplateTags(ctx context.Context, templateID string) ([]TemplateTagRecord, error) {
	if strings.TrimSpace(templateID) == "" {
		return nil, ErrCatalogTemplateIDEmpty
	}
	tags, err := r.client.SMembers(ctx, templateTagIndexKey(templateID)).Result()
	if err != nil {
		return nil, fmt.Errorf("redis smembers template tags %s: %w", templateID, err)
	}
	out := make([]TemplateTagRecord, 0, len(tags))
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		rec, err := r.GetTemplateTag(ctx, templateID, tag)
		if errors.Is(err, ErrTemplateTagNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

func (r *Redis) DeleteTemplateTags(ctx context.Context, templateID string, tags []string) error {
	if strings.TrimSpace(templateID) == "" {
		return ErrCatalogTemplateIDEmpty
	}
	if len(tags) == 0 {
		return ErrTemplateTagEmpty
	}
	pipe := r.client.TxPipeline()
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return ErrTemplateTagEmpty
		}
		pipe.Del(ctx, templateTagKey(templateID, tag))
		pipe.SRem(ctx, templateTagIndexKey(templateID), tag)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis delete template tags %s: %w", templateID, err)
	}
	return nil
}
