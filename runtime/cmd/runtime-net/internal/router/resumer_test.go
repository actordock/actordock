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

package router

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/actordock/runtime/pkg/proto/runtimeapipb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type resumerMockClient struct {
	runtimeapipb.ControlClient
	resumeFn func(ctx context.Context, in *runtimeapipb.ResumeActorRequest, opts ...grpc.CallOption) (*runtimeapipb.ResumeActorResponse, error)
}

func (m *resumerMockClient) ResumeActor(ctx context.Context, in *runtimeapipb.ResumeActorRequest, opts ...grpc.CallOption) (*runtimeapipb.ResumeActorResponse, error) {
	if m.resumeFn != nil {
		return m.resumeFn(ctx, in, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func TestActorResumer_ResumeActor(t *testing.T) {
	const testActorID = "actor-a"
	const expectedIP = "10.0.0.52"

	t.Run("SuspendedResumedSuccessfully", func(t *testing.T) {
		var resumeCalled int
		mock := &resumerMockClient{
			resumeFn: func(ctx context.Context, in *runtimeapipb.ResumeActorRequest, opts ...grpc.CallOption) (*runtimeapipb.ResumeActorResponse, error) {
				resumeCalled++
				return &runtimeapipb.ResumeActorResponse{
					Actor: &runtimeapipb.Actor{
						ActorId:      testActorID,
						Status:       runtimeapipb.Actor_STATUS_RUNNING,
						SandboxPodIp: expectedIP,
					},
				}, nil
			},
		}

		resumer := NewActorResumer(mock)
		actor, err := resumer.ResumeActor(context.Background(), testActorID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if actor.GetSandboxPodIp() != expectedIP {
			t.Errorf("expected IP %q, got %q", expectedIP, actor.GetSandboxPodIp())
		}
		if resumeCalled != 1 {
			t.Errorf("expected ResumeActor called 1 time, got %d", resumeCalled)
		}
	})

	t.Run("RetryOnAbortedConflict", func(t *testing.T) {
		var resumeCalled int
		mock := &resumerMockClient{
			resumeFn: func(ctx context.Context, in *runtimeapipb.ResumeActorRequest, opts ...grpc.CallOption) (*runtimeapipb.ResumeActorResponse, error) {
				resumeCalled++
				if resumeCalled < 3 {
					return nil, status.Error(codes.Aborted, "concurrent update conflict")
				}
				return &runtimeapipb.ResumeActorResponse{
					Actor: &runtimeapipb.Actor{
						ActorId:      testActorID,
						Status:       runtimeapipb.Actor_STATUS_RUNNING,
						SandboxPodIp: expectedIP,
					},
				}, nil
			},
		}

		resumer := NewActorResumer(mock)
		actor, err := resumer.ResumeActor(context.Background(), testActorID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if actor.GetSandboxPodIp() != expectedIP {
			t.Errorf("expected IP %q, got %q", expectedIP, actor.GetSandboxPodIp())
		}
		if resumeCalled != 3 {
			t.Errorf("expected ResumeActor called 3 times, got %d", resumeCalled)
		}
	})

	t.Run("ActorNotFound", func(t *testing.T) {
		mock := &resumerMockClient{
			resumeFn: func(ctx context.Context, in *runtimeapipb.ResumeActorRequest, opts ...grpc.CallOption) (*runtimeapipb.ResumeActorResponse, error) {
				return nil, status.Error(codes.NotFound, "not found")
			},
		}

		resumer := NewActorResumer(mock)
		_, err := resumer.ResumeActor(context.Background(), testActorID)
		if got := status.Code(err); got != codes.NotFound {
			t.Errorf("expected gRPC code NotFound, got %v (err=%v)", got, err)
		}
	})

	t.Run("SingleflightDeduplication", func(t *testing.T) {
		var resumeCalled int
		var mu sync.Mutex

		mock := &resumerMockClient{
			resumeFn: func(ctx context.Context, in *runtimeapipb.ResumeActorRequest, opts ...grpc.CallOption) (*runtimeapipb.ResumeActorResponse, error) {
				mu.Lock()
				resumeCalled++
				mu.Unlock()
				time.Sleep(20 * time.Millisecond)
				return &runtimeapipb.ResumeActorResponse{
					Actor: &runtimeapipb.Actor{
						ActorId:      testActorID,
						Status:       runtimeapipb.Actor_STATUS_RUNNING,
						SandboxPodIp: expectedIP,
					},
				}, nil
			},
		}

		resumer := NewActorResumer(mock)

		var wg sync.WaitGroup
		const concurrentRequests = 10
		results := make([]*runtimeapipb.Actor, concurrentRequests)
		errs := make([]error, concurrentRequests)

		wg.Add(concurrentRequests)
		for i := 0; i < concurrentRequests; i++ {
			go func(idx int) {
				defer wg.Done()
				results[idx], errs[idx] = resumer.ResumeActor(context.Background(), testActorID)
			}(i)
		}
		wg.Wait()

		for i := 0; i < concurrentRequests; i++ {
			if errs[i] != nil {
				t.Fatalf("request %d failed: %v", i, errs[i])
			}
			if results[i].GetSandboxPodIp() != expectedIP {
				t.Errorf("request %d expected IP %q, got %q", i, expectedIP, results[i].GetSandboxPodIp())
			}
		}

		mu.Lock()
		defer mu.Unlock()
		if resumeCalled != 1 {
			t.Errorf("expected ResumeActor called exactly once by singleflight, got %d", resumeCalled)
		}
	})
}
