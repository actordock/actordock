// Copyright 2026 Google LLC
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

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/actordock/runtime/internal/sandboxpath"
	"github.com/actordock/runtime/internal/proto/runtimeworkerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "actor-id")

	// One shared write over an existing value, as happens on every resume;
	// each subtest checks one postcondition.
	if err := os.WriteFile(target, []byte("golden-id"), 0o600); err != nil {
		t.Fatalf("seeding target: %v", err)
	}
	if err := writeFileAtomic(target, []byte("counter-1"), 0o644); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	t.Run("replaces content", func(t *testing.T) {
		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("reading target: %v", err)
		}
		if string(got) != "counter-1" {
			t.Errorf("content = %q, want %q", got, "counter-1")
		}
	})

	t.Run("sets permissions", func(t *testing.T) {
		info, err := os.Stat(target)
		if err != nil {
			t.Fatalf("stat target: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o644 {
			t.Errorf("perm = %o, want 644", perm)
		}
	})

	t.Run("leaves no temp files", func(t *testing.T) {
		// The directory is visible inside the actor.
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("reading dir: %v", err)
		}
		if len(entries) != 1 {
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Errorf("leftover files in identity dir: %v", names)
		}
	})
}

func TestValidateActorRequest(t *testing.T) {
	const okNS, okTmpl, okID, okUID = "runtime-demo", "counter", "counter-1", "422938ba-8860-4983-a25d-d6bcb0a69d4e"
	okSpec := &runtimeworkerpb.WorkloadSpec{Containers: []*runtimeworkerpb.Container{{Name: "worker"}}}

	tests := []struct {
		name              string
		ns, tmpl, id, uid string
		spec              *runtimeworkerpb.WorkloadSpec
		wantErr           bool
	}{
		{"all valid", okNS, okTmpl, okID, okUID, okSpec, false},
		{"bad namespace", "../x", okTmpl, okID, okUID, okSpec, true},
		{"bad actor id", okNS, okTmpl, "../x", okUID, okSpec, true},
		{"bad uid", okNS, okTmpl, okID, "../x", okSpec, true},
		{"bad container", okNS, okTmpl, okID, okUID, &runtimeworkerpb.WorkloadSpec{Containers: []*runtimeworkerpb.Container{{Name: "../x"}}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateActorRequest(tc.ns, tc.tmpl, tc.id, tc.uid, tc.spec); (err != nil) != tc.wantErr {
				t.Errorf("validateActorRequest err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// validRunRequest, validCheckpointRequest, and validRestoreRequest build
// requests whose every field passes validation; the per-request tests below
// break one field per case.
func validRunRequest() *runtimeworkerpb.RunRequest {
	return &runtimeworkerpb.RunRequest{
		ActorTemplateNamespace: "runtime-demo",
		ActorTemplateName:      "counter",
		ActorId:                "counter-1",
		TargetSandboxPodUid:         "422938ba-8860-4983-a25d-d6bcb0a69d4e",
		Spec:                   &runtimeworkerpb.WorkloadSpec{Containers: []*runtimeworkerpb.Container{{Name: "worker"}}},
	}
}

func validCheckpointRequest() *runtimeworkerpb.CheckpointRequest {
	return &runtimeworkerpb.CheckpointRequest{
		ActorTemplateNamespace: "runtime-demo",
		ActorTemplateName:      "counter",
		ActorId:                "counter-1",
		TargetSandboxPodUid:         "422938ba-8860-4983-a25d-d6bcb0a69d4e",
		Spec:                   &runtimeworkerpb.WorkloadSpec{Containers: []*runtimeworkerpb.Container{{Name: "worker"}}},
		Type:                   runtimeworkerpb.CheckpointType_CHECKPOINT_TYPE_EXTERNAL,
		Config: &runtimeworkerpb.CheckpointRequest_ExternalConfig{
			ExternalConfig: &runtimeworkerpb.ExternalCheckpointConfiguration{
				SnapshotUriPrefix: "gs://bucket/actors/1/snapshots/2/",
			},
		},
	}
}

func validRestoreRequest() *runtimeworkerpb.RestoreRequest {
	return &runtimeworkerpb.RestoreRequest{
		ActorTemplateNamespace: "runtime-demo",
		ActorTemplateName:      "counter",
		ActorId:                "counter-1",
		TargetSandboxPodUid:         "422938ba-8860-4983-a25d-d6bcb0a69d4e",
		Spec:                   &runtimeworkerpb.WorkloadSpec{Containers: []*runtimeworkerpb.Container{{Name: "worker"}}},
		Type:                   runtimeworkerpb.CheckpointType_CHECKPOINT_TYPE_EXTERNAL,
		Config: &runtimeworkerpb.RestoreRequest_ExternalConfig{
			ExternalConfig: &runtimeworkerpb.ExternalCheckpointConfiguration{
				SnapshotUriPrefix: "gs://bucket/actors/1/snapshots/2/",
			},
		},
	}
}

func TestValidateRunRequest(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*runtimeworkerpb.RunRequest)
		wantErr bool
	}{
		{"valid", func(*runtimeworkerpb.RunRequest) {}, false},
		{"invalid runtime-sandbox uid", func(r *runtimeworkerpb.RunRequest) { r.TargetSandboxPodUid = "../escape" }, true},
		{"invalid actor id", func(r *runtimeworkerpb.RunRequest) { r.ActorId = "../escape" }, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := validRunRequest()
			tc.mutate(req)
			if err := validateRunRequest(req); (err != nil) != tc.wantErr {
				t.Errorf("validateRunRequest err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// Checkpoint and Restore must reject a bad snapshot URI prefix even when
// every common field is valid.
func TestValidateCheckpointRequest(t *testing.T) {
	makeReq := func(opts ...func(*runtimeworkerpb.CheckpointRequest)) *runtimeworkerpb.CheckpointRequest {
		r := validCheckpointRequest()
		for _, opt := range opts {
			opt(r)
		}
		return r
	}

	tests := []struct {
		name    string
		req     *runtimeworkerpb.CheckpointRequest
		wantErr bool
	}{
		{"valid", makeReq(), false},
		{"empty snapshot uri", makeReq(func(r *runtimeworkerpb.CheckpointRequest) { r.GetExternalConfig().SnapshotUriPrefix = "" }), true},
		{"bucketless snapshot uri", makeReq(func(r *runtimeworkerpb.CheckpointRequest) { r.GetExternalConfig().SnapshotUriPrefix = "relative/path" }), true},
		{"invalid runtime-sandbox uid", makeReq(func(r *runtimeworkerpb.CheckpointRequest) { r.TargetSandboxPodUid = "../escape" }), true},
		{"invalid local snapshot prefix", makeReq(func(r *runtimeworkerpb.CheckpointRequest) {
			r.Type = runtimeworkerpb.CheckpointType_CHECKPOINT_TYPE_LOCAL
			r.Config = &runtimeworkerpb.CheckpointRequest_LocalConfig{LocalConfig: &runtimeworkerpb.LocalCheckpointConfiguration{SnapshotPrefix: ""}}
		}), true},
		{"unspecified snapshot type", makeReq(func(r *runtimeworkerpb.CheckpointRequest) { r.Type = runtimeworkerpb.CheckpointType_CHECKPOINT_TYPE_UNSPECIFIED }), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateCheckpointRequest(tc.req); (err != nil) != tc.wantErr {
				t.Errorf("validateCheckpointRequest err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateRestoreRequest(t *testing.T) {
	makeReq := func(opts ...func(*runtimeworkerpb.RestoreRequest)) *runtimeworkerpb.RestoreRequest {
		r := validRestoreRequest()
		for _, opt := range opts {
			opt(r)
		}
		return r
	}

	tests := []struct {
		name    string
		req     *runtimeworkerpb.RestoreRequest
		wantErr bool
	}{
		{"valid", makeReq(), false},
		{"empty snapshot uri", makeReq(func(r *runtimeworkerpb.RestoreRequest) { r.GetExternalConfig().SnapshotUriPrefix = "" }), true},
		{"bucketless snapshot uri", makeReq(func(r *runtimeworkerpb.RestoreRequest) { r.GetExternalConfig().SnapshotUriPrefix = "relative/path" }), true},
		{"invalid runtime-sandbox uid", makeReq(func(r *runtimeworkerpb.RestoreRequest) { r.TargetSandboxPodUid = "../escape" }), true},
		{"invalid local snapshot prefix", makeReq(func(r *runtimeworkerpb.RestoreRequest) {
			r.Type = runtimeworkerpb.CheckpointType_CHECKPOINT_TYPE_LOCAL
			r.Config = &runtimeworkerpb.RestoreRequest_LocalConfig{LocalConfig: &runtimeworkerpb.LocalCheckpointConfiguration{SnapshotPrefix: ""}}
		}), true},
		{"unspecified snapshot type", makeReq(func(r *runtimeworkerpb.RestoreRequest) { r.Type = runtimeworkerpb.CheckpointType_CHECKPOINT_TYPE_UNSPECIFIED }), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateRestoreRequest(tc.req); (err != nil) != tc.wantErr {
				t.Errorf("validateRestoreRequest err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// TestFetchAssetRejectsBadHash confirms fetchAsset validates the asset hash
// before the cache-hit os.Stat/early-return, not merely "at some point". To
// prove the ordering, it plants a real file at the exact path an invalid hash
// resolves to: a correctly-ordered fetchAsset validates first and returns an
// error, while a regression that stats first would find this file and return it
// with a nil error, failing the test. StaticFilesDir is redirected to a temp
// dir so the planted path is writable and isolated.
func TestFetchAssetRejectsBadHash(t *testing.T) {
	orig := sandboxpath.StaticFilesDir
	sandboxpath.StaticFilesDir = t.TempDir()
	t.Cleanup(func() { sandboxpath.StaticFilesDir = orig })

	// Invalid (8 chars, not 64) but separator-free, so it resolves to a normal
	// filename inside the temp StaticFilesDir.
	const badHash = "deadbeef"
	if err := os.WriteFile(sandboxpath.RunSCBinaryPath(badHash), []byte("planted"), 0o755); err != nil {
		t.Fatalf("planting cache file: %v", err)
	}

	s := &SandboxHerder{}
	if _, err := s.fetchAsset(context.Background(), assetEntry{SHA256: badHash}); err == nil {
		t.Error("fetchAsset returned a cache hit for an invalid hash; validation must run before the os.Stat early return")
	}
}

// TestRPCBoundariesReject confirms each of the three RPCs validates path inputs
// before touching its (here nil) dependencies. A traversal value must be
// rejected as InvalidArgument rather than panicking or surfacing as
// Internal. Guards against a future removal or reordering of the validation
// call at any boundary.
func TestRPCBoundariesReject(t *testing.T) {
	s := &SandboxHerder{}
	ctx := context.Background()
	badUID := "../escape" // valid actor ref, invalid runtime-sandbox UID
	const okNS, okTmpl, okID = "runtime-demo", "counter", "counter-1"
	okSpec := &runtimeworkerpb.WorkloadSpec{Containers: []*runtimeworkerpb.Container{{Name: "worker"}}}

	wantInvalidArgument := func(t *testing.T, rpc string, err error) {
		t.Helper()
		if err == nil {
			t.Errorf("%s accepted an invalid target runtime-sandbox UID", rpc)
			return
		}
		if code := status.Code(err); code != codes.InvalidArgument {
			t.Errorf("%s returned code %v, want InvalidArgument", rpc, code)
		}
	}

	t.Run("Run", func(t *testing.T) {
		_, err := s.Run(ctx, &runtimeworkerpb.RunRequest{
			ActorTemplateNamespace: okNS, ActorTemplateName: okTmpl, ActorId: okID,
			TargetSandboxPodUid: badUID, Spec: okSpec,
		})
		wantInvalidArgument(t, "Run", err)
	})
	t.Run("Checkpoint", func(t *testing.T) {
		_, err := s.Checkpoint(ctx, &runtimeworkerpb.CheckpointRequest{
			ActorTemplateNamespace: okNS, ActorTemplateName: okTmpl, ActorId: okID,
			TargetSandboxPodUid: badUID, Spec: okSpec,
		})
		wantInvalidArgument(t, "Checkpoint", err)
	})
	t.Run("Restore", func(t *testing.T) {
		_, err := s.Restore(ctx, &runtimeworkerpb.RestoreRequest{
			ActorTemplateNamespace: okNS, ActorTemplateName: okTmpl, ActorId: okID,
			TargetSandboxPodUid: badUID, Spec: okSpec,
		})
		wantInvalidArgument(t, "Restore", err)
	})
}
