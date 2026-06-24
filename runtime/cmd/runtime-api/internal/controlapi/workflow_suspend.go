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

package controlapi

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/actordock/runtime/cmd/runtime-api/internal/store"
	"github.com/actordock/runtime/internal/proto/runtimeworkerpb"
	runtimev1alpha1 "github.com/actordock/runtime/pkg/api/v1alpha1"
	listersv1alpha1 "github.com/actordock/runtime/pkg/client/listers/api/v1alpha1"
	"github.com/actordock/runtime/pkg/proto/runtimeapipb"
	"k8s.io/apimachinery/pkg/util/wait"
)

// SuspendInput holds the immutable parameters requested by the client.
type SuspendInput struct {
	ActorID string
}

// SuspendState holds the mutable state loaded and modified during execution.
type SuspendState struct {
	Actor         *runtimeapipb.Actor
	ActorTemplate *runtimev1alpha1.ActorTemplate
}

type LoadActorForSuspendStep struct {
	store               store.Interface
	actorTemplateLister listersv1alpha1.ActorTemplateLister
}

func (s *LoadActorForSuspendStep) Name() string { return "LoadActorForSuspend" }
func (s *LoadActorForSuspendStep) IsComplete(ctx context.Context, input *SuspendInput, state *SuspendState) (bool, error) {
	// Always run to get the freshest state
	return false, nil
}
func (s *LoadActorForSuspendStep) Execute(ctx context.Context, input *SuspendInput, state *SuspendState) error {
	actor, err := s.store.GetActor(ctx, input.ActorID)
	if err != nil {
		return err
	}
	state.Actor = actor

	actorTemplate, err := s.actorTemplateLister.ActorTemplates(actor.GetActorTemplateNamespace()).Get(actor.GetActorTemplateName())
	if err != nil {
		return fmt.Errorf("while getting ActorTemplate: %w", err)
	}
	state.ActorTemplate = actorTemplate

	return nil
}

func (s *LoadActorForSuspendStep) RetryBackoff() *wait.Backoff { return nil }

type MarkSuspendingStep struct {
	store store.Interface
}

func (s *MarkSuspendingStep) Name() string { return "MarkSuspending" }
func (s *MarkSuspendingStep) IsComplete(ctx context.Context, input *SuspendInput, state *SuspendState) (bool, error) {
	// Fast forward if we've already marked our intent or if we are further along.
	return state.Actor.GetStatus() == runtimeapipb.Actor_STATUS_SUSPENDING || state.Actor.GetStatus() == runtimeapipb.Actor_STATUS_SUSPENDED, nil
}
func (s *MarkSuspendingStep) Execute(ctx context.Context, input *SuspendInput, state *SuspendState) error {
	if state.Actor.GetStatus() != runtimeapipb.Actor_STATUS_RUNNING {
		return nil
	}

	state.Actor.Status = runtimeapipb.Actor_STATUS_SUSPENDING
	snapshotID := time.Now().Format(time.RFC3339) + "-" + rand.Text()
	state.Actor.InProgressSnapshot = strings.TrimSuffix(state.ActorTemplate.Spec.SnapshotsConfig.Location, "/") + "/" + input.ActorID + "/" + snapshotID
	return s.store.UpdateActor(ctx, state.Actor, state.Actor.GetVersion())
}

func (s *MarkSuspendingStep) RetryBackoff() *wait.Backoff { return nil }

type CallAteletSuspendStep struct {
	dialer *WorkerDialer
}

func (s *CallAteletSuspendStep) Name() string { return "CallAteletSuspend" }
func (s *CallAteletSuspendStep) IsComplete(ctx context.Context, input *SuspendInput, state *SuspendState) (bool, error) {
	// If we are already SUSPENDED, we've already called Atelet
	return state.Actor.GetStatus() == runtimeapipb.Actor_STATUS_SUSPENDED, nil
}
func (s *CallAteletSuspendStep) Execute(ctx context.Context, input *SuspendInput, state *SuspendState) error {
	if state.Actor.GetSandboxPodNamespace() == "" {
		return fmt.Errorf("actor is in SUSPENDING state but has no active worker")
	}

	workerConn, err := s.dialer.DialForWorker(state.Actor.GetSandboxPodNamespace(), state.Actor.GetSandboxPodName())
	if err != nil {
		if errors.Is(err, ErrWorkerPodNotFound) {
			slog.Warn("Skipping suspend for dangling worker pod", "namespace", state.Actor.GetSandboxPodNamespace(), "pod", state.Actor.GetSandboxPodName())
			return nil
		}
		return fmt.Errorf("while getting runtime-worker conn for worker pod: %w", err)
	}
	client := runtimeworkerpb.NewWorkerHerderClient(workerConn)

	// Checkpoint does not carry the sandbox config: runtime-worker uses the version the
	// actor is currently running (recorded on-node at Run/Restore) and pins it
	// into the snapshot manifest.
	req := &runtimeworkerpb.CheckpointRequest{
		TargetSandboxPodUid:         state.Actor.GetSandboxPodUid(),
		ActorTemplateNamespace: state.Actor.GetActorTemplateNamespace(),
		ActorTemplateName:      state.Actor.GetActorTemplateName(),
		ActorId:                state.Actor.GetActorId(),
		Spec: &runtimeworkerpb.WorkloadSpec{
			PauseImage: state.ActorTemplate.Spec.PauseImage,
		},
		Type: runtimeworkerpb.CheckpointType_CHECKPOINT_TYPE_EXTERNAL,
		Config: &runtimeworkerpb.CheckpointRequest_ExternalConfig{
			ExternalConfig: &runtimeworkerpb.ExternalCheckpointConfiguration{
				SnapshotUriPrefix: state.Actor.GetInProgressSnapshot(),
			},
		},
	}
	for _, ctr := range state.ActorTemplate.Spec.Containers {
		workerCtr := &runtimeworkerpb.Container{
			Name:    ctr.Name,
			Image:   ctr.Image,
			Command: ctr.Command,
		}
		for _, env := range ctr.Env {
			var val string
			if env.Value != nil {
				val = *env.Value
			}
			workerEnv := &runtimeworkerpb.EnvEntry{
				Name:  env.Name,
				Value: val,
			}
			workerCtr.Env = append(workerCtr.Env, workerEnv)
		}
		req.Spec.Containers = append(req.Spec.Containers, workerCtr)
	}
	_, err = client.Checkpoint(ctx, req)
	if err != nil {
		return fmt.Errorf("while checkpointing workload: %w", err)
	}

	return nil
}

func (s *CallAteletSuspendStep) RetryBackoff() *wait.Backoff { return nil }

type FinalizeSuspendedStep struct {
	store store.Interface
}

func (s *FinalizeSuspendedStep) Name() string { return "FinalizeSuspended" }
func (s *FinalizeSuspendedStep) IsComplete(ctx context.Context, input *SuspendInput, state *SuspendState) (bool, error) {
	// The workflow is completely done ONLY if the status is SUSPENDED *and* we've successfully freed the worker.
	return state.Actor.GetStatus() == runtimeapipb.Actor_STATUS_SUSPENDED && state.Actor.GetSandboxPodNamespace() == "", nil
}
func (s *FinalizeSuspendedStep) Execute(ctx context.Context, input *SuspendInput, state *SuspendState) error {
	latestActor, err := s.store.GetActor(ctx, input.ActorID)
	if err != nil {
		return err
	}

	// 1. Free the worker (if it hasn't been freed yet)
	if latestActor.GetSandboxPodNamespace() != "" {
		workerNs := latestActor.GetSandboxPodNamespace()
		workerPod := latestActor.GetSandboxPodName()

		workerPool := latestActor.GetWorkerPoolName()

		worker, err := s.store.GetWorker(ctx, workerNs, workerPool, workerPod)
		if err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("while getting worker for release: %w", err)
			}
			slog.Warn("Worker already gone during finalize suspend, skipping release", "worker", workerPod)
		} else {
			// Only free it if it still belongs to us
			if worker.GetActorId() == input.ActorID {
				worker.ActorNamespace = ""
				worker.ActorTemplate = ""
				worker.ActorId = ""

				err = s.store.UpdateWorker(ctx, worker, worker.Version)
				if err != nil {
					return err
				}
			}
		}

		// 2. Safely clear ActiveWorker now that the worker object in DB is freed
		latestActor, err = s.store.GetActor(ctx, input.ActorID)
		if err != nil {
			return err
		}
		latestActor.Status = runtimeapipb.Actor_STATUS_SUSPENDED
		if latestActor.InProgressSnapshot != "" {
			latestActor.LatestSnapshotInfo = &runtimeapipb.SnapshotInfo{
				Type: runtimeapipb.SnapshotType_SNAPSHOT_TYPE_EXTERNAL,
				Data: &runtimeapipb.SnapshotInfo_External{
					External: &runtimeapipb.ExternalSnapshotInfo{
						SnapshotUriPrefix: latestActor.InProgressSnapshot,
					},
				},
			}
			latestActor.InProgressSnapshot = ""
		}
		latestActor.SandboxPodNamespace = ""
		latestActor.SandboxPodName = ""
		latestActor.SandboxPodIp = ""
		latestActor.WorkerPoolName = ""
		err = s.store.UpdateActor(ctx, latestActor, latestActor.GetVersion())
		if err != nil {
			return err
		}
	}

	state.Actor = latestActor
	return nil
}

func (s *FinalizeSuspendedStep) RetryBackoff() *wait.Backoff { return nil }
