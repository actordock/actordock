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
	"context"
	"fmt"
	"strings"
	"time"

	v1alpha1 "github.com/actordock/runtime/pkg/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ActorTemplateManager creates user ActorTemplates and waits for golden snapshot readiness.
type ActorTemplateManager struct {
	Client    client.Client
	Namespace string
}

type envdContainer struct {
	Image   string
	Command []string
	Env     []v1alpha1.EnvVar
}

func (m *ActorTemplateManager) Replace(ctx context.Context, name string, builtImage string, base *v1alpha1.ActorTemplate, envd envdContainer) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("actor template name is required")
	}
	builtImage = strings.TrimSpace(builtImage)
	if builtImage == "" || !strings.Contains(builtImage, "@") {
		return fmt.Errorf("built image must be digest-pinned")
	}
	if base == nil {
		return fmt.Errorf("base actor template is required")
	}

	key := types.NamespacedName{Namespace: m.Namespace, Name: name}
	var existing v1alpha1.ActorTemplate
	if err := m.Client.Get(ctx, key, &existing); err == nil {
		if err := m.Client.Delete(ctx, &existing); err != nil {
			return fmt.Errorf("delete existing actortemplate %s: %w", name, err)
		}
		if err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
			err := m.Client.Get(ctx, key, &existing)
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}); err != nil {
			return fmt.Errorf("wait for actortemplate %s deletion: %w", name, err)
		}
	} else if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get actortemplate %s: %w", name, err)
	}

	cmd := envd.Command
	if len(cmd) == 0 {
		cmd = []string{"/ko-app/envd"}
	}
	at := &v1alpha1.ActorTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: m.Namespace,
		},
		Spec: v1alpha1.ActorTemplateSpec{
			PauseImage: base.Spec.PauseImage,
			Containers: []v1alpha1.Container{{
				Name:    "envd",
				Image:   builtImage,
				Command: append([]string(nil), cmd...),
				Env:     append([]v1alpha1.EnvVar(nil), envd.Env...),
			}},
			SnapshotsConfig: base.Spec.SnapshotsConfig,
			WorkerSelector:  base.Spec.WorkerSelector,
		},
	}
	if err := m.Client.Create(ctx, at); err != nil {
		return fmt.Errorf("create actortemplate %s: %w", name, err)
	}
	return nil
}

func (m *ActorTemplateManager) WaitReady(ctx context.Context, name string) error {
	key := types.NamespacedName{Namespace: m.Namespace, Name: name}
	return wait.PollUntilContextTimeout(ctx, 5*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		var at v1alpha1.ActorTemplate
		if err := m.Client.Get(ctx, key, &at); err != nil {
			return false, err
		}
		if at.Status.Phase == v1alpha1.PhaseFailed {
			return false, fmt.Errorf("actortemplate %s failed", name)
		}
		for _, cond := range at.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == metav1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
}

func envdFromActorTemplate(at *v1alpha1.ActorTemplate) (envdContainer, error) {
	for _, c := range at.Spec.Containers {
		if c.Name == "envd" {
			return envdContainer{
				Image:   c.Image,
				Command: append([]string(nil), c.Command...),
				Env:     append([]v1alpha1.EnvVar(nil), c.Env...),
			}, nil
		}
	}
	return envdContainer{}, fmt.Errorf("actortemplate %s has no envd container", at.Name)
}

func loadActorTemplate(ctx context.Context, c client.Client, namespace, name string) (*v1alpha1.ActorTemplate, error) {
	var at v1alpha1.ActorTemplate
	key := types.NamespacedName{Namespace: namespace, Name: name}
	if err := c.Get(ctx, key, &at); err != nil {
		return nil, fmt.Errorf("get actortemplate %s: %w", name, err)
	}
	return &at, nil
}
