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

package platform

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/actordock/actordock/internal/config"
	"github.com/actordock/actordock/internal/store"
	v1alpha1 "github.com/actordock/runtime/pkg/api/v1alpha1"
	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
)

var ErrTemplateNotFound = errors.New("template not found")

// CatalogTemplate is a pre-provisioned ActorTemplate entry exposed as an E2B template.
type CatalogTemplate struct {
	TemplateID  string
	Namespace   string
	Name        string
	Aliases     []string
	Names       []string
	CPUCount    int
	MemoryMB    int
	DiskSizeMB  int
	EnvdVersion string
	BuildID     string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Public      bool
}

// TemplateCatalog lists cluster templates for Platform read routes and sandbox create.
type TemplateCatalog interface {
	List(ctx context.Context) ([]CatalogTemplate, error)
	Get(ctx context.Context, templateID string) (CatalogTemplate, error)
	ResolveAlias(ctx context.Context, alias string) (CatalogTemplate, error)
	Create(ctx context.Context, input CreateTemplateInput) (CatalogTemplate, error)
	Update(ctx context.Context, templateID string, public *bool) (CatalogTemplate, error)
}

// CreateTemplateInput is metadata accepted by POST /templates (no build).
type CreateTemplateInput struct {
	TemplateID string
	Alias      string
	Dockerfile string
	StartCmd   string
	ReadyCmd   string
	CPUCount   int
	MemoryMB   int
	Public     bool
}

type staticTemplateCatalog struct {
	templates []CatalogTemplate
	byID      map[string]CatalogTemplate
	byAlias   map[string]CatalogTemplate
}

func NewStaticTemplateCatalog(cfg config.Platform) TemplateCatalog {
	tmpl := catalogTemplateFromConfig(cfg, cfg.TemplateName, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	return newMemoryTemplateCatalog([]CatalogTemplate{tmpl})
}

func newMemoryTemplateCatalog(templates []CatalogTemplate) TemplateCatalog {
	byID := make(map[string]CatalogTemplate, len(templates))
	byAlias := make(map[string]CatalogTemplate, len(templates))
	for _, tmpl := range templates {
		byID[strings.ToLower(tmpl.TemplateID)] = tmpl
		byID[strings.ToLower(tmpl.Name)] = tmpl
		for _, name := range tmpl.Names {
			byID[strings.ToLower(name)] = tmpl
		}
		for _, alias := range tmpl.Aliases {
			byAlias[strings.ToLower(alias)] = tmpl
		}
	}
	return &staticTemplateCatalog{
		templates: append([]CatalogTemplate(nil), templates...),
		byID:      byID,
		byAlias:   byAlias,
	}
}

func (c *staticTemplateCatalog) List(_ context.Context) ([]CatalogTemplate, error) {
	return append([]CatalogTemplate(nil), c.templates...), nil
}

func (c *staticTemplateCatalog) Get(_ context.Context, templateID string) (CatalogTemplate, error) {
	key := strings.ToLower(strings.TrimSpace(templateID))
	if tmpl, ok := c.byID[key]; ok {
		return tmpl, nil
	}
	return CatalogTemplate{}, ErrTemplateNotFound
}

func (c *staticTemplateCatalog) ResolveAlias(_ context.Context, alias string) (CatalogTemplate, error) {
	key := strings.ToLower(strings.TrimSpace(alias))
	if tmpl, ok := c.byAlias[key]; ok {
		return tmpl, nil
	}
	if tmpl, ok := c.byID[key]; ok {
		return tmpl, nil
	}
	return CatalogTemplate{}, ErrTemplateNotFound
}

func (c *staticTemplateCatalog) Create(context.Context, CreateTemplateInput) (CatalogTemplate, error) {
	return CatalogTemplate{}, errTemplateCatalogReadOnly
}

func (c *staticTemplateCatalog) Update(context.Context, string, *bool) (CatalogTemplate, error) {
	return CatalogTemplate{}, errTemplateCatalogReadOnly
}

var errTemplateCatalogReadOnly = errors.New("template catalog is read-only")

type writableTemplateCatalog struct {
	base  TemplateCatalog
	store catalogTemplateStore
	cfg   config.Platform
}

// NewWritableTemplateCatalog overlays Redis-backed user templates on a base catalog.
func NewWritableTemplateCatalog(cfg config.Platform, base TemplateCatalog, st catalogTemplateStore) TemplateCatalog {
	if base == nil {
		base = NewStaticTemplateCatalog(cfg)
	}
	return &writableTemplateCatalog{base: base, store: st, cfg: cfg}
}

func (c *writableTemplateCatalog) List(ctx context.Context) ([]CatalogTemplate, error) {
	merged, err := c.base.List(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]CatalogTemplate, len(merged))
	for _, tmpl := range merged {
		byID[strings.ToLower(tmpl.TemplateID)] = tmpl
	}
	user, err := c.store.ListCatalogTemplates(ctx)
	if err != nil {
		return nil, err
	}
	for _, rec := range user {
		tmpl := catalogTemplateFromRecord(rec)
		byID[strings.ToLower(tmpl.TemplateID)] = tmpl
	}
	out := make([]CatalogTemplate, 0, len(byID))
	for _, tmpl := range byID {
		out = append(out, tmpl)
	}
	return out, nil
}

func (c *writableTemplateCatalog) Get(ctx context.Context, templateID string) (CatalogTemplate, error) {
	if tmpl, err := c.getUser(ctx, templateID); err == nil {
		return tmpl, nil
	} else if !errors.Is(err, store.ErrCatalogTemplateNotFound) {
		return CatalogTemplate{}, err
	}
	return c.base.Get(ctx, templateID)
}

func (c *writableTemplateCatalog) ResolveAlias(ctx context.Context, alias string) (CatalogTemplate, error) {
	key := strings.ToLower(strings.TrimSpace(alias))
	user, err := c.store.ListCatalogTemplates(ctx)
	if err != nil {
		return CatalogTemplate{}, err
	}
	for _, rec := range user {
		tmpl := catalogTemplateFromRecord(rec)
		if strings.EqualFold(tmpl.TemplateID, key) {
			return tmpl, nil
		}
		for _, item := range tmpl.Aliases {
			if strings.EqualFold(item, key) {
				return tmpl, nil
			}
		}
		for _, item := range tmpl.Names {
			if strings.EqualFold(item, key) {
				return tmpl, nil
			}
		}
	}
	return c.base.ResolveAlias(ctx, alias)
}

func (c *writableTemplateCatalog) Create(ctx context.Context, input CreateTemplateInput) (CatalogTemplate, error) {
	templateID := strings.TrimSpace(input.TemplateID)
	if templateID == "" {
		return CatalogTemplate{}, store.ErrCatalogTemplateIDEmpty
	}
	if strings.TrimSpace(input.Dockerfile) == "" {
		return CatalogTemplate{}, store.ErrCatalogTemplateDockerfile
	}
	if _, err := c.Get(ctx, templateID); err == nil {
		return CatalogTemplate{}, store.ErrCatalogTemplateExists
	} else if !errors.Is(err, ErrTemplateNotFound) && !errors.Is(err, store.ErrCatalogTemplateNotFound) {
		return CatalogTemplate{}, err
	}

	cpuCount := input.CPUCount
	if cpuCount == 0 {
		cpuCount = defaultCPUCount
	}
	memoryMB := input.MemoryMB
	if memoryMB == 0 {
		memoryMB = defaultMemoryMB
	}
	if cpuCount < 1 {
		return CatalogTemplate{}, fmt.Errorf("cpuCount must be at least 1")
	}
	if memoryMB < 128 {
		return CatalogTemplate{}, fmt.Errorf("memoryMB must be at least 128")
	}

	alias := strings.TrimSpace(input.Alias)
	if alias == "" {
		alias = templateID
	}
	now := time.Now().UTC()
	ns := c.cfg.TemplateNamespace
	rec := store.CatalogTemplateRecord{
		TemplateID:  templateID,
		Namespace:   ns,
		Name:        c.cfg.TemplateName,
		Aliases:     []string{alias},
		Names:       []string{alias},
		CPUCount:    cpuCount,
		MemoryMB:    memoryMB,
		DiskSizeMB:  defaultDiskSizeMB,
		EnvdVersion: c.cfg.EnvdVersion,
		BuildID:     stableTemplateBuildID(ns, templateID),
		CreatedAt:   now,
		UpdatedAt:   now,
		Public:      true,
		Dockerfile:  input.Dockerfile,
		StartCmd:    input.StartCmd,
		ReadyCmd:    input.ReadyCmd,
	}
	if err := c.store.PutCatalogTemplate(ctx, rec); err != nil {
		return CatalogTemplate{}, err
	}
	return catalogTemplateFromRecord(rec), nil
}

func (c *writableTemplateCatalog) Update(ctx context.Context, templateID string, public *bool) (CatalogTemplate, error) {
	if public == nil {
		return c.Get(ctx, templateID)
	}
	rec, err := c.store.GetCatalogTemplate(ctx, templateID)
	if errors.Is(err, store.ErrCatalogTemplateNotFound) {
		base, getErr := c.base.Get(ctx, templateID)
		if getErr != nil {
			return CatalogTemplate{}, ErrTemplateNotFound
		}
		now := time.Now().UTC()
		rec = catalogTemplateToRecord(base)
		rec.Dockerfile = "FROM scratch"
		rec.UpdatedAt = now
		if err := c.store.PutCatalogTemplate(ctx, rec); err != nil {
			return CatalogTemplate{}, err
		}
	} else if err != nil {
		return CatalogTemplate{}, err
	}
	rec.Public = *public
	rec.UpdatedAt = time.Now().UTC()
	if err := c.store.UpdateCatalogTemplate(ctx, rec); err != nil {
		return CatalogTemplate{}, err
	}
	return catalogTemplateFromRecord(rec), nil
}

func (c *writableTemplateCatalog) getUser(ctx context.Context, templateID string) (CatalogTemplate, error) {
	rec, err := c.store.GetCatalogTemplate(ctx, templateID)
	if err != nil {
		return CatalogTemplate{}, err
	}
	return catalogTemplateFromRecord(rec), nil
}

func catalogTemplateFromRecord(rec store.CatalogTemplateRecord) CatalogTemplate {
	return CatalogTemplate{
		TemplateID:  rec.TemplateID,
		Namespace:   rec.Namespace,
		Name:        rec.Name,
		Aliases:     append([]string(nil), rec.Aliases...),
		Names:       append([]string(nil), rec.Names...),
		CPUCount:    rec.CPUCount,
		MemoryMB:    rec.MemoryMB,
		DiskSizeMB:  rec.DiskSizeMB,
		EnvdVersion: rec.EnvdVersion,
		BuildID:     rec.BuildID,
		CreatedAt:   rec.CreatedAt,
		UpdatedAt:   rec.UpdatedAt,
		Public:      rec.Public,
	}
}

func catalogTemplateToRecord(tmpl CatalogTemplate) store.CatalogTemplateRecord {
	return store.CatalogTemplateRecord{
		TemplateID:  tmpl.TemplateID,
		Namespace:   tmpl.Namespace,
		Name:        tmpl.Name,
		Aliases:     append([]string(nil), tmpl.Aliases...),
		Names:       append([]string(nil), tmpl.Names...),
		CPUCount:    tmpl.CPUCount,
		MemoryMB:    tmpl.MemoryMB,
		DiskSizeMB:  tmpl.DiskSizeMB,
		EnvdVersion: tmpl.EnvdVersion,
		BuildID:     tmpl.BuildID,
		CreatedAt:   tmpl.CreatedAt,
		UpdatedAt:   tmpl.UpdatedAt,
		Public:      tmpl.Public,
	}
}

type k8sTemplateCatalog struct {
	client    client.Client
	namespace string
	cfg       config.Platform
}

func NewTemplateCatalogFromCluster(cfg config.Platform) (TemplateCatalog, error) {
	restConfig, err := ctrlconfig.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("kubernetes config: %w", err)
	}
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("add scheme: %w", err)
	}
	c, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("kubernetes client: %w", err)
	}
	return &k8sTemplateCatalog{
		client:    c,
		namespace: cfg.TemplateNamespace,
		cfg:       cfg,
	}, nil
}

func (c *k8sTemplateCatalog) List(ctx context.Context) ([]CatalogTemplate, error) {
	var list v1alpha1.ActorTemplateList
	if err := c.client.List(ctx, &list, client.InNamespace(c.namespace)); err != nil {
		return nil, fmt.Errorf("list actortemplates: %w", err)
	}
	out := make([]CatalogTemplate, 0, len(list.Items))
	for i := range list.Items {
		item := &list.Items[i]
		if item.Status.Phase != v1alpha1.PhaseReady {
			continue
		}
		out = append(out, mapActorTemplate(c.cfg, item))
	}
	return out, nil
}

func (c *k8sTemplateCatalog) Get(ctx context.Context, templateID string) (CatalogTemplate, error) {
	templates, err := c.List(ctx)
	if err != nil {
		return CatalogTemplate{}, err
	}
	key := strings.ToLower(strings.TrimSpace(templateID))
	for _, tmpl := range templates {
		if strings.EqualFold(tmpl.TemplateID, key) || strings.EqualFold(tmpl.Name, key) {
			return tmpl, nil
		}
		for _, name := range tmpl.Names {
			if strings.EqualFold(name, key) {
				return tmpl, nil
			}
		}
	}
	return CatalogTemplate{}, ErrTemplateNotFound
}

func (c *k8sTemplateCatalog) ResolveAlias(ctx context.Context, alias string) (CatalogTemplate, error) {
	templates, err := c.List(ctx)
	if err != nil {
		return CatalogTemplate{}, err
	}
	key := strings.ToLower(strings.TrimSpace(alias))
	for _, tmpl := range templates {
		for _, item := range tmpl.Aliases {
			if strings.EqualFold(item, key) {
				return tmpl, nil
			}
		}
		if strings.EqualFold(tmpl.TemplateID, key) || strings.EqualFold(tmpl.Name, key) {
			return tmpl, nil
		}
	}
	return CatalogTemplate{}, ErrTemplateNotFound
}

func (c *k8sTemplateCatalog) Create(context.Context, CreateTemplateInput) (CatalogTemplate, error) {
	return CatalogTemplate{}, errTemplateCatalogReadOnly
}

func (c *k8sTemplateCatalog) Update(context.Context, string, *bool) (CatalogTemplate, error) {
	return CatalogTemplate{}, errTemplateCatalogReadOnly
}

func catalogTemplateFromConfig(cfg config.Platform, name string, createdAt time.Time) CatalogTemplate {
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	ns := cfg.TemplateNamespace
	return CatalogTemplate{
		TemplateID:  name,
		Namespace:   ns,
		Name:        name,
		Aliases:     []string{name},
		Names:       []string{name},
		CPUCount:    defaultCPUCount,
		MemoryMB:    defaultMemoryMB,
		DiskSizeMB:  defaultDiskSizeMB,
		EnvdVersion: cfg.EnvdVersion,
		BuildID:     stableTemplateBuildID(ns, name),
		CreatedAt:   createdAt.UTC(),
		UpdatedAt:   createdAt.UTC(),
		Public:      true,
	}
}

func mapActorTemplate(cfg config.Platform, at *v1alpha1.ActorTemplate) CatalogTemplate {
	createdAt := at.CreationTimestamp.Time.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	updatedAt := createdAt
	for _, cond := range at.Status.Conditions {
		if !cond.LastTransitionTime.IsZero() {
			updatedAt = cond.LastTransitionTime.Time.UTC()
			break
		}
	}
	name := at.Name
	ns := at.Namespace
	if ns == "" {
		ns = cfg.TemplateNamespace
	}
	return CatalogTemplate{
		TemplateID:  name,
		Namespace:   ns,
		Name:        name,
		Aliases:     []string{name},
		Names:       []string{name},
		CPUCount:    defaultCPUCount,
		MemoryMB:    defaultMemoryMB,
		DiskSizeMB:  defaultDiskSizeMB,
		EnvdVersion: cfg.EnvdVersion,
		BuildID:     stableTemplateBuildID(ns, name),
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		Public:      true,
	}
}

func stableTemplateBuildID(namespace, name string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(namespace+"/"+name)).String()
}

func formatRFC3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
