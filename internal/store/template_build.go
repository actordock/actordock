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
	ErrTemplateBuildNotFound      = errors.New("template build not found")
	ErrTemplateBuildIDEmpty       = errors.New("template build id is required")
	ErrTemplateBuildFileNotFound  = errors.New("template build file not found")
	ErrTemplateBuildFileHashEmpty = errors.New("template build file hash is required")
)

// TemplateBuildStatus is the E2B-compatible template build lifecycle state.
type TemplateBuildStatus string

const (
	TemplateBuildStatusWaiting  TemplateBuildStatus = "waiting"
	TemplateBuildStatusBuilding TemplateBuildStatus = "building"
	TemplateBuildStatusReady    TemplateBuildStatus = "ready"
	TemplateBuildStatusError    TemplateBuildStatus = "error"
)

const (
	templateBuildKeyPrefix       = "actordock:template-build:"
	templateBuildLatestKeyPrefix = "actordock:template-build-latest:"
	templateBuildTemplatesKey    = "actordock:template-build-templates"
	templateBuildLogKeyPrefix    = "actordock:template-build-log:"
	templateBuildFileKeyPrefix   = "actordock:template-build-file:"
)

// TemplateBuild is persisted build metadata for E2B v3 template builds.
type TemplateBuild struct {
	TemplateID   string              `json:"template_id"`
	BuildID      string              `json:"build_id"`
	Status       TemplateBuildStatus `json:"status"`
	StepsJSON    json.RawMessage     `json:"steps_json,omitempty"`
	CPUCount     int                 `json:"cpu_count"`
	MemoryMB     int                 `json:"memory_mb"`
	Tags         []string            `json:"tags,omitempty"`
	StartCmd     string              `json:"start_cmd,omitempty"`
	ReadyCmd     string              `json:"ready_cmd,omitempty"`
	FromTemplate string              `json:"from_template,omitempty"`
	FromImage    string              `json:"from_image,omitempty"`
	Namespace    string              `json:"namespace"`
	ActorName    string              `json:"actor_name"`
	Public       bool                `json:"public"`
	EnvdVersion  string              `json:"envd_version"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
	FinishedAt   *time.Time          `json:"finished_at,omitempty"`
	ErrorMessage string              `json:"error_message,omitempty"`
	PinnedImage  string              `json:"pinned_image,omitempty"`
}

// BuildLogEntry is an append-only build log line.
type BuildLogEntry struct {
	TemplateID string    `json:"template_id"`
	BuildID    string    `json:"build_id"`
	Timestamp  time.Time `json:"timestamp"`
	Level      string    `json:"level"`
	Message    string    `json:"message"`
	Step       string    `json:"step,omitempty"`
}

// TemplateBuildFile indexes uploaded build layer tar blobs by content hash.
type TemplateBuildFile struct {
	FilesHash string    `json:"files_hash"`
	ObjectKey string    `json:"object_key"`
	Present   bool      `json:"present"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func templateBuildKey(templateID, buildID string) string {
	return templateBuildKeyPrefix + templateID + ":" + buildID
}

func templateBuildLatestKey(templateID string) string {
	return templateBuildLatestKeyPrefix + templateID
}

func templateBuildLogKey(templateID, buildID string) string {
	return templateBuildLogKeyPrefix + templateID + ":" + buildID
}

func templateBuildFileKey(filesHash string) string {
	return templateBuildFileKeyPrefix + filesHash
}

func validateTemplateBuild(b TemplateBuild) error {
	if strings.TrimSpace(b.TemplateID) == "" {
		return ErrCatalogTemplateIDEmpty
	}
	if strings.TrimSpace(b.BuildID) == "" {
		return ErrTemplateBuildIDEmpty
	}
	switch b.Status {
	case TemplateBuildStatusWaiting, TemplateBuildStatusBuilding, TemplateBuildStatusReady, TemplateBuildStatusError:
	default:
		return fmt.Errorf("invalid template build status %q", b.Status)
	}
	if b.CPUCount < 1 {
		return fmt.Errorf("cpuCount must be at least 1")
	}
	if b.MemoryMB < 128 {
		return fmt.Errorf("memoryMB must be at least 128")
	}
	if b.Namespace == "" {
		return fmt.Errorf("template build namespace is required")
	}
	if b.ActorName == "" {
		return fmt.Errorf("template build actor name is required")
	}
	if b.EnvdVersion == "" {
		return fmt.Errorf("template build envd version is required")
	}
	if b.CreatedAt.IsZero() || b.UpdatedAt.IsZero() {
		return fmt.Errorf("template build timestamps are required")
	}
	return nil
}

func (r *Redis) PutTemplateBuild(ctx context.Context, build TemplateBuild) error {
	if err := validateTemplateBuild(build); err != nil {
		return err
	}
	data, err := json.Marshal(build)
	if err != nil {
		return fmt.Errorf("marshal template build: %w", err)
	}
	key := templateBuildKey(build.TemplateID, build.BuildID)
	pipe := r.client.TxPipeline()
	pipe.Set(ctx, key, data, 0)
	pipe.SAdd(ctx, templateBuildTemplatesKey, build.TemplateID)
	pipe.Set(ctx, templateBuildLatestKey(build.TemplateID), build.BuildID, 0)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis put template build %s/%s: %w", build.TemplateID, build.BuildID, err)
	}
	return nil
}

func (r *Redis) GetTemplateBuild(ctx context.Context, templateID, buildID string) (TemplateBuild, error) {
	if strings.TrimSpace(templateID) == "" {
		return TemplateBuild{}, ErrCatalogTemplateIDEmpty
	}
	if strings.TrimSpace(buildID) == "" {
		return TemplateBuild{}, ErrTemplateBuildIDEmpty
	}
	data, err := r.client.Get(ctx, templateBuildKey(templateID, buildID)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return TemplateBuild{}, ErrTemplateBuildNotFound
	}
	if err != nil {
		return TemplateBuild{}, fmt.Errorf("redis get template build %s/%s: %w", templateID, buildID, err)
	}
	var build TemplateBuild
	if err := json.Unmarshal(data, &build); err != nil {
		return TemplateBuild{}, fmt.Errorf("unmarshal template build %s/%s: %w", templateID, buildID, err)
	}
	return build, nil
}

func (r *Redis) UpdateTemplateBuild(ctx context.Context, build TemplateBuild) error {
	if strings.TrimSpace(build.TemplateID) == "" {
		return ErrCatalogTemplateIDEmpty
	}
	if strings.TrimSpace(build.BuildID) == "" {
		return ErrTemplateBuildIDEmpty
	}
	exists, err := r.client.Exists(ctx, templateBuildKey(build.TemplateID, build.BuildID)).Result()
	if err != nil {
		return fmt.Errorf("redis exists template build %s/%s: %w", build.TemplateID, build.BuildID, err)
	}
	if exists == 0 {
		return ErrTemplateBuildNotFound
	}
	if build.UpdatedAt.IsZero() {
		return fmt.Errorf("template build updated_at is required")
	}
	if err := validateTemplateBuild(build); err != nil {
		return err
	}
	data, err := json.Marshal(build)
	if err != nil {
		return fmt.Errorf("marshal template build: %w", err)
	}
	if err := r.client.Set(ctx, templateBuildKey(build.TemplateID, build.BuildID), data, 0).Err(); err != nil {
		return fmt.Errorf("redis update template build %s/%s: %w", build.TemplateID, build.BuildID, err)
	}
	return nil
}

func (r *Redis) ListTemplateBuilds(ctx context.Context, templateID string) ([]TemplateBuild, error) {
	if strings.TrimSpace(templateID) == "" {
		return nil, ErrCatalogTemplateIDEmpty
	}
	pattern := templateBuildKeyPrefix + templateID + ":*"
	var out []TemplateBuild
	iter := r.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		parts := strings.SplitN(strings.TrimPrefix(key, templateBuildKeyPrefix), ":", 2)
		if len(parts) != 2 {
			continue
		}
		build, err := r.GetTemplateBuild(ctx, parts[0], parts[1])
		if err != nil {
			return nil, err
		}
		out = append(out, build)
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("redis scan template builds: %w", err)
	}
	return out, nil
}

func (r *Redis) GetLatestTemplateBuild(ctx context.Context, templateID string) (TemplateBuild, error) {
	if strings.TrimSpace(templateID) == "" {
		return TemplateBuild{}, ErrCatalogTemplateIDEmpty
	}
	buildID, err := r.client.Get(ctx, templateBuildLatestKey(templateID)).Result()
	if errors.Is(err, goredis.Nil) {
		return TemplateBuild{}, ErrTemplateBuildNotFound
	}
	if err != nil {
		return TemplateBuild{}, fmt.Errorf("redis get latest template build %s: %w", templateID, err)
	}
	return r.GetTemplateBuild(ctx, templateID, buildID)
}

func (r *Redis) ListLatestTemplateBuilds(ctx context.Context) ([]TemplateBuild, error) {
	templateIDs, err := r.client.SMembers(ctx, templateBuildTemplatesKey).Result()
	if err != nil {
		return nil, fmt.Errorf("redis smembers template build templates: %w", err)
	}
	out := make([]TemplateBuild, 0, len(templateIDs))
	for _, templateID := range templateIDs {
		if templateID == "" {
			continue
		}
		build, err := r.GetLatestTemplateBuild(ctx, templateID)
		if errors.Is(err, ErrTemplateBuildNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, build)
	}
	return out, nil
}

func (r *Redis) AppendBuildLog(ctx context.Context, entry BuildLogEntry) error {
	if strings.TrimSpace(entry.TemplateID) == "" {
		return ErrCatalogTemplateIDEmpty
	}
	if strings.TrimSpace(entry.BuildID) == "" {
		return ErrTemplateBuildIDEmpty
	}
	if entry.Timestamp.IsZero() {
		return fmt.Errorf("build log timestamp is required")
	}
	if strings.TrimSpace(entry.Level) == "" {
		return fmt.Errorf("build log level is required")
	}
	if strings.TrimSpace(entry.Message) == "" {
		return fmt.Errorf("build log message is required")
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal build log: %w", err)
	}
	key := templateBuildLogKey(entry.TemplateID, entry.BuildID)
	if err := r.client.RPush(ctx, key, data).Err(); err != nil {
		return fmt.Errorf("redis append build log %s/%s: %w", entry.TemplateID, entry.BuildID, err)
	}
	return nil
}

func (r *Redis) ListBuildLogs(ctx context.Context, templateID, buildID string, offset, limit int) ([]BuildLogEntry, error) {
	if strings.TrimSpace(templateID) == "" {
		return nil, ErrCatalogTemplateIDEmpty
	}
	if strings.TrimSpace(buildID) == "" {
		return nil, ErrTemplateBuildIDEmpty
	}
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}
	key := templateBuildLogKey(templateID, buildID)
	stop := int64(offset + limit - 1)
	if limit == 0 {
		stop = -1
	}
	raw, err := r.client.LRange(ctx, key, int64(offset), stop).Result()
	if err != nil {
		return nil, fmt.Errorf("redis lrange build logs %s/%s: %w", templateID, buildID, err)
	}
	out := make([]BuildLogEntry, 0, len(raw))
	for _, item := range raw {
		var entry BuildLogEntry
		if err := json.Unmarshal([]byte(item), &entry); err != nil {
			return nil, fmt.Errorf("unmarshal build log: %w", err)
		}
		out = append(out, entry)
	}
	return out, nil
}

func (r *Redis) PutTemplateBuildFile(ctx context.Context, file TemplateBuildFile) error {
	if strings.TrimSpace(file.FilesHash) == "" {
		return ErrTemplateBuildFileHashEmpty
	}
	if strings.TrimSpace(file.ObjectKey) == "" {
		return fmt.Errorf("template build file object key is required")
	}
	if file.CreatedAt.IsZero() || file.UpdatedAt.IsZero() {
		return fmt.Errorf("template build file timestamps are required")
	}
	data, err := json.Marshal(file)
	if err != nil {
		return fmt.Errorf("marshal template build file: %w", err)
	}
	if err := r.client.Set(ctx, templateBuildFileKey(file.FilesHash), data, 0).Err(); err != nil {
		return fmt.Errorf("redis put template build file %s: %w", file.FilesHash, err)
	}
	return nil
}

func (r *Redis) GetTemplateBuildFile(ctx context.Context, filesHash string) (TemplateBuildFile, error) {
	if strings.TrimSpace(filesHash) == "" {
		return TemplateBuildFile{}, ErrTemplateBuildFileHashEmpty
	}
	data, err := r.client.Get(ctx, templateBuildFileKey(filesHash)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return TemplateBuildFile{}, ErrTemplateBuildFileNotFound
	}
	if err != nil {
		return TemplateBuildFile{}, fmt.Errorf("redis get template build file %s: %w", filesHash, err)
	}
	var file TemplateBuildFile
	if err := json.Unmarshal(data, &file); err != nil {
		return TemplateBuildFile{}, fmt.Errorf("unmarshal template build file %s: %w", filesHash, err)
	}
	return file, nil
}

func (r *Redis) MarkTemplateBuildFilePresent(ctx context.Context, filesHash string, present bool) error {
	file, err := r.GetTemplateBuildFile(ctx, filesHash)
	if err != nil {
		return err
	}
	file.Present = present
	file.UpdatedAt = time.Now().UTC()
	return r.PutTemplateBuildFile(ctx, file)
}
