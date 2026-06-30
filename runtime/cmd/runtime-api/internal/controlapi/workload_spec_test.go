//  Copyright 2026 Google LLC
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package controlapi

import (
	"context"
	"testing"

	"github.com/actordock/runtime/internal/proto/runtimeworkerpb"
	runtimev1alpha1 "github.com/actordock/runtime/pkg/api/v1alpha1"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
)

func TestWorkloadSpecFromActorTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template *runtimev1alpha1.ActorTemplate
		want     *runtimeworkerpb.WorkloadSpec
	}{
		{
			name: "converts DurableDir volume and mounts",
			template: &runtimev1alpha1.ActorTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "tmpl1", Namespace: "agent-ns"},
				Spec: runtimev1alpha1.ActorTemplateSpec{
					PauseImage: "pause",
					Volumes: []runtimev1alpha1.Volume{
						{Name: "home", VolumeSource: runtimev1alpha1.VolumeSource{DurableDir: &runtimev1alpha1.DurableDirVolumeSource{}}},
					},
					Containers: []runtimev1alpha1.Container{
						{
							Name:  "main",
							Image: "main",
							VolumeMounts: []runtimev1alpha1.VolumeMount{
								{Name: "home", MountPath: "/home/user"},
								{Name: "home", MountPath: "/workspace"},
							},
						},
					},
				},
			},
			want: &runtimeworkerpb.WorkloadSpec{
				PauseImage: "pause",
				Volumes: []*runtimeworkerpb.Volume{
					{
						Name:   "home",
						Type:   runtimeworkerpb.VolumeType_VOLUME_TYPE_DURABLE_DIR,
						Source: &runtimeworkerpb.Volume_DurableDir{DurableDir: &runtimeworkerpb.DurableDirVolume{}},
					},
				},
				Containers: []*runtimeworkerpb.Container{
					{
						Name:  "main",
						Image: "main",
						VolumeMounts: []*runtimeworkerpb.VolumeMount{
							{Name: "home", MountPath: "/home/user"},
							{Name: "home", MountPath: "/workspace"},
						},
					},
				},
			},
		},
		{
			name: "skips non-DurableDir volumes",
			template: &runtimev1alpha1.ActorTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "tmpl1", Namespace: "agent-ns"},
				Spec: runtimev1alpha1.ActorTemplateSpec{
					Volumes: []runtimev1alpha1.Volume{
						{Name: "unsupported", VolumeSource: runtimev1alpha1.VolumeSource{}},
						{Name: "home", VolumeSource: runtimev1alpha1.VolumeSource{DurableDir: &runtimev1alpha1.DurableDirVolumeSource{}}},
					},
					Containers: []runtimev1alpha1.Container{
						{
							Name:  "main",
							Image: "main",
							VolumeMounts: []runtimev1alpha1.VolumeMount{
								{Name: "home", MountPath: "/workspace"},
							},
						},
					},
				},
			},
			want: &runtimeworkerpb.WorkloadSpec{
				Volumes: []*runtimeworkerpb.Volume{
					{
						Name:   "home",
						Type:   runtimeworkerpb.VolumeType_VOLUME_TYPE_DURABLE_DIR,
						Source: &runtimeworkerpb.Volume_DurableDir{DurableDir: &runtimeworkerpb.DurableDirVolume{}},
					},
				},
				Containers: []*runtimeworkerpb.Container{
					{
						Name:  "main",
						Image: "main",
						VolumeMounts: []*runtimeworkerpb.VolumeMount{
							{Name: "home", MountPath: "/workspace"},
						},
					},
				},
			},
		},
		{
			name: "container without volume mounts has none",
			template: &runtimev1alpha1.ActorTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "tmpl1", Namespace: "agent-ns"},
				Spec: runtimev1alpha1.ActorTemplateSpec{
					Volumes: []runtimev1alpha1.Volume{
						{Name: "home", VolumeSource: runtimev1alpha1.VolumeSource{DurableDir: &runtimev1alpha1.DurableDirVolumeSource{}}},
					},
					Containers: []runtimev1alpha1.Container{
						{Name: "main", Image: "main"},
					},
				},
			},
			want: &runtimeworkerpb.WorkloadSpec{
				Volumes: []*runtimeworkerpb.Volume{
					{
						Name:   "home",
						Type:   runtimeworkerpb.VolumeType_VOLUME_TYPE_DURABLE_DIR,
						Source: &runtimeworkerpb.Volume_DurableDir{DurableDir: &runtimeworkerpb.DurableDirVolume{}},
					},
				},
				Containers: []*runtimeworkerpb.Container{{Name: "main", Image: "main"}},
			},
		},
		{
			name: "ignores container env",
			template: &runtimev1alpha1.ActorTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "tmpl1", Namespace: "agent-ns"},
				Spec: runtimev1alpha1.ActorTemplateSpec{
					Containers: []runtimev1alpha1.Container{
						{
							Name:  "main",
							Image: "main",
							Env: []runtimev1alpha1.EnvVar{
								{Name: "LITERAL", Value: ptr.To("plain")},
								{
									Name: "SECRET",
									ValueFrom: &runtimev1alpha1.EnvVarSource{
										SecretKeyRef: &runtimev1alpha1.SecretKeySelector{Name: "any", Key: "any"},
									},
								},
							},
						},
					},
				},
			},
			want: &runtimeworkerpb.WorkloadSpec{
				Containers: []*runtimeworkerpb.Container{{Name: "main", Image: "main"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := workloadSpecFromActorTemplate(tt.template)
			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("WorkloadSpec mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestWorkloadSpecFromActorTemplateWithEnv(t *testing.T) {
	tests := []struct {
		name        string
		secrets     []runtime.Object
		template    *runtimev1alpha1.ActorTemplate
		want        *runtimeworkerpb.WorkloadSpec
		wantErrCode codes.Code
	}{
		{
			name: "resolves literal and secretKeyRef env",
			secrets: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "some-secret", Namespace: "agent-ns"},
					Data:       map[string][]byte{"some-key": []byte("some-value")},
				},
			},
			template: &runtimev1alpha1.ActorTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "tmpl1", Namespace: "agent-ns"},
				Spec: runtimev1alpha1.ActorTemplateSpec{
					PauseImage: "pause",
					Containers: []runtimev1alpha1.Container{
						{
							Name:    "main",
							Image:   "main",
							Command: []string{"/main"},
							Env: []runtimev1alpha1.EnvVar{
								{Name: "LITERAL", Value: ptr.To("plain")},
								{
									Name: "SOME_KEY",
									ValueFrom: &runtimev1alpha1.EnvVarSource{
										SecretKeyRef: &runtimev1alpha1.SecretKeySelector{Name: "some-secret", Key: "some-key"},
									},
								},
							},
						},
					},
				},
			},
			want: &runtimeworkerpb.WorkloadSpec{
				PauseImage: "pause",
				Containers: []*runtimeworkerpb.Container{
					{
						Name:    "main",
						Image:   "main",
						Command: []string{"/main"},
						Env: []*runtimeworkerpb.EnvEntry{
							{Name: "LITERAL", Value: "plain"},
							{Name: "SOME_KEY", Value: "some-value"},
						},
					},
				},
			},
		},
		{
			name: "skips optional missing secret",
			template: &runtimev1alpha1.ActorTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "tmpl1", Namespace: "agent-ns"},
				Spec: runtimev1alpha1.ActorTemplateSpec{
					Containers: []runtimev1alpha1.Container{
						{
							Name:  "main",
							Image: "main",
							Env: []runtimev1alpha1.EnvVar{
								{
									Name: "OPTIONAL",
									ValueFrom: &runtimev1alpha1.EnvVarSource{
										SecretKeyRef: &runtimev1alpha1.SecretKeySelector{Name: "missing", Key: "key", Optional: ptr.To(true)},
									},
								},
							},
						},
					},
				},
			},
			want: &runtimeworkerpb.WorkloadSpec{
				Containers: []*runtimeworkerpb.Container{{Name: "main", Image: "main"}},
			},
		},
		{
			name: "required missing secret fails",
			template: &runtimev1alpha1.ActorTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "tmpl1", Namespace: "agent-ns"},
				Spec: runtimev1alpha1.ActorTemplateSpec{
					Containers: []runtimev1alpha1.Container{
						{
							Name:  "main",
							Image: "main",
							Env: []runtimev1alpha1.EnvVar{
								{
									Name: "REQUIRED",
									ValueFrom: &runtimev1alpha1.EnvVarSource{
										SecretKeyRef: &runtimev1alpha1.SecretKeySelector{Name: "missing", Key: "key"},
									},
								},
							},
						},
					},
				},
			},
			wantErrCode: codes.FailedPrecondition,
		},
		{
			name: "empty valueFrom fails",
			template: &runtimev1alpha1.ActorTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "tmpl1", Namespace: "agent-ns"},
				Spec: runtimev1alpha1.ActorTemplateSpec{
					Containers: []runtimev1alpha1.Container{
						{
							Name:  "main",
							Image: "main",
							Env: []runtimev1alpha1.EnvVar{
								{Name: "EMPTY", ValueFrom: &runtimev1alpha1.EnvVarSource{}},
							},
						},
					},
				},
			},
			wantErrCode: codes.FailedPrecondition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeClient := fake.NewSimpleClientset(tt.secrets...)
			got, err := workloadSpecFromActorTemplateWithEnv(context.Background(), kubeClient, nil, tt.template)
			if tt.wantErrCode != codes.OK {
				if status.Code(err) != tt.wantErrCode {
					t.Fatalf("error code = %v, want %v: %v", status.Code(err), tt.wantErrCode, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("workloadSpecFromActorTemplateWithEnv failed: %v", err)
			}
			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("WorkloadSpec mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestWorkloadSpecFromActorTemplatePropagatesReadyz(t *testing.T) {
	got := workloadSpecFromActorTemplate(&runtimev1alpha1.ActorTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "tmpl-readyz", Namespace: "agent-ns"},
		Spec: runtimev1alpha1.ActorTemplateSpec{
			Containers: []runtimev1alpha1.Container{
				{
					Name:  "with-probe",
					Image: "main",
					Readyz: &runtimev1alpha1.ContainerReadyz{
						HTTPGet: &runtimev1alpha1.HTTPGetAction{Path: "/health", Port: 8080},
					},
				},
				{
					Name:  "without-probe",
					Image: "side",
				},
			},
		},
	})

	want := &runtimeworkerpb.WorkloadSpec{
		Containers: []*runtimeworkerpb.Container{
			{
				Name:  "with-probe",
				Image: "main",
				Readyz: &runtimeworkerpb.Readyz{
					HttpGet: &runtimeworkerpb.HTTPGetAction{Path: "/health", Port: 8080},
				},
			},
			{
				Name:  "without-probe",
				Image: "side",
			},
		},
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("WorkloadSpec mismatch (-want +got):\n%s", diff)
	}
}

func TestWorkloadSpecFromActorTemplateWithEnvCachesSecretsAcrossCalls(t *testing.T) {
	ctx := context.Background()
	secretCache := newEnvSecretCache(envSecretCacheTTL)
	kubeClient := fake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "some-secret",
				Namespace: "agent-ns",
			},
			Data: map[string][]byte{
				"some-key": []byte("some-value"),
			},
		},
	)
	actorTemplate := &runtimev1alpha1.ActorTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tmpl1",
			Namespace: "agent-ns",
		},
		Spec: runtimev1alpha1.ActorTemplateSpec{
			Containers: []runtimev1alpha1.Container{
				{
					Name:  "main",
					Image: "main",
					Env: []runtimev1alpha1.EnvVar{
						{
							Name: "SOME_KEY",
							ValueFrom: &runtimev1alpha1.EnvVarSource{
								SecretKeyRef: &runtimev1alpha1.SecretKeySelector{
									Name: "some-secret",
									Key:  "some-key",
								},
							},
						},
					},
				},
			},
		},
	}

	if _, err := workloadSpecFromActorTemplateWithEnv(ctx, kubeClient, secretCache, actorTemplate); err != nil {
		t.Fatalf("first workloadSpecFromActorTemplateWithEnv failed: %v", err)
	}
	if _, err := workloadSpecFromActorTemplateWithEnv(ctx, kubeClient, secretCache, actorTemplate); err != nil {
		t.Fatalf("second workloadSpecFromActorTemplateWithEnv failed: %v", err)
	}
	if got := secretGetCount(kubeClient); got != 1 {
		t.Fatalf("secret gets before TTL expiry = %d, want 1", got)
	}

	expireSecretCache(secretCache)
	if _, err := workloadSpecFromActorTemplateWithEnv(ctx, kubeClient, secretCache, actorTemplate); err != nil {
		t.Fatalf("third workloadSpecFromActorTemplateWithEnv failed: %v", err)
	}
	if got := secretGetCount(kubeClient); got != 2 {
		t.Fatalf("secret gets after TTL expiry = %d, want 2", got)
	}
}

func expireSecretCache(secretCache *envSecretCache) {
	secretCache.mu.Lock()
	defer secretCache.mu.Unlock()

	for key, entry := range secretCache.entries {
		entry.expiresAt = entry.expiresAt.Add(-envSecretCacheTTL)
		secretCache.entries[key] = entry
	}
}

func secretGetCount(kubeClient *fake.Clientset) int {
	count := 0
	for _, action := range kubeClient.Actions() {
		if action.GetVerb() == "get" && action.GetResource().Resource == "secrets" {
			count++
		}
	}
	return count
}
