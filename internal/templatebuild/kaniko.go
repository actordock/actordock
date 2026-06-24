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
	"path/filepath"
	"strings"
	"time"

	"github.com/actordock/actordock/internal/config"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// KanikoImageBuilder runs Kaniko as a Kubernetes Job to build and push images.
type KanikoImageBuilder struct {
	Client        client.Client
	Namespace     string
	BuildDataDir  string
	PVCName       string
	KanikoImage   string
	Insecure      bool
	JobTTLSeconds int32
}

func NewKanikoImageBuilder(cfg config.TemplateBuilder, c client.Client) *KanikoImageBuilder {
	return &KanikoImageBuilder{
		Client:        c,
		Namespace:     cfg.TemplateNamespace,
		BuildDataDir:  cfg.BuildDataDir,
		PVCName:       cfg.BuildPVCName,
		KanikoImage:   cfg.KanikoImage,
		Insecure:      cfg.BuildRegistryInsecure,
		JobTTLSeconds: 600,
	}
}

func (k *KanikoImageBuilder) Build(ctx context.Context, req BuildRequest) (string, error) {
	if strings.TrimSpace(req.ContextDir) == "" {
		return "", fmt.Errorf("context dir is required")
	}
	if strings.TrimSpace(req.Destination) == "" {
		return "", fmt.Errorf("destination is required")
	}
	if strings.TrimSpace(req.BuildID) == "" {
		return "", fmt.Errorf("build id is required")
	}

	subPath, err := contextSubPath(k.BuildDataDir, req.ContextDir)
	if err != nil {
		return "", err
	}

	jobName := kanikoJobName(req.BuildID)
	volumeName := "build-context"
	ttl := k.JobTTLSeconds
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: k.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": "template-builder",
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:  "kaniko",
						Image: k.KanikoImage,
						Args:  kanikoArgs(req.Destination, k.Insecure),
						VolumeMounts: []corev1.VolumeMount{{
							Name:      volumeName,
							MountPath: "/workspace",
							SubPath:   subPath,
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: volumeName,
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: k.PVCName,
							},
						},
					}},
				},
			},
		},
	}

	if err := k.Client.Create(ctx, job); err != nil {
		return "", fmt.Errorf("create kaniko job: %w", err)
	}
	defer func() { _ = k.Client.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)) }()

	if err := wait.PollUntilContextTimeout(ctx, 3*time.Second, 30*time.Minute, true, func(ctx context.Context) (bool, error) {
		var current batchv1.Job
		if err := k.Client.Get(ctx, client.ObjectKeyFromObject(job), &current); err != nil {
			return false, err
		}
		for _, cond := range current.Status.Conditions {
			if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
				return true, nil
			}
			if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
				return false, fmt.Errorf("kaniko job failed")
			}
		}
		return false, nil
	}); err != nil {
		return "", fmt.Errorf("wait for kaniko job: %w", err)
	}

	ref, err := name.ParseReference(req.Destination)
	if err != nil {
		return "", fmt.Errorf("parse destination: %w", err)
	}
	desc, err := remote.Get(ref, remoteOptions(k.Insecure)...)
	if err != nil {
		return "", fmt.Errorf("resolve built image digest: %w", err)
	}
	pinned := ref.Context().Digest(desc.Digest.String())
	return pinned.String(), nil
}

func contextSubPath(buildDataDir, contextDir string) (string, error) {
	rel, err := filepath.Rel(filepath.Clean(buildDataDir), filepath.Clean(contextDir))
	if err != nil {
		return "", fmt.Errorf("context path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("context dir must be under build data dir")
	}
	return filepath.ToSlash(rel), nil
}

func kanikoJobName(buildID string) string {
	name := "tpl-build-" + strings.ToLower(sanitizeImageTag(buildID))
	name = strings.NewReplacer(".", "-").Replace(name)
	if len(name) > 63 {
		name = name[:63]
	}
	return strings.TrimRight(name, "-")
}

func kanikoArgs(destination string, insecure bool) []string {
	args := []string{
		"--dockerfile=/workspace/Dockerfile",
		"--context=dir:///workspace",
		"--destination=" + destination,
	}
	if insecure {
		args = append(args, "--insecure", "--skip-tls-verify")
	}
	return args
}

func remoteOptions(insecure bool) []remote.Option {
	if !insecure {
		return nil
	}
	return []remote.Option{remote.WithTransport(insecureTransport())}
}
