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
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func sampleCatalogTemplate(id string) CatalogTemplateRecord {
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	return CatalogTemplateRecord{
		TemplateID:  id,
		Namespace:   "actordock",
		Name:        id,
		Aliases:     []string{id},
		Names:       []string{id},
		CPUCount:    2,
		MemoryMB:    512,
		DiskSizeMB:  512,
		EnvdVersion: "0.1.0",
		BuildID:     "build-" + id,
		CreatedAt:   now,
		UpdatedAt:   now,
		Public:      true,
		Dockerfile:  "FROM ubuntu",
	}
}

func TestCatalogTemplateRoundTrip(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	s, err := NewRedis(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	ctx := context.Background()
	rec := sampleCatalogTemplate("my-tpl")
	if err := s.PutCatalogTemplate(ctx, rec); err != nil {
		t.Fatalf("PutCatalogTemplate: %v", err)
	}
	got, err := s.GetCatalogTemplate(ctx, "my-tpl")
	if err != nil {
		t.Fatalf("GetCatalogTemplate: %v", err)
	}
	if got.Dockerfile != "FROM ubuntu" || got.Name != "my-tpl" {
		t.Fatalf("got = %+v", got)
	}
	list, err := s.ListCatalogTemplates(ctx)
	if err != nil {
		t.Fatalf("ListCatalogTemplates: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len = %d", len(list))
	}
	rec.Public = false
	rec.UpdatedAt = rec.UpdatedAt.Add(time.Hour)
	if err := s.UpdateCatalogTemplate(ctx, rec); err != nil {
		t.Fatalf("UpdateCatalogTemplate: %v", err)
	}
	got, err = s.GetCatalogTemplate(ctx, "my-tpl")
	if err != nil {
		t.Fatalf("GetCatalogTemplate after update: %v", err)
	}
	if got.Public {
		t.Fatal("public not updated")
	}
}

func TestCatalogTemplateDuplicateRejected(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	s, err := NewRedis(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	ctx := context.Background()
	rec := sampleCatalogTemplate("dup")
	if err := s.PutCatalogTemplate(ctx, rec); err != nil {
		t.Fatalf("PutCatalogTemplate: %v", err)
	}
	if err := s.PutCatalogTemplate(ctx, rec); !errors.Is(err, ErrCatalogTemplateExists) {
		t.Fatalf("duplicate = %v", err)
	}
}
