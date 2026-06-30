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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	systemNamespace    = "actordock-system"
	byNamespaceAndName = "by-namespace-and-name"
	byWorkerPool       = "by-worker-pool"
	byNode             = "by-node"
	workerPodLabel     = "actordock.dev/worker-pool"
)

// WorkerDaemonInformer creates a SharedInformerFactory and SharedIndexInformer for runtime-worker pods.
func WorkerDaemonInformer(kc kubernetes.Interface) (informers.SharedInformerFactory, cache.SharedIndexInformer) {
	factory := informers.NewSharedInformerFactoryWithOptions(kc, 0,
		informers.WithNamespace(systemNamespace),
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.LabelSelector = `app in (runtime-worker)`
		}),
	)
	workerDaemonInformer := factory.Core().V1().Pods().Informer()
	workerDaemonInformer.AddIndexers(cache.Indexers{
		byNode: func(obj any) ([]string, error) {
			pod := obj.(*corev1.Pod)
			return []string{pod.Spec.NodeName}, nil
		},
	})
	return factory, workerDaemonInformer
}

// WorkerPodInformer creates a SharedInformerFactory and SharedIndexInformer for Worker pods.
func WorkerPodInformer(kc kubernetes.Interface) (informers.SharedInformerFactory, cache.SharedIndexInformer) {
	factory := informers.NewSharedInformerFactoryWithOptions(kc, 5*time.Minute,
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.LabelSelector = "actordock.dev/worker-pool"
		}),
	)
	workerPodInformer := factory.Core().V1().Pods().Informer()
	workerPodInformer.AddIndexers(cache.Indexers{
		byNamespaceAndName: func(obj any) ([]string, error) {
			pod := obj.(*corev1.Pod)
			key := pod.ObjectMeta.Namespace + "/" + pod.ObjectMeta.Name
			return []string{key}, nil
		},
		byWorkerPool: func(obj any) ([]string, error) {
			pod := obj.(*corev1.Pod)
			workerPoolRef := pod.ObjectMeta.Namespace + "/" + pod.ObjectMeta.Labels["actordock.dev/worker-pool"]
			return []string{workerPoolRef}, nil
		},
	})

	return factory, workerPodInformer
}
