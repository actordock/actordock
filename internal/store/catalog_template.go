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
	ErrCatalogTemplateNotFound   = errors.New("catalog template not found")
	ErrCatalogTemplateExists     = errors.New("catalog template already exists")
	ErrCatalogTemplateIDEmpty    = errors.New("catalog template id is required")
	ErrCatalogTemplateDockerfile = errors.New("dockerfile is required")
)

const catalogTemplateKeyPrefix = "actordock:catalog-template:"

// CatalogTemplateRecord is persisted user-defined template metadata (no build pipeline).
type CatalogTemplateRecord struct {
	TemplateID  string    `json:"template_id"`
	Namespace   string    `json:"namespace"`
	Name        string    `json:"name"`
	Aliases     []string  `json:"aliases,omitempty"`
	Names       []string  `json:"names,omitempty"`
	CPUCount    int       `json:"cpu_count"`
	MemoryMB    int       `json:"memory_mb"`
	DiskSizeMB  int       `json:"disk_size_mb"`
	EnvdVersion string    `json:"envd_version"`
	BuildID     string    `json:"build_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Public      bool      `json:"public"`
	Dockerfile  string    `json:"dockerfile,omitempty"`
	StartCmd    string    `json:"start_cmd,omitempty"`
	ReadyCmd    string    `json:"ready_cmd,omitempty"`
}

func catalogTemplateKey(id string) string {
	return catalogTemplateKeyPrefix + id
}

func validateCatalogTemplate(rec CatalogTemplateRecord) error {
	if strings.TrimSpace(rec.TemplateID) == "" {
		return ErrCatalogTemplateIDEmpty
	}
	if strings.TrimSpace(rec.Dockerfile) == "" {
		return ErrCatalogTemplateDockerfile
	}
	if rec.Name == "" {
		return fmt.Errorf("catalog template actor name is required")
	}
	if rec.Namespace == "" {
		return fmt.Errorf("catalog template namespace is required")
	}
	if rec.CPUCount < 1 {
		return fmt.Errorf("cpuCount must be at least 1")
	}
	if rec.MemoryMB < 128 {
		return fmt.Errorf("memoryMB must be at least 128")
	}
	if rec.BuildID == "" {
		return fmt.Errorf("catalog template build id is required")
	}
	if rec.EnvdVersion == "" {
		return fmt.Errorf("catalog template envd version is required")
	}
	if rec.CreatedAt.IsZero() || rec.UpdatedAt.IsZero() {
		return fmt.Errorf("catalog template timestamps are required")
	}
	return nil
}

func (r *Redis) PutCatalogTemplate(ctx context.Context, rec CatalogTemplateRecord) error {
	if err := validateCatalogTemplate(rec); err != nil {
		return err
	}
	key := catalogTemplateKey(rec.TemplateID)
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("redis exists catalog template %s: %w", rec.TemplateID, err)
	}
	if exists > 0 {
		return ErrCatalogTemplateExists
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal catalog template: %w", err)
	}
	if err := r.client.Set(ctx, key, data, 0).Err(); err != nil {
		return fmt.Errorf("redis put catalog template %s: %w", rec.TemplateID, err)
	}
	return nil
}

func (r *Redis) GetCatalogTemplate(ctx context.Context, templateID string) (CatalogTemplateRecord, error) {
	if strings.TrimSpace(templateID) == "" {
		return CatalogTemplateRecord{}, ErrCatalogTemplateIDEmpty
	}
	data, err := r.client.Get(ctx, catalogTemplateKey(templateID)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return CatalogTemplateRecord{}, ErrCatalogTemplateNotFound
	}
	if err != nil {
		return CatalogTemplateRecord{}, fmt.Errorf("redis get catalog template %s: %w", templateID, err)
	}
	var rec CatalogTemplateRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return CatalogTemplateRecord{}, fmt.Errorf("unmarshal catalog template %s: %w", templateID, err)
	}
	return rec, nil
}

func (r *Redis) ListCatalogTemplates(ctx context.Context) ([]CatalogTemplateRecord, error) {
	var out []CatalogTemplateRecord
	iter := r.client.Scan(ctx, 0, catalogTemplateKeyPrefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		id := strings.TrimPrefix(iter.Val(), catalogTemplateKeyPrefix)
		if id == "" {
			continue
		}
		rec, err := r.GetCatalogTemplate(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("redis scan catalog templates: %w", err)
	}
	return out, nil
}

func (r *Redis) UpdateCatalogTemplate(ctx context.Context, rec CatalogTemplateRecord) error {
	if strings.TrimSpace(rec.TemplateID) == "" {
		return ErrCatalogTemplateIDEmpty
	}
	if rec.UpdatedAt.IsZero() {
		return fmt.Errorf("catalog template updated_at is required")
	}
	key := catalogTemplateKey(rec.TemplateID)
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("redis exists catalog template %s: %w", rec.TemplateID, err)
	}
	if exists == 0 {
		return ErrCatalogTemplateNotFound
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal catalog template: %w", err)
	}
	if err := r.client.Set(ctx, key, data, 0).Err(); err != nil {
		return fmt.Errorf("redis update catalog template %s: %w", rec.TemplateID, err)
	}
	return nil
}
