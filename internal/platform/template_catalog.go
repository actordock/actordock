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
	v1alpha1 "github.com/agent-substrate/substrate/pkg/api/v1alpha1"
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
