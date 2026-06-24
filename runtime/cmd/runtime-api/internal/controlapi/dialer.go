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
	"errors"
	"fmt"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/lru"
)

var ErrWorkerPodNotFound = errors.New("worker pod not found")

// WorkerDialer handles gRPC connections to Worker pods.
type WorkerDialer struct {
	workerIndexer cache.Indexer
	workerPodSidecarIndexer cache.Indexer
	workerConns   *lru.Cache
}

// NewWorkerDialer creates a new WorkerDialer.
func NewWorkerDialer(workerIndexer cache.Indexer, workerPodSidecarIndexer cache.Indexer) *WorkerDialer {
	return &WorkerDialer{
		workerIndexer: workerIndexer,
		workerPodSidecarIndexer: workerPodSidecarIndexer,
		workerConns:   lru.New(1024),
	}
}

// DialForWorker returns a gRPC connection to the Worker running on the same node as the specified worker pod.
// Returns ErrWorkerPodNotFound if the worker pod is not found in the informer cache.
func (d *WorkerDialer) DialForWorker(workerPodNamespace, workerPodName string) (*grpc.ClientConn, error) {
	workerPodKey := workerPodNamespace + "/" + workerPodName
	matchingPods, err := d.workerIndexer.ByIndex(byNamespaceAndName, workerPodKey)
	if err != nil {
		return nil, fmt.Errorf("while finding pod %q: %w", workerPodKey, err)
	}

	if len(matchingPods) == 0 {
		return nil, ErrWorkerPodNotFound
	}

	if len(matchingPods) > 1 {
		return nil, fmt.Errorf("expected 1 pod match, got %d", len(matchingPods))
	}

	selectedWorker := matchingPods[0].(*corev1.Pod)

	matchingWorkerPods, err := d.workerPodSidecarIndexer.ByIndex(byNode, selectedWorker.Spec.NodeName)
	if err != nil {
		return nil, fmt.Errorf("while finding runtime-worker for worker pod %q on node %q: %w", workerPodKey, selectedWorker.Spec.NodeName, err)
	}

	if len(matchingWorkerPods) != 1 {
		return nil, fmt.Errorf("found %d runtime-worker pods on node %q, expected 1", len(matchingWorkerPods), selectedWorker.Spec.NodeName)
	}

	selectedWorkerPod := matchingWorkerPods[0].(*corev1.Pod)
	runtimeWorkerPodKey := selectedWorkerPod.ObjectMeta.Namespace + "/" + selectedWorkerPod.ObjectMeta.Name

	workerConnAny, ok := d.workerConns.Get(runtimeWorkerPodKey)
	if ok {
		return workerConnAny.(*grpc.ClientConn), nil
	}

	if len(selectedWorkerPod.Status.PodIPs) == 0 {
		return nil, fmt.Errorf("selected runtime-worker %q has no assigned IPs: %w", selectedWorkerPod.ObjectMeta.Namespace+"/"+selectedWorkerPod.ObjectMeta.Name, err)
	}

	workerConn, err := grpc.NewClient(
		selectedWorkerPod.Status.PodIPs[0].IP+":8085",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, fmt.Errorf("while creating runtime-worker gRPC client connection: %w", err)
	}

	d.workerConns.Add(runtimeWorkerPodKey, workerConn)

	return workerConn, nil
}
