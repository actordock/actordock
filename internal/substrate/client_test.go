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

package substrate

import (
	"context"
	"errors"
	"testing"

	"github.com/agent-substrate/substrate/pkg/proto/ateapipb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestClient(api ateapipb.ControlClient) *Client {
	return &Client{api: api}
}

type fakeControl struct {
	getStatus    ateapipb.Actor_Status
	getErr       error
	suspendCalls int
	suspendErr   error
	resumeErr    error
	deleteCalls  int
	deleteErr    error
}

func (f *fakeControl) GetActor(_ context.Context, req *ateapipb.GetActorRequest, _ ...grpc.CallOption) (*ateapipb.GetActorResponse, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return &ateapipb.GetActorResponse{
		Actor: &ateapipb.Actor{
			ActorId: req.GetActorId(),
			Status:  f.getStatus,
		},
	}, nil
}

func (f *fakeControl) SuspendActor(_ context.Context, req *ateapipb.SuspendActorRequest, _ ...grpc.CallOption) (*ateapipb.SuspendActorResponse, error) {
	f.suspendCalls++
	if f.suspendErr != nil {
		return nil, f.suspendErr
	}
	return &ateapipb.SuspendActorResponse{
		Actor: &ateapipb.Actor{
			ActorId: req.GetActorId(),
			Status:  ateapipb.Actor_STATUS_SUSPENDED,
		},
	}, nil
}

func (f *fakeControl) ResumeActor(_ context.Context, req *ateapipb.ResumeActorRequest, _ ...grpc.CallOption) (*ateapipb.ResumeActorResponse, error) {
	if f.resumeErr != nil {
		return nil, f.resumeErr
	}
	return &ateapipb.ResumeActorResponse{
		Actor: &ateapipb.Actor{
			ActorId: req.GetActorId(),
			Status:  ateapipb.Actor_STATUS_RUNNING,
		},
	}, nil
}

func (f *fakeControl) DeleteActor(_ context.Context, _ *ateapipb.DeleteActorRequest, _ ...grpc.CallOption) (*ateapipb.DeleteActorResponse, error) {
	f.deleteCalls++
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	return &ateapipb.DeleteActorResponse{}, nil
}

func (f *fakeControl) CreateActor(context.Context, *ateapipb.CreateActorRequest, ...grpc.CallOption) (*ateapipb.CreateActorResponse, error) {
	return nil, status.Error(codes.Unimplemented, "CreateActor")
}

func (f *fakeControl) UpdateActor(context.Context, *ateapipb.UpdateActorRequest, ...grpc.CallOption) (*ateapipb.UpdateActorResponse, error) {
	return nil, status.Error(codes.Unimplemented, "UpdateActor")
}

func (f *fakeControl) PauseActor(context.Context, *ateapipb.PauseActorRequest, ...grpc.CallOption) (*ateapipb.PauseActorResponse, error) {
	return nil, status.Error(codes.Unimplemented, "PauseActor")
}

func (f *fakeControl) ListWorkers(context.Context, *ateapipb.ListWorkersRequest, ...grpc.CallOption) (*ateapipb.ListWorkersResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ListWorkers")
}

func (f *fakeControl) ListActors(context.Context, *ateapipb.ListActorsRequest, ...grpc.CallOption) (*ateapipb.ListActorsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ListActors")
}

func (f *fakeControl) DebugClear(context.Context, *ateapipb.DebugClearRequest, ...grpc.CallOption) (*ateapipb.DebugClearResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DebugClear")
}

func TestResumeSandbox(t *testing.T) {
	t.Parallel()

	fake := &fakeControl{getStatus: ateapipb.Actor_STATUS_SUSPENDED}
	client := newTestClient(fake)

	if err := client.ResumeSandbox(context.Background(), "actor-1"); err != nil {
		t.Fatalf("ResumeSandbox() = %v, want nil", err)
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

	fake := &fakeControl{getStatus: ateapipb.Actor_STATUS_RUNNING}
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

	fake := &fakeControl{getStatus: ateapipb.Actor_STATUS_SUSPENDED}
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
		getStatus:  ateapipb.Actor_STATUS_RUNNING,
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

	fake := &fakeControl{getStatus: ateapipb.Actor_STATUS_RUNNING}
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
