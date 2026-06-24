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
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func sampleTemplateBuild(templateID, buildID string, status TemplateBuildStatus) TemplateBuild {
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	return TemplateBuild{
		TemplateID:   templateID,
		BuildID:      buildID,
		Status:       status,
		StepsJSON:    json.RawMessage(`[{"type":"RUN","args":["echo hi"]}]`),
		CPUCount:     2,
		MemoryMB:     512,
		Tags:         []string{"latest"},
		Namespace:    "actordock",
		ActorName:    templateID,
		Public:       true,
		EnvdVersion:  "0.1.0",
		CreatedAt:    now,
		UpdatedAt:    now,
		FromTemplate: "python",
	}
}

func TestTemplateBuildRoundTrip(t *testing.T) {
	t.Parallel()
	s := newTestRedis(t)
	ctx := context.Background()

	build := sampleTemplateBuild("my-tpl", "build-1", TemplateBuildStatusBuilding)
	if err := s.PutTemplateBuild(ctx, build); err != nil {
		t.Fatalf("PutTemplateBuild: %v", err)
	}

	got, err := s.GetTemplateBuild(ctx, "my-tpl", "build-1")
	if err != nil {
		t.Fatalf("GetTemplateBuild: %v", err)
	}
	if got.ActorName != "my-tpl" || got.Status != TemplateBuildStatusBuilding {
		t.Fatalf("got = %+v", got)
	}

	latest, err := s.GetLatestTemplateBuild(ctx, "my-tpl")
	if err != nil {
		t.Fatalf("GetLatestTemplateBuild: %v", err)
	}
	if latest.BuildID != "build-1" {
		t.Fatalf("latest = %+v", latest)
	}

	build.Status = TemplateBuildStatusReady
	finished := time.Date(2026, 6, 24, 10, 5, 0, 0, time.UTC)
	build.FinishedAt = &finished
	build.UpdatedAt = finished
	if err := s.UpdateTemplateBuild(ctx, build); err != nil {
		t.Fatalf("UpdateTemplateBuild: %v", err)
	}

	list, err := s.ListLatestTemplateBuilds(ctx)
	if err != nil {
		t.Fatalf("ListLatestTemplateBuilds: %v", err)
	}
	if len(list) != 1 || list[0].Status != TemplateBuildStatusReady {
		t.Fatalf("list = %+v", list)
	}
}

func TestBuildLogsAppendAndList(t *testing.T) {
	t.Parallel()
	s := newTestRedis(t)
	ctx := context.Background()

	ts := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	entries := []BuildLogEntry{
		{TemplateID: "tpl", BuildID: "b1", Timestamp: ts, Level: "info", Message: "start"},
		{TemplateID: "tpl", BuildID: "b1", Timestamp: ts.Add(time.Second), Level: "info", Message: "done", Step: "build"},
	}
	for _, entry := range entries {
		if err := s.AppendBuildLog(ctx, entry); err != nil {
			t.Fatalf("AppendBuildLog: %v", err)
		}
	}

	got, err := s.ListBuildLogs(ctx, "tpl", "b1", 0, 10)
	if err != nil {
		t.Fatalf("ListBuildLogs: %v", err)
	}
	if len(got) != 2 || got[1].Step != "build" {
		t.Fatalf("logs = %+v", got)
	}

	page, err := s.ListBuildLogs(ctx, "tpl", "b1", 1, 1)
	if err != nil {
		t.Fatalf("ListBuildLogs page: %v", err)
	}
	if len(page) != 1 || page[0].Message != "done" {
		t.Fatalf("page = %+v", page)
	}
}

func TestTemplateBuildFilePresent(t *testing.T) {
	t.Parallel()
	s := newTestRedis(t)
	ctx := context.Background()

	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	file := TemplateBuildFile{
		FilesHash: "abc123",
		ObjectKey: "build-files/abc123.tar",
		Present:   false,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.PutTemplateBuildFile(ctx, file); err != nil {
		t.Fatalf("PutTemplateBuildFile: %v", err)
	}
	if err := s.MarkTemplateBuildFilePresent(ctx, "abc123", true); err != nil {
		t.Fatalf("MarkTemplateBuildFilePresent: %v", err)
	}
	got, err := s.GetTemplateBuildFile(ctx, "abc123")
	if err != nil {
		t.Fatalf("GetTemplateBuildFile: %v", err)
	}
	if !got.Present {
		t.Fatal("expected present=true")
	}
}

func TestTemplateBuildNotFound(t *testing.T) {
	t.Parallel()
	s := newTestRedis(t)
	ctx := context.Background()

	_, err := s.GetTemplateBuild(ctx, "missing", "build")
	if !errors.Is(err, ErrTemplateBuildNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func newTestRedis(t *testing.T) *Redis {
	t.Helper()
	mr := miniredis.RunT(t)
	s, err := NewRedis(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	return s
}
