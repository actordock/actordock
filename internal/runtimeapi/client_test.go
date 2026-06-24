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

package runtimeapi

import (
	"context"
	"errors"
	"testing"

	"github.com/actordock/runtime/pkg/proto/runtimeapipb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestClient(api runtimeapipb.ControlClient) *Client {
	return &Client{api: api}
}

type fakeControl struct {
	getStatus    runtimeapipb.Actor_Status
	podIP        string
	getErr       error
	suspendCalls int
	suspendErr   error
	resumeErr    error
	deleteCalls  int
	deleteErr    error
}

func (f *fakeControl) actor(actorID string, status runtimeapipb.Actor_Status) *runtimeapipb.Actor {
	return &runtimeapipb.Actor{
		ActorId:    actorID,
		Status:     status,
		SandboxPodIp: f.podIP,
	}
}

func (f *fakeControl) GetActor(_ context.Context, req *runtimeapipb.GetActorRequest, _ ...grpc.CallOption) (*runtimeapipb.GetActorResponse, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return &runtimeapipb.GetActorResponse{
		Actor: f.actor(req.GetActorId(), f.getStatus),
	}, nil
}

func (f *fakeControl) SuspendActor(_ context.Context, req *runtimeapipb.SuspendActorRequest, _ ...grpc.CallOption) (*runtimeapipb.SuspendActorResponse, error) {
	f.suspendCalls++
	if f.suspendErr != nil {
		return nil, f.suspendErr
	}
	return &runtimeapipb.SuspendActorResponse{
		Actor: &runtimeapipb.Actor{
			ActorId: req.GetActorId(),
			Status:  runtimeapipb.Actor_STATUS_SUSPENDED,
		},
	}, nil
}

func (f *fakeControl) ResumeActor(_ context.Context, req *runtimeapipb.ResumeActorRequest, _ ...grpc.CallOption) (*runtimeapipb.ResumeActorResponse, error) {
	if f.resumeErr != nil {
		return nil, f.resumeErr
	}
	return &runtimeapipb.ResumeActorResponse{
		Actor: f.actor(req.GetActorId(), runtimeapipb.Actor_STATUS_RUNNING),
	}, nil
}

func (f *fakeControl) DeleteActor(_ context.Context, _ *runtimeapipb.DeleteActorRequest, _ ...grpc.CallOption) (*runtimeapipb.DeleteActorResponse, error) {
	f.deleteCalls++
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	return &runtimeapipb.DeleteActorResponse{}, nil
}

func (f *fakeControl) CreateActor(context.Context, *runtimeapipb.CreateActorRequest, ...grpc.CallOption) (*runtimeapipb.CreateActorResponse, error) {
	return nil, status.Error(codes.Unimplemented, "CreateActor")
}

func (f *fakeControl) UpdateActor(context.Context, *runtimeapipb.UpdateActorRequest, ...grpc.CallOption) (*runtimeapipb.UpdateActorResponse, error) {
	return nil, status.Error(codes.Unimplemented, "UpdateActor")
}

func (f *fakeControl) PauseActor(context.Context, *runtimeapipb.PauseActorRequest, ...grpc.CallOption) (*runtimeapipb.PauseActorResponse, error) {
	return nil, status.Error(codes.Unimplemented, "PauseActor")
}

func (f *fakeControl) ListWorkers(context.Context, *runtimeapipb.ListWorkersRequest, ...grpc.CallOption) (*runtimeapipb.ListWorkersResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ListWorkers")
}

func (f *fakeControl) ListActors(context.Context, *runtimeapipb.ListActorsRequest, ...grpc.CallOption) (*runtimeapipb.ListActorsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ListActors")
}

func (f *fakeControl) DebugClear(context.Context, *runtimeapipb.DebugClearRequest, ...grpc.CallOption) (*runtimeapipb.DebugClearResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DebugClear")
}

func TestResumeSandbox(t *testing.T) {
	t.Parallel()

	fake := &fakeControl{getStatus: runtimeapipb.Actor_STATUS_SUSPENDED}
	client := newTestClient(fake)

	if err := client.ResumeSandbox(context.Background(), "actor-1"); err != nil {
		t.Fatalf("ResumeSandbox() = %v, want nil", err)
	}
}

func TestGetActorBackend(t *testing.T) {
	t.Parallel()

	fake := &fakeControl{getStatus: runtimeapipb.Actor_STATUS_RUNNING, podIP: "10.0.0.5"}
	client := newTestClient(fake)

	backend, err := client.GetActorBackend(context.Background(), "actor-1", 80)
	if err != nil {
		t.Fatalf("GetActorBackend() = %v, want nil", err)
	}
	if backend != "10.0.0.5:80" {
		t.Fatalf("backend = %q, want 10.0.0.5:80", backend)
	}
}

func TestGetActorBackendNoWorker(t *testing.T) {
	t.Parallel()

	fake := &fakeControl{getStatus: runtimeapipb.Actor_STATUS_RUNNING}
	client := newTestClient(fake)

	_, err := client.GetActorBackend(context.Background(), "actor-1", 80)
	if err == nil {
		t.Fatal("GetActorBackend() = nil, want error")
	}
}

func TestResumeSandboxBackendRunning(t *testing.T) {
	t.Parallel()

	fake := &fakeControl{getStatus: runtimeapipb.Actor_STATUS_RUNNING, podIP: "10.0.0.5"}
	client := newTestClient(fake)

	backend, waitEnvd, err := client.ResumeSandboxBackend(context.Background(), "actor-1", 80)
	if err != nil {
		t.Fatalf("ResumeSandboxBackend() = %v, want nil", err)
	}
	if backend != "10.0.0.5:80" {
		t.Fatalf("backend = %q, want 10.0.0.5:80", backend)
	}
	if waitEnvd {
		t.Fatal("waitEnvd = true, want false")
	}
}

func TestResumeSandboxBackendSuspended(t *testing.T) {
	t.Parallel()

	fake := &fakeControl{getStatus: runtimeapipb.Actor_STATUS_SUSPENDED, podIP: "10.0.0.5"}
	client := newTestClient(fake)

	backend, waitEnvd, err := client.ResumeSandboxBackend(context.Background(), "actor-1", 80)
	if err != nil {
		t.Fatalf("ResumeSandboxBackend() = %v, want nil", err)
	}
	if backend != "10.0.0.5:80" {
		t.Fatalf("backend = %q, want 10.0.0.5:80", backend)
	}
	if !waitEnvd {
		t.Fatal("waitEnvd = false, want true")
	}
}

func TestResumeSandboxNotFound(t *testing.T) {
	t.Parallel()

	fake := &fakeControl{
		resumeErr: status.Error(codes.NotFound, "actor not found"),
	}
	client := newTestClient(fake)

	err := client.ResumeSandbox(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ResumeSandbox() = %v, want ErrNotFound", err)
	}
}

func TestSuspendSandbox(t *testing.T) {
	t.Parallel()

	fake := &fakeControl{getStatus: runtimeapipb.Actor_STATUS_RUNNING}
	client := newTestClient(fake)

	if err := client.SuspendSandbox(context.Background(), "actor-1"); err != nil {
		t.Fatalf("SuspendSandbox() = %v, want nil", err)
	}
	if fake.suspendCalls != 1 {
		t.Fatalf("suspendCalls = %d, want 1", fake.suspendCalls)
	}
}

func TestSuspendSandboxAlreadySuspended(t *testing.T) {
	t.Parallel()

	fake := &fakeControl{getStatus: runtimeapipb.Actor_STATUS_SUSPENDED}
	client := newTestClient(fake)

	if err := client.SuspendSandbox(context.Background(), "actor-1"); err != nil {
		t.Fatalf("SuspendSandbox() = %v, want nil", err)
	}
	if fake.suspendCalls != 0 {
		t.Fatalf("suspendCalls = %d, want 0", fake.suspendCalls)
	}
}

func TestSuspendSandboxNotFoundOnGet(t *testing.T) {
	t.Parallel()

	fake := &fakeControl{
		getErr: status.Error(codes.NotFound, "actor not found"),
	}
	client := newTestClient(fake)

	err := client.SuspendSandbox(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("SuspendSandbox() = %v, want ErrNotFound", err)
	}
	if fake.suspendCalls != 0 {
		t.Fatalf("suspendCalls = %d, want 0", fake.suspendCalls)
	}
}

func TestSuspendSandboxNotFoundOnSuspend(t *testing.T) {
	t.Parallel()

	fake := &fakeControl{
		getStatus:  runtimeapipb.Actor_STATUS_RUNNING,
		suspendErr: status.Error(codes.NotFound, "actor not found"),
	}
	client := newTestClient(fake)

	err := client.SuspendSandbox(context.Background(), "actor-1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("SuspendSandbox() = %v, want ErrNotFound", err)
	}
}

func TestDeleteSandboxUsesSuspendSandbox(t *testing.T) {
	t.Parallel()

	fake := &fakeControl{getStatus: runtimeapipb.Actor_STATUS_RUNNING}
	client := newTestClient(fake)

	if err := client.DeleteSandbox(context.Background(), "actor-1"); err != nil {
		t.Fatalf("DeleteSandbox() = %v, want nil", err)
	}
	if fake.suspendCalls != 1 {
		t.Fatalf("suspendCalls = %d, want 1", fake.suspendCalls)
	}
	if fake.deleteCalls != 1 {
		t.Fatalf("deleteCalls = %d, want 1", fake.deleteCalls)
	}
}
