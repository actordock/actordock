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
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/actordock/runtime/cmd/runtime-api/internal/store/runtimeredis"
	"github.com/actordock/runtime/internal/runtimeinterceptors"
	"github.com/actordock/runtime/internal/envtestbins"
	"github.com/actordock/runtime/internal/proto/runtimeworkerpb"
	runtimev1alpha1 "github.com/actordock/runtime/pkg/api/v1alpha1"
	"github.com/actordock/runtime/pkg/client/clientset/versioned"
	"github.com/actordock/runtime/pkg/client/informers/externalversions"
	listersv1alpha1 "github.com/actordock/runtime/pkg/client/listers/api/v1alpha1"
	"github.com/actordock/runtime/pkg/proto/runtimeapipb"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	testEnv    *envtest.Environment
	cfg        *rest.Config
	fakeWorker = &FakeWorkerServer{}
)

func TestMain(m *testing.M) {
	binaryAssetsDirectory, err := envtestbins.BinaryAssetsDir()
	if err != nil {
		log.Fatalf("%v", err)
	}

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{"../../../../manifests/runtime-install/generated"},
		BinaryAssetsDirectory: binaryAssetsDirectory,
	}

	cfg, err = testEnv.Start()
	if err != nil {
		log.Fatalf("testEnv.Start: %v", err)
	}

	// Create actordock-system namespace
	k8sClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("kubernetes.NewForConfig: %v", err)
	}
	_, err = k8sClient.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "actordock-system"},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		log.Fatalf("create actordock-system namespace: %v", err)
	}

	// Create shared Worker Pod
	workerSidecarPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "runtime-worker-shared",
			Namespace: "actordock-system",
			Labels: map[string]string{
				"app": "runtime-worker",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "node1",
			Containers: []corev1.Container{
				{Name: "main", Image: "nginx"},
			},
		},
	}
	createdWorkerPod, err := k8sClient.CoreV1().Pods("actordock-system").Create(context.Background(), workerSidecarPod, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		log.Fatalf("create runtime-worker pod: %v", err)
	}
	if err == nil {
		createdWorkerPod.Status.PodIPs = []corev1.PodIP{{IP: "127.0.0.1"}}
		createdWorkerPod.Status.Phase = corev1.PodRunning
		_, err = k8sClient.CoreV1().Pods("actordock-system").UpdateStatus(context.Background(), createdWorkerPod, metav1.UpdateOptions{})
		if err != nil {
			log.Fatalf("update runtime-worker pod status: %v", err)
		}
	}

	// Start Fake Worker Server on port 8085
	workerGrpcServer := grpc.NewServer()
	runtimeworkerpb.RegisterWorkerHerderServer(workerGrpcServer, fakeWorker)
	workerLis, err := net.Listen("tcp", "127.0.0.1:8085")
	if err != nil {
		log.Fatalf("listen on 127.0.0.1:8085: %v", err)
	}
	go func() {
		if err := workerGrpcServer.Serve(workerLis); err != nil {
			fmt.Printf("runtime-worker grpc server exited: %v\n", err)
		}
	}()

	code := m.Run()

	workerGrpcServer.Stop()

	err = testEnv.Stop()
	if err != nil {
		log.Fatalf("testEnv.Stop: %v", err)
	}

	os.Exit(code)
}

// FakeWorkerServer implements runtimeworkerpb.WorkersServer
type FakeWorkerServer struct {
	runtimeworkerpb.UnimplementedWorkerHerderServer

	Lock sync.Mutex

	RunCalled  bool
	RunRequest *runtimeworkerpb.RunRequest
	FailRun    error

	CheckpointCalled  bool
	CheckpointRequest *runtimeworkerpb.CheckpointRequest

	RestoreCalled  bool
	RestoreRequest *runtimeworkerpb.RestoreRequest
	FailRestore    error
	RestoreDelay   time.Duration
}

func (f *FakeWorkerServer) Reset() {
	f.Lock.Lock()
	defer f.Lock.Unlock()

	f.RunCalled = false
	f.RunRequest = nil
	f.FailRun = nil

	f.CheckpointCalled = false
	f.CheckpointRequest = nil

	f.RestoreCalled = false
	f.RestoreRequest = nil
	f.FailRestore = nil
	f.RestoreDelay = 0
}

func (f *FakeWorkerServer) Run(ctx context.Context, req *runtimeworkerpb.RunRequest) (*runtimeworkerpb.RunResponse, error) {
	f.Lock.Lock()
	defer f.Lock.Unlock()

	f.RunCalled = true
	f.RunRequest = proto.Clone(req).(*runtimeworkerpb.RunRequest)
	if f.FailRun != nil {
		return nil, f.FailRun
	}

	return &runtimeworkerpb.RunResponse{}, nil
}

func (f *FakeWorkerServer) Checkpoint(ctx context.Context, req *runtimeworkerpb.CheckpointRequest) (*runtimeworkerpb.CheckpointResponse, error) {
	f.Lock.Lock()
	defer f.Lock.Unlock()

	f.CheckpointCalled = true
	f.CheckpointRequest = proto.Clone(req).(*runtimeworkerpb.CheckpointRequest)

	return &runtimeworkerpb.CheckpointResponse{}, nil
}

func (f *FakeWorkerServer) Restore(ctx context.Context, req *runtimeworkerpb.RestoreRequest) (*runtimeworkerpb.RestoreResponse, error) {
	f.Lock.Lock()
	defer f.Lock.Unlock()

	f.RestoreCalled = true
	f.RestoreRequest = proto.Clone(req).(*runtimeworkerpb.RestoreRequest)
	if f.RestoreDelay > 0 {
		time.Sleep(f.RestoreDelay)
	}
	if f.FailRestore != nil {
		return nil, f.FailRestore
	}
	return &runtimeworkerpb.RestoreResponse{}, nil
}

func (f *FakeWorkerServer) lastRestoreRequest() *runtimeworkerpb.RestoreRequest {
	f.Lock.Lock()
	defer f.Lock.Unlock()

	if f.RestoreRequest == nil {
		return nil
	}
	return proto.Clone(f.RestoreRequest).(*runtimeworkerpb.RestoreRequest)
}

type testContext struct {
	mr                  *miniredis.Miniredis
	service             *Service
	client              runtimeapipb.ControlClient
	k8sClient           kubernetes.Interface
	runtimeAPIClient     versioned.Interface
	persistence         *runtimeredis.Persistence
	fakeWorker          *FakeWorkerServer
	cleanup             func()
	actorTemplateLister listersv1alpha1.ActorTemplateLister
	workerPoolLister    listersv1alpha1.WorkerPoolLister
	sandboxConfigLister listersv1alpha1.SandboxConfigLister
}

// setupTest sets up a fully isolated test environment.
func setupTest(t *testing.T, ns string) *testContext {
	t.Helper()
	// 1. Start Miniredis
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	rdb := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: []string{mr.Addr()},
	})
	persistence := runtimeredis.NewPersistence(rdb)

	// 2. Initialize Clientsets using global cfg
	k8sClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		mr.Close()
		t.Fatalf("failed to create k8s clientset: %v", err)
	}

	runtimeAPIClient, err := versioned.NewForConfig(cfg)
	if err != nil {
		mr.Close()
		t.Fatalf("failed to create runtime clientset: %v", err)
	}

	// 3. Initialize Informers
	workerFactory, workerInformer := WorkerPodInformer(k8sClient)
	workerPodSidecarFactory, workerPodSidecarInformer := RuntimeWorkerPodInformer(k8sClient)

	runtimeInformerFactory := externalversions.NewSharedInformerFactory(runtimeAPIClient, 0)
	actorTemplateLister := runtimeInformerFactory.Api().V1alpha1().ActorTemplates().Lister()
	workerPoolLister := runtimeInformerFactory.Api().V1alpha1().WorkerPools().Lister()
	sandboxConfigLister := runtimeInformerFactory.Api().V1alpha1().SandboxConfigs().Lister()

	ctx, cancel := context.WithCancel(context.Background())

	syncer := NewWorkerPoolSyncer(persistence, workerInformer)
	syncer.Start(ctx)

	workerFactory.Start(ctx.Done())
	workerPodSidecarFactory.Start(ctx.Done())
	runtimeInformerFactory.Start(ctx.Done())

	workerFactory.WaitForCacheSync(ctx.Done())
	workerPodSidecarFactory.WaitForCacheSync(ctx.Done())
	runtimeInformerFactory.WaitForCacheSync(ctx.Done())

	// 4. Initialize Service
	dialer := NewWorkerDialer(workerInformer.GetIndexer(), workerPodSidecarInformer.GetIndexer())
	service := NewService(persistence, actorTemplateLister, workerPoolLister, sandboxConfigLister, dialer, k8sClient)

	// 5. Start REAL gRPC Server for runtime API
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(runtimeinterceptors.ServerUnaryInterceptor))
	runtimeapipb.RegisterControlServer(grpcServer, service)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		cancel()
		mr.Close()
		t.Fatalf("failed to listen: %v", err)
	}

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			t.Logf("grpc server exited: %v", err)
		}
	}()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		grpcServer.Stop()
		cancel()
		mr.Close()
		t.Fatalf("failed to connect: %v", err)
	}

	client := runtimeapipb.NewControlClient(conn)

	// Call Reset on global mock
	fakeWorker.Reset()

	// Create namespace
	_, err = k8sClient.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{})
	if err != nil {
		conn.Close()
		grpcServer.Stop()
		cancel()
		mr.Close()
		t.Fatalf("failed to create namespace %s: %v", ns, err)
	}

	cleanup := func() {
		conn.Close()
		grpcServer.Stop()
		cancel()
		mr.Close()
	}

	return &testContext{
		mr:                  mr,
		service:             service,
		client:              client,
		k8sClient:           k8sClient,
		runtimeAPIClient:     runtimeAPIClient,
		persistence:         persistence,
		fakeWorker:          fakeWorker,
		cleanup:             cleanup,
		actorTemplateLister: actorTemplateLister,
		workerPoolLister:    workerPoolLister,
		sandboxConfigLister: sandboxConfigLister,
	}
}

func namespaceForTest(baseName string) string {
	return fmt.Sprintf("%s-%d", baseName, time.Now().UnixNano())
}

func selectorLabelsOfSize(n int) map[string]string {
	labels := make(map[string]string, n)
	for i := 0; i < n; i++ {
		labels[fmt.Sprintf("k%d", i)] = "v"
	}
	return labels
}

func createTemplate(t *testing.T, tc *testContext, ns string) {
	t.Helper()
	createTemplateWithContainers(t, tc, ns, []runtimev1alpha1.Container{
		{
			Name:    "main",
			Image:   "main@sha256:abc",
			Command: []string{"/main"},
		},
	})
}

const poolLabelKey = "pool"

func createTemplateWithContainers(t *testing.T, tc *testContext, ns string, containers []runtimev1alpha1.Container) {
	t.Helper()

	// Sandbox binaries now live on a (cluster-scoped) SandboxConfig resolved via
	// the actor's WorkerPool, not on the ActorTemplate. Create a default gvisor
	// SandboxConfig so a boot-from-spec Run can resolve its assets.
	ensureDefaultGvisorSandboxConfig(t, tc)
	createWorkerPool(t, tc, ns, "pool1", map[string]string{poolLabelKey: ns})

	actorTemplate := &runtimev1alpha1.ActorTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tmpl1",
			Namespace: ns,
		},
		Spec: runtimev1alpha1.ActorTemplateSpec{
			PauseImage: "pause@sha256:abc",
			SnapshotsConfig: runtimev1alpha1.SnapshotsConfig{
				Location: "gs://fake-fake-fake",
			},
			Containers: containers,
			WorkerSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{poolLabelKey: ns},
			},
		},
	}
	createdTemplate, err := tc.runtimeAPIClient.ApiV1alpha1().ActorTemplates(ns).Create(context.Background(), actorTemplate, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create actor template: %v", err)
	}

	createdTemplate.Status = runtimev1alpha1.ActorTemplateStatus{
		GoldenSnapshot: "gs://my-bucket/my-folder",
	}

	_, err = tc.runtimeAPIClient.ApiV1alpha1().ActorTemplates(ns).UpdateStatus(context.Background(), createdTemplate, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	// Wait for Informer cache to sync
	err = wait.PollUntilContextTimeout(context.Background(), 100*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		tmpl, err := tc.actorTemplateLister.ActorTemplates(ns).Get("tmpl1")
		if err != nil {
			return false, nil // Retry if not found in cache yet
		}
		return tmpl.Status.GoldenSnapshot != "", nil
	})
	if err != nil {
		t.Fatalf("failed to wait for template status update in informer: %v", err)
	}
}

// ensureDefaultGvisorSandboxConfig creates the cluster-scoped default gvisor
// SandboxConfig (idempotently) and waits for it to appear in the lister.
func ensureDefaultGvisorSandboxConfig(t *testing.T, tc *testContext) {
	t.Helper()
	const name = "gvisor-default"
	sc := &runtimev1alpha1.SandboxConfig{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: runtimev1alpha1.SandboxConfigSpec{
			SandboxClass: runtimev1alpha1.SandboxClassGvisor,
			Default:      true,
			Assets: map[string]map[string]runtimev1alpha1.AssetFile{
				"amd64": {"runsc": {
					URL:    "gs://gvisor/releases/nightly/2026-05-19/x86_64/runsc",
					SHA256: "a397be1abc2420d26bce6c70e6e2ff96c73aaaab929756c56f5e2089ea842b63",
				}},
				"arm64": {"runsc": {
					URL:    "gs://gvisor/releases/nightly/2026-05-19/aarch64/runsc",
					SHA256: "1ba2366ae2efceba166046f51a4104f9261c9cb72c6db8f5b3fe2dc57dea86b9",
				}},
			},
		},
	}
	if _, err := tc.runtimeAPIClient.ApiV1alpha1().SandboxConfigs().Create(context.Background(), sc, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("failed to create default SandboxConfig: %v", err)
	}
	if err := wait.PollUntilContextTimeout(context.Background(), 100*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		_, err := tc.sandboxConfigLister.Get(name)
		return err == nil, nil
	}); err != nil {
		t.Fatalf("default SandboxConfig not synced into lister: %v", err)
	}
}

func createWorkerPool(t *testing.T, tc *testContext, ns string, name string, labels map[string]string) {
	t.Helper()
	wp := &runtimev1alpha1.WorkerPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: runtimev1alpha1.WorkerPoolSpec{
			Replicas:   1,
			SandboxImage: "runtime-sandbox@sha256:abc",
		},
	}
	_, err := tc.runtimeAPIClient.ApiV1alpha1().WorkerPools(ns).Create(context.Background(), wp, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create WorkerPool: %v", err)
	}

	err = wait.PollUntilContextTimeout(context.Background(), 100*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		_, err := tc.workerPoolLister.WorkerPools(ns).Get(name)
		return err == nil, nil
	})
	if err != nil {
		t.Fatalf("failed to wait for WorkerPool %s/%s in informer: %v", ns, name, err)
	}
}

func createTemplateWithSelector(t *testing.T, tc *testContext, ns string, name string, selector *metav1.LabelSelector) {
	t.Helper()
	ensureDefaultGvisorSandboxConfig(t, tc)
	actorTemplate := &runtimev1alpha1.ActorTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: runtimev1alpha1.ActorTemplateSpec{
			PauseImage: "pause@sha256:abc",
			SnapshotsConfig: runtimev1alpha1.SnapshotsConfig{
				Location: "gs://fake-fake-fake",
			},
			Containers: []runtimev1alpha1.Container{
				{Name: "main", Image: "main@sha256:abc", Command: []string{"/main"}},
			},
			WorkerSelector: selector,
		},
	}
	_, err := tc.runtimeAPIClient.ApiV1alpha1().ActorTemplates(ns).Create(context.Background(), actorTemplate, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create actor template: %v", err)
	}

	err = wait.PollUntilContextTimeout(context.Background(), 100*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		_, err := tc.actorTemplateLister.ActorTemplates(ns).Get(name)
		return err == nil, nil
	})
	if err != nil {
		t.Fatalf("failed to wait for template %s/%s in informer: %v", ns, name, err)
	}
}

func createWorkerPod(t *testing.T, tc *testContext, ns string, name string, nodeName string, poolName string) {
	t.Helper()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				"actordock.dev/worker-pool": poolName,
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{
				{Name: "main", Image: "nginx"},
			},
		},
	}
	createdPod, err := tc.k8sClient.CoreV1().Pods(ns).Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create worker pod: %v", err)
	}
	createdPod.Status.PodIPs = []corev1.PodIP{{IP: "127.0.0.1"}}
	createdPod.Status.Phase = corev1.PodRunning
	_, err = tc.k8sClient.CoreV1().Pods(ns).UpdateStatus(context.Background(), createdPod, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update worker pod status: %v", err)
	}

	// Wait for worker to be registered via API
	err = wait.PollUntilContextTimeout(context.Background(), 100*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		resp, err := tc.client.ListWorkers(ctx, &runtimeapipb.ListWorkersRequest{})
		if err != nil {
			return false, nil // Retry on API error
		}
		for _, w := range resp.GetWorkers() {
			if w.GetWorkerNamespace() == ns && w.GetWorkerPod() == name {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("failed to wait for worker to be registered: %v", err)
	}
}

func deleteWorkerPod(t *testing.T, tc *testContext, ns string, name string) {
	t.Helper()
	err := tc.k8sClient.CoreV1().Pods(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete worker pod %s: %v", name, err)
	}

	// Wait for worker to be removed from API
	err = wait.PollUntilContextTimeout(context.Background(), 100*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		resp, err := tc.client.ListWorkers(ctx, &runtimeapipb.ListWorkersRequest{})
		if err != nil {
			return false, nil // Retry on API error
		}
		for _, w := range resp.GetWorkers() {
			if w.GetWorkerNamespace() == ns && w.GetWorkerPod() == name {
				return false, nil // Still there
			}
		}
		return true, nil // Gone!
	})
	if err != nil {
		t.Fatalf("failed to wait for worker to be removed: %v", err)
	}
}

// TestCreateActor_Success tests the happy path for creating an actor.
// Workflow:
// 1. Creates a mock ActorTemplate in the test namespace.
// 2. Calls CreateActor RPC.
// 3. Verifies that the actor is successfully created and returned in the response with a generated ID.
func TestCreateActor_Success(t *testing.T) {
	ns := namespaceForTest("ns-create-success")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createTemplate(t, tc, ns)

	createResp, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
		WorkerSelector:         &runtimeapipb.Selector{MatchLabels: map[string]string{"tier": "free"}},
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}

	want := &runtimeapipb.CreateActorResponse{
		Actor: &runtimeapipb.Actor{
			ActorId:                "id1",
			Version:                1,
			ActorTemplateNamespace: ns,
			ActorTemplateName:      "tmpl1",
			Status:                 runtimeapipb.Actor_STATUS_SUSPENDED,
			WorkerSelector:         &runtimeapipb.Selector{MatchLabels: map[string]string{"tier": "free"}},
		},
	}

	if diff := cmp.Diff(want, createResp, protocmp.Transform()); diff != "" {
		t.Errorf("CreateActor response mismatch (-want +got):\n%s", diff)
	}
}

// TestCreateActor_TemplateNotFound tests that creating an actor with a non-existent template fails with FailedPrecondition.
func TestCreateActor_TemplateNotFound(t *testing.T) {
	ns := namespaceForTest("ns-create-notfound")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	_, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "non-existent",
		ActorId:                "id1",
	})
	assertGrpcError(t, err, codes.FailedPrecondition, fmt.Sprintf("ActorTemplate %s/non-existent not found", ns))
}

// TestCreateActor_Duplicate tests that creating an actor with an existing ID fails.
func TestCreateActor_Duplicate(t *testing.T) {
	ns := namespaceForTest("ns-create-dup")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createTemplate(t, tc, ns)

	_, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
	})
	if err != nil {
		t.Fatalf("first CreateActor failed: %v", err)
	}

	_, err = tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
	})
	assertGrpcError(t, err, codes.AlreadyExists, "Actor id1 already exists")
}

// TestGetActor_Found tests that an existing actor can be retrieved.
func TestGetActor_Found(t *testing.T) {
	ns := namespaceForTest("ns-get-found")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createTemplate(t, tc, ns)

	createResp, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}

	id := createResp.GetActor().GetActorId()

	getResp, err := tc.client.GetActor(context.Background(), &runtimeapipb.GetActorRequest{
		ActorId: id,
	})
	if err != nil {
		t.Fatalf("GetActor failed: %v", err)
	}

	want := &runtimeapipb.GetActorResponse{
		Actor: createResp.GetActor(),
	}

	if diff := cmp.Diff(want, getResp, protocmp.Transform()); diff != "" {
		t.Errorf("GetActor response mismatch (-want +got):\n%s", diff)
	}
}

// TestGetActor_NotFound tests that retrieving a non-existent actor fails.
// Workflow:
// 1. Calls GetActor RPC with a non-existent ID.
// 2. Verifies that it returns an error (NotFound).
func TestGetActor_NotFound(t *testing.T) {
	ns := namespaceForTest("ns-get-notfound")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	_, err := tc.client.GetActor(context.Background(), &runtimeapipb.GetActorRequest{
		ActorId: "non-existent",
	})
	assertGrpcError(t, err, codes.NotFound, "Actor non-existent not found")
}

// TestListActors tests that all created actors can be listed.
// Workflow:
// 1. Creates a mock ActorTemplate.
// 2. Calls CreateActor twice to create two actors.
// 3. Calls ListActors RPC.
// 4. Verifies that both actors are returned in the list.
func TestListActors(t *testing.T) {
	ns := namespaceForTest("ns-list-actors")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createTemplate(t, tc, ns)

	resp1, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
	})
	if err != nil {
		t.Fatalf("CreateActor 1 failed: %v", err)
	}
	resp2, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id2",
	})
	if err != nil {
		t.Fatalf("CreateActor 2 failed: %v", err)
	}

	listResp, err := tc.client.ListActors(context.Background(), &runtimeapipb.ListActorsRequest{})
	if err != nil {
		t.Fatalf("ListActors failed: %v", err)
	}

	if len(listResp.Actors) != 2 {
		t.Fatalf("expected 2 actors, got %d", len(listResp.Actors))
	}

	want := []*runtimeapipb.Actor{
		resp1.GetActor(),
		resp2.GetActor(),
	}

	opts := []cmp.Option{
		protocmp.Transform(),
		cmpopts.SortSlices(func(a, b *runtimeapipb.Actor) bool {
			return a.ActorId < b.ActorId
		}),
	}

	if diff := cmp.Diff(want, listResp.Actors, opts...); diff != "" {
		t.Errorf("ListActors response mismatch (-want +got):\n%s", diff)
	}
}

// TestListActors_Pagination tests that ListActors correctly paginates results.
func TestListActors_Pagination(t *testing.T) {
	ns := namespaceForTest("ns-list-actors-pagination")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createTemplate(t, tc, ns)

	var want []*runtimeapipb.Actor
	for i := 0; i < 5; i++ {
		resp, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
			ActorTemplateNamespace: ns,
			ActorTemplateName:      "tmpl1",
			ActorId:                fmt.Sprintf("id%d", i),
		})
		if err != nil {
			t.Fatalf("CreateActor %d failed: %v", i, err)
		}
		want = append(want, resp.GetActor())
	}

	var allActors []*runtimeapipb.Actor
	pageToken := ""

	for {
		listResp, err := tc.client.ListActors(context.Background(), &runtimeapipb.ListActorsRequest{
			PageSize:  2,
			PageToken: pageToken,
		})
		if err != nil {
			t.Fatalf("ListActors failed: %v", err)
		}

		allActors = append(allActors, listResp.Actors...)
		pageToken = listResp.GetNextPageToken()
		if pageToken == "" {
			break
		}
	}

	if len(allActors) != 5 {
		t.Fatalf("expected 5 actors total, got %d", len(allActors))
	}

	opts := []cmp.Option{
		protocmp.Transform(),
		cmpopts.SortSlices(func(a, b *runtimeapipb.Actor) bool {
			return a.ActorId < b.ActorId
		}),
	}

	if diff := cmp.Diff(want, allActors, opts...); diff != "" {
		t.Errorf("ListActors pagination response mismatch (-want +got):\n%s", diff)
	}
}

func TestListActors_PageSizeValidation(t *testing.T) {
	ns := namespaceForTest("ns-list-actors-validation")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	// 1. Negative page size
	_, err := tc.client.ListActors(context.Background(), &runtimeapipb.ListActorsRequest{
		PageSize: -1,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument error for negative page_size, got: %v", err)
	}

	// 2. Page size exceeding maxPageSize (1000)
	_, err = tc.client.ListActors(context.Background(), &runtimeapipb.ListActorsRequest{
		PageSize: 1001,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument error for page_size > 1000, got: %v", err)
	}
}

// TestListWorkers tests that workers mirrored to Redis are listed.
// Workflow:
// 1. Creates a mock worker Pod in Kubernetes.
// 2. Waits for the background WorkerPoolSyncer to mirror it to Redis.
// 3. Calls ListWorkers RPC.
// 4. Verifies that the worker appears in the response.
func TestListWorkers(t *testing.T) {
	ns := namespaceForTest("ns-list-workers")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createWorkerPod(t, tc, ns, "worker-1", "", "pool1")

	listResp, err := tc.client.ListWorkers(context.Background(), &runtimeapipb.ListWorkersRequest{})
	if err != nil {
		t.Fatalf("ListWorkers failed: %v", err)
	}

	var filteredWorkers []*runtimeapipb.Worker
	for _, w := range listResp.GetWorkers() {
		if w.GetWorkerNamespace() == ns {
			filteredWorkers = append(filteredWorkers, w)
		}
	}

	want := []*runtimeapipb.Worker{
		{
			WorkerNamespace: ns,
			WorkerPool:      "pool1",
			WorkerPod:       "worker-1",
			Ip:              "127.0.0.1",
			Version:         1,
		},
	}

	if diff := cmp.Diff(want, filteredWorkers, protocmp.Transform(), protocmp.IgnoreFields(&runtimeapipb.Worker{}, "worker_pod_uid")); diff != "" {
		t.Errorf("ListWorkers response mismatch (-want +got):\n%s", diff)
	}
}

// TestResumeActor tests the full workflow of resuming a suspended actor.
// Workflow:
// 1. Creates a mock ActorTemplate.
// 2. Creates a mock Worker Pod in 'actordock-system' namespace on 'node1'.
// 3. Creates a mock worker Pod in the test namespace on 'node1'.
// 4. Waits for the WorkerPoolSyncer to mirror the worker to Redis.
// 5. Creates an actor (starts as SUSPENDED).
// 6. Calls ResumeActor RPC.
// 7. Verifies that the fake Worker received the Restore call.
// 8. Verifies that the actor status is updated to RUNNING.
func TestResumeActor(t *testing.T) {
	ns := namespaceForTest("ns-resume")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createTemplate(t, tc, ns)

	createWorkerPod(t, tc, ns, "worker-1", "node1", "pool1")

	_, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}
	id := "id1"

	_, err = tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{
		ActorId: id,
	})
	if err != nil {
		t.Fatalf("ResumeActor failed: %v", err)
	}

	if !tc.fakeWorker.RestoreCalled {
		t.Errorf("expected Restore to be called")
	}

	getResp, err := tc.client.GetActor(context.Background(), &runtimeapipb.GetActorRequest{
		ActorId: id,
	})
	if err != nil {
		t.Fatalf("GetActor failed: %v", err)
	}
	want := &runtimeapipb.GetActorResponse{
		Actor: &runtimeapipb.Actor{
			ActorId:                id,
			ActorTemplateNamespace: ns,
			ActorTemplateName:      "tmpl1",
			Status:                 runtimeapipb.Actor_STATUS_RUNNING,
			SandboxPodNamespace:      ns,
			SandboxPodName:           "worker-1",
			SandboxPodIp:             "127.0.0.1",
			WorkerPoolName:         "pool1",
		},
	}
	if diff := cmp.Diff(want, getResp, protocmp.Transform(), protocmp.IgnoreFields(&runtimeapipb.Actor{}, "version"), protocmp.IgnoreFields(&runtimeapipb.Actor{}, "sandbox_pod_uid")); diff != "" {
		t.Errorf("GetActor response mismatch (-want +got):\n%s", diff)
	}

	// Verify that the worker record also has the assigned actor details
	listWorkersResp, err := tc.client.ListWorkers(context.Background(), &runtimeapipb.ListWorkersRequest{})
	if err != nil {
		t.Fatalf("ListWorkers failed: %v", err)
	}
	var actorWorker *runtimeapipb.Worker
	for _, w := range listWorkersResp.GetWorkers() {
		if w.GetWorkerNamespace() == ns && w.GetWorkerPod() == "worker-1" {
			actorWorker = w
			break
		}
	}
	if actorWorker == nil {
		t.Fatalf("expected worker-1 in namespace %s not found in ListWorkers", ns)
	}

	wantWorker := &runtimeapipb.Worker{
		WorkerNamespace: ns,
		WorkerPool:      "pool1",
		WorkerPod:       "worker-1",
		ActorNamespace:  ns,
		ActorTemplate:   "tmpl1",
		ActorId:         id,
		Ip:              "127.0.0.1",
		NodeName:        "node1",
	}

	if diff := cmp.Diff(wantWorker, actorWorker, protocmp.Transform(), protocmp.IgnoreFields(&runtimeapipb.Worker{}, "version"), protocmp.IgnoreFields(&runtimeapipb.Worker{}, "worker_pod_uid")); diff != "" {
		t.Errorf("Worker state mismatch (-want +got):\n%s", diff)
	}
}

func TestResumeActorResolvesValueFromEnv(t *testing.T) {
	ns := namespaceForTest("ns-resume-secret-env")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	_, err := tc.k8sClient.CoreV1().Secrets(ns).Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-keys",
			Namespace: ns,
		},
		Data: map[string][]byte{
			"anthropic": []byte("sk-test"),
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create secret: %v", err)
	}

	createTemplateWithContainers(t, tc, ns, []runtimev1alpha1.Container{
		{
			Name:    "main",
			Image:   "main@sha256:abc",
			Command: []string{"/main"},
			Env: []runtimev1alpha1.EnvVar{
				{
					Name:  "LITERAL",
					Value: ptr.To("plain"),
				},
				{
					Name: "ANTHROPIC_API_KEY",
					ValueFrom: &runtimev1alpha1.EnvVarSource{
						SecretKeyRef: &runtimev1alpha1.SecretKeySelector{
							Name: "api-keys",
							Key:  "anthropic",
						},
					},
				},
			},
		},
	})
	createWorkerPod(t, tc, ns, "worker-1", "node1", "pool1")

	_, err = tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}
	_, err = tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{
		ActorId: "id1",
	})
	if err != nil {
		t.Fatalf("ResumeActor failed: %v", err)
	}

	restoreReq := tc.fakeWorker.lastRestoreRequest()
	if restoreReq == nil {
		t.Fatalf("expected Restore to be called")
	}
	if len(restoreReq.GetSpec().GetContainers()) != 1 {
		t.Fatalf("expected one container in restore request, got %d", len(restoreReq.GetSpec().GetContainers()))
	}
	gotEnv := map[string]string{}
	for _, env := range restoreReq.GetSpec().GetContainers()[0].GetEnv() {
		gotEnv[env.GetName()] = env.GetValue()
	}
	wantEnv := map[string]string{
		"LITERAL":           "plain",
		"ANTHROPIC_API_KEY": "sk-test",
	}
	if diff := cmp.Diff(wantEnv, gotEnv); diff != "" {
		t.Errorf("resolved env mismatch (-want +got):\n%s", diff)
	}
}

// TestResumeActor_NoWorkers tests that resuming an actor fails when no free workers are available.
// Workflow:
// 1. Creates a mock ActorTemplate.
// 2. Creates an actor.
// 3. Calls ResumeActor RPC without creating any workers.
// 4. Verifies that ResumeActor fails with FailedPrecondition status.
// TestResumeActor_NoWorkers tests that resuming an actor fails when no free workers are available.
// Workflow:
// 1. Creates a mock ActorTemplate.
// 2. Creates an actor.
// 3. Calls ResumeActor RPC without creating any workers.
// 4. Verifies that ResumeActor fails with FailedPrecondition status.
func TestResumeActor_NoWorkers(t *testing.T) {
	ns := namespaceForTest("ns-resume-no-workers")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createTemplate(t, tc, ns)

	createResp, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}

	id := createResp.GetActor().GetActorId()

	_, err = tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{
		ActorId: id,
	})
	assertGrpcError(t, err, codes.FailedPrecondition, "no free workers available")
}

// TestResumeActor_NoEligiblePool tests the distinct failure mode from
// TestResumeActor_NoWorkers: here no WorkerPool's labels satisfy the
// template's WorkerSelector at all, so there isn't even a pool to look for
// free workers in.
func TestResumeActor_NoEligiblePool(t *testing.T) {
	ns := namespaceForTest("ns-resume-no-eligible-pool")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createTemplateWithSelector(t, tc, ns, "tmpl1", &metav1.LabelSelector{
		MatchLabels: map[string]string{"nonexistent": ns},
	})

	createResp, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}

	_, err = tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{
		ActorId: createResp.GetActor().GetActorId(),
	})
	assertGrpcError(t, err, codes.FailedPrecondition, "no worker pool matches the template and actor selectors")
}

// TestResumeActor_MultiPoolSelector exercises the AND-of-two-selectors path
// end to end: a template's WorkerSelector gates two pools, and the actor's
// worker_selector narrows to just one of them.
func TestResumeActor_MultiPoolSelector(t *testing.T) {
	ns := namespaceForTest("ns-multi-pool")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createWorkerPool(t, tc, ns, "pool-a", map[string]string{"group": ns, "tier": "a"})
	createWorkerPool(t, tc, ns, "pool-b", map[string]string{"group": ns, "tier": "b"})
	createTemplateWithSelector(t, tc, ns, "tmpl1", &metav1.LabelSelector{
		MatchLabels: map[string]string{"group": ns},
	})

	createWorkerPod(t, tc, ns, "worker-a", "node1", "pool-a")
	createWorkerPod(t, tc, ns, "worker-b", "node1", "pool-b")

	_, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
		WorkerSelector: &runtimeapipb.Selector{
			MatchLabels: map[string]string{"tier": "b"},
		},
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}

	_, err = tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{ActorId: "id1"})
	if err != nil {
		t.Fatalf("ResumeActor failed: %v", err)
	}

	getResp, err := tc.client.GetActor(context.Background(), &runtimeapipb.GetActorRequest{ActorId: "id1"})
	if err != nil {
		t.Fatalf("GetActor failed: %v", err)
	}
	if got := getResp.GetActor().GetSandboxPodName(); got != "worker-b" {
		t.Errorf("expected actor to be assigned to worker-b (pool-b, matching narrowed selector), got %q", got)
	}
	if got := getResp.GetActor().GetWorkerPoolName(); got != "pool-b" {
		t.Errorf("expected actor's worker_pool_name to be pool-b, got %q", got)
	}
}

// TestResumeActor_RequiresBothSelectorsToMatch proves eligibility is the AND
// of the template's WorkerSelector and the actor's worker_selector, not
// either one alone: a pool matching only the template selector and a pool
// matching only the actor selector must both be rejected, end to end
// through CreateActor/ResumeActor (not just the eligibleWorkerPools unit
// test), while a pool matching both is the one actually used.
func TestResumeActor_RequiresBothSelectorsToMatch(t *testing.T) {
	ns := namespaceForTest("ns-resume-and-selectors")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createWorkerPool(t, tc, ns, "pool-both", map[string]string{"group": ns, "tier": "b"})
	createWorkerPool(t, tc, ns, "pool-template-only", map[string]string{"group": ns, "tier": "a"})
	createWorkerPool(t, tc, ns, "pool-actor-only", map[string]string{"tier": "b"})
	createTemplateWithSelector(t, tc, ns, "tmpl1", &metav1.LabelSelector{
		MatchLabels: map[string]string{"group": ns},
	})

	createWorkerPod(t, tc, ns, "worker-both", "node1", "pool-both")
	createWorkerPod(t, tc, ns, "worker-template-only", "node1", "pool-template-only")
	createWorkerPod(t, tc, ns, "worker-actor-only", "node1", "pool-actor-only")

	_, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
		WorkerSelector: &runtimeapipb.Selector{
			MatchLabels: map[string]string{"tier": "b"},
		},
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}

	if _, err := tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{ActorId: "id1"}); err != nil {
		t.Fatalf("ResumeActor failed: %v", err)
	}

	getResp, err := tc.client.GetActor(context.Background(), &runtimeapipb.GetActorRequest{ActorId: "id1"})
	if err != nil {
		t.Fatalf("GetActor failed: %v", err)
	}
	if got := getResp.GetActor().GetWorkerPoolName(); got != "pool-both" {
		t.Errorf("expected actor to be assigned to pool-both (the only pool matching both selectors), got worker_pool_name=%q", got)
	}
}

// TestResumeActor_Reentrancy tests the failure recovery and re-entrancy of ResumeActor.
// Workflow:
// 1. Creates a mock ActorTemplate.
// 2. Creates a mock Worker Pod and a mock Worker Pod.
// 3. Waits for the WorkerPoolSyncer to mirror the worker to store.
// 4. Creates an actor in SUSPENDED state.
// 5. Configures fake Worker to FAIL on Restore.
// 6. Calls ResumeActor and verifies it fails, but actor status becomes RESUMING.
// 7. Configures fake Worker to SUCCEED on Restore.
// 8. Calls ResumeActor again and verifies it succeeds and actor status becomes RUNNING.
func TestResumeActor_Reentrancy(t *testing.T) {
	ns := namespaceForTest("ns-resume-reentrancy")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createTemplate(t, tc, ns)

	// Create Worker Pod
	createWorkerPod(t, tc, ns, "worker-1", "node1", "pool1")

	_, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}
	id := "id1"

	// STEP 1: Make Worker FAIL on Restore!
	tc.fakeWorker.FailRestore = fmt.Errorf("mock runtime-worker failure")

	_, err = tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{
		ActorId: id,
	})
	if err == nil {
		t.Fatalf("expected ResumeActor to fail due to runtime-worker error")
	}

	// Verify actor state is RESUMING in Redis!
	actor, err := tc.persistence.GetActor(context.Background(), id)
	if err != nil {
		t.Fatalf("failed to get actor from store: %v", err)
	}
	if actor.GetStatus() != runtimeapipb.Actor_STATUS_RESUMING {
		t.Errorf("expected status RESUMING, got %v", actor.GetStatus())
	}

	// STEP 2: Make Worker SUCCEED!
	tc.fakeWorker.FailRestore = nil
	tc.fakeWorker.RestoreCalled = false // reset for verification

	_, err = tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{
		ActorId: id,
	})
	if err != nil {
		t.Fatalf("ResumeActor failed on retry: %v", err)
	}

	if !tc.fakeWorker.RestoreCalled {
		t.Errorf("expected Restore to be called on retry")
	}

	// Verify actor state is RUNNING!
	actor, err = tc.persistence.GetActor(context.Background(), id)
	if err != nil {
		t.Fatalf("failed to get actor from store: %v", err)
	}
	if actor.GetStatus() != runtimeapipb.Actor_STATUS_RUNNING {
		t.Errorf("expected status RUNNING, got %v", actor.GetStatus())
	}
}

// TestSuspendActor tests the full workflow of suspending a running actor.
// Workflow:
// 1. Creates a mock ActorTemplate.
// 2. Creates a mock Worker Pod on 'node1'.
// 3. Creates a mock worker Pod on 'node1'.
// 4. Waits for the WorkerPoolSyncer to mirror the worker to Redis.
// 5. Creates an actor.
// 6. Calls ResumeActor to transition it to RUNNING.
// 7. Calls SuspendActor RPC.
// 8. Verifies that the fake Worker received the Suspend call.
func TestSuspendActor(t *testing.T) {
	ns := namespaceForTest("ns-suspend")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createTemplate(t, tc, ns)

	createWorkerPod(t, tc, ns, "worker-1", "node1", "pool1")

	_, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}
	id := "id1"

	// Resume first to make it running
	_, err = tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{
		ActorId: id,
	})
	if err != nil {
		t.Fatalf("ResumeActor failed: %v", err)
	}

	// Suspend
	_, err = tc.client.SuspendActor(context.Background(), &runtimeapipb.SuspendActorRequest{
		ActorId: id,
	})
	if err != nil {
		t.Fatalf("SuspendActor failed: %v", err)
	}

	if !tc.fakeWorker.CheckpointCalled {
		t.Errorf("expected runtime-worker Checkpoint to be called")
	}

	getResp, err := tc.client.GetActor(context.Background(), &runtimeapipb.GetActorRequest{
		ActorId: id,
	})
	if err != nil {
		t.Fatalf("GetActor failed: %v", err)
	}
	want := &runtimeapipb.GetActorResponse{
		Actor: &runtimeapipb.Actor{
			ActorId:                id,
			ActorTemplateNamespace: ns,
			ActorTemplateName:      "tmpl1",
			Status:                 runtimeapipb.Actor_STATUS_SUSPENDED,
			LatestSnapshotInfo: &runtimeapipb.SnapshotInfo{
				Type: runtimeapipb.SnapshotType_SNAPSHOT_TYPE_EXTERNAL,
				Data: &runtimeapipb.SnapshotInfo_External{
					External: &runtimeapipb.ExternalSnapshotInfo{
						SnapshotUriPrefix: fmt.Sprintf("gs://fake-fake-fake/%s/", id),
					},
				},
			},
		},
	}

	if diff := cmp.Diff(want, getResp,
		protocmp.Transform(),
		protocmp.IgnoreFields(&runtimeapipb.Actor{}, "version"),
		protocmp.IgnoreFields(&runtimeapipb.Actor{}, "sandbox_pod_uid"),
		protocmp.FilterField(&runtimeapipb.ExternalSnapshotInfo{}, "snapshot_uri_prefix", cmp.Comparer(func(x, y string) bool {
			return strings.HasPrefix(y, x)
		})),
	); diff != "" {
		t.Errorf("GetActor response mismatch (-want +got):\n%s", diff)
	}
}

// TestPauseActor tests the full workflow of pausing a running actor.
// Workflow:
// 1. Creates a mock ActorTemplate.
// 2. Creates a mock Worker Pod on 'node1'.
// 3. Creates a mock worker Pod on 'node1'.
// 4. Waits for the WorkerPoolSyncer to mirror the worker to Redis.
// 5. Creates an actor.
// 6. Calls ResumeActor to transition it to RUNNING.
// 7. Calls PauseActor RPC.
// 8. Verifies that the fake Worker received the Pause call.
func TestPauseActor(t *testing.T) {
	ns := namespaceForTest("ns-pause")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createTemplate(t, tc, ns)

	createWorkerPod(t, tc, ns, "worker-1", "node1", "pool1")

	_, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}
	id := "id1"

	// Resume first to make it running
	_, err = tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{
		ActorId: id,
	})
	if err != nil {
		t.Fatalf("ResumeActor failed: %v", err)
	}

	// Pause
	_, err = tc.client.PauseActor(context.Background(), &runtimeapipb.PauseActorRequest{
		ActorId: id,
	})
	if err != nil {
		t.Fatalf("PauseActor failed: %v", err)
	}

	if !tc.fakeWorker.CheckpointCalled {
		t.Errorf("expected runtime-worker Checkpoint to be called")
	}

	getResp, err := tc.client.GetActor(context.Background(), &runtimeapipb.GetActorRequest{
		ActorId: id,
	})
	if err != nil {
		t.Fatalf("GetActor failed: %v", err)
	}
	want := &runtimeapipb.GetActorResponse{
		Actor: &runtimeapipb.Actor{
			ActorId:                id,
			ActorTemplateNamespace: ns,
			ActorTemplateName:      "tmpl1",
			Status:                 runtimeapipb.Actor_STATUS_PAUSED,
			LatestSnapshotInfo: &runtimeapipb.SnapshotInfo{
				Type: runtimeapipb.SnapshotType_SNAPSHOT_TYPE_LOCAL,
				Data: &runtimeapipb.SnapshotInfo_Local{
					Local: &runtimeapipb.LocalSnapshotInfo{
						SnapshotPrefix:            "id1",
						NodeVmsWithLocalSnapshots: []string{"node1"},
					},
				},
			},
		},
	}

	if diff := cmp.Diff(want, getResp,
		protocmp.Transform(),
		protocmp.IgnoreFields(&runtimeapipb.Actor{}, "version"),
		protocmp.IgnoreFields(&runtimeapipb.Actor{}, "sandbox_pod_uid"),
		protocmp.FilterField(&runtimeapipb.LocalSnapshotInfo{}, "snapshot_prefix", cmp.Comparer(func(x, y string) bool {
			return strings.HasPrefix(y, x)
		})),
	); diff != "" {
		t.Errorf("GetActor response mismatch (-want +got):\n%s", diff)
	}
}

// TestUpdateActor_Success verifies UpdateActor replaces the actor's
// worker_selector and that the change is durably persisted.
func TestUpdateActor_Success(t *testing.T) {
	ns := namespaceForTest("ns-update-actor")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createTemplate(t, tc, ns)

	_, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
		WorkerSelector: &runtimeapipb.Selector{
			MatchLabels: map[string]string{"tier": "free"},
		},
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}

	updateResp, err := tc.client.UpdateActor(context.Background(), &runtimeapipb.UpdateActorRequest{
		ActorId: "id1",
		WorkerSelector: &runtimeapipb.Selector{
			MatchLabels: map[string]string{"tier": "paid"},
		},
	})
	if err != nil {
		t.Fatalf("UpdateActor failed: %v", err)
	}

	wantActor := &runtimeapipb.Actor{
		ActorId:                "id1",
		Version:                2,
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		Status:                 runtimeapipb.Actor_STATUS_SUSPENDED,
		WorkerSelector: &runtimeapipb.Selector{
			MatchLabels: map[string]string{"tier": "paid"},
		},
	}
	wantUpdateResp := &runtimeapipb.UpdateActorResponse{Actor: wantActor}
	if diff := cmp.Diff(wantUpdateResp, updateResp, protocmp.Transform()); diff != "" {
		t.Errorf("UpdateActor response mismatch (-want +got):\n%s", diff)
	}

	getResp, err := tc.client.GetActor(context.Background(), &runtimeapipb.GetActorRequest{ActorId: "id1"})
	if err != nil {
		t.Fatalf("GetActor failed: %v", err)
	}
	wantGetResp := &runtimeapipb.GetActorResponse{Actor: wantActor}
	if diff := cmp.Diff(wantGetResp, getResp, protocmp.Transform()); diff != "" {
		t.Errorf("GetActor response mismatch after UpdateActor (-want +got):\n%s", diff)
	}
}

func TestUpdateActor_NotFound(t *testing.T) {
	ns := namespaceForTest("ns-update-actor-notfound")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	_, err := tc.client.UpdateActor(context.Background(), &runtimeapipb.UpdateActorRequest{ActorId: "does-not-exist"})
	assertGrpcError(t, err, codes.NotFound, "Actor does-not-exist not found")
}

// TestResumeActor_ReleasesStaleWorkerWhenPoolBecomesIneligible verifies that
// a worker claimed by a failed resume attempt is released back to the free
// pool if, by the next resume attempt, the actor's worker_selector has
// changed such that the worker's pool is no longer eligible.
// Workflow:
//  1. Creates pool-a (tier=a) and pool-b (tier=b), and an actor narrowed to
//     tier=a.
//  2. Makes the fake runtime-worker fail Run, then resumes: the actor gets assigned
//     to worker-a (the only eligible pool) and the resume fails after the
//     worker is claimed, leaving worker-a's actor_id set and the actor
//     stuck in RESUMING.
//  3. Updates the actor's selector to tier=b, making pool-a ineligible.
//  4. Resumes again; asserts it succeeds onto worker-b, and that worker-a
//     has been released (actor_id cleared) rather than left dangling.
func TestResumeActor_ReleasesStaleWorkerWhenPoolBecomesIneligible(t *testing.T) {
	ns := namespaceForTest("ns-resume-release-stale")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createWorkerPool(t, tc, ns, "pool-a", map[string]string{"group": ns, "tier": "a"})
	createWorkerPool(t, tc, ns, "pool-b", map[string]string{"group": ns, "tier": "b"})
	createTemplateWithSelector(t, tc, ns, "tmpl1", &metav1.LabelSelector{
		MatchLabels: map[string]string{"group": ns},
	})
	createWorkerPod(t, tc, ns, "worker-a", "node1", "pool-a")
	createWorkerPod(t, tc, ns, "worker-b", "node1", "pool-b")

	id := "id1"
	_, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                id,
		WorkerSelector:         &runtimeapipb.Selector{MatchLabels: map[string]string{"tier": "a"}},
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}

	tc.fakeWorker.FailRun = fmt.Errorf("mock runtime-worker failure")
	_, err = tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{ActorId: id})
	if err == nil {
		t.Fatalf("expected first ResumeActor (onto worker-a) to fail")
	}
	tc.fakeWorker.FailRun = nil

	if _, err := tc.client.UpdateActor(context.Background(), &runtimeapipb.UpdateActorRequest{
		ActorId:        id,
		WorkerSelector: &runtimeapipb.Selector{MatchLabels: map[string]string{"tier": "b"}},
	}); err != nil {
		t.Fatalf("UpdateActor failed: %v", err)
	}

	if _, err := tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{ActorId: id}); err != nil {
		t.Fatalf("second ResumeActor failed: %v", err)
	}

	getResp, err := tc.client.GetActor(context.Background(), &runtimeapipb.GetActorRequest{ActorId: id})
	if err != nil {
		t.Fatalf("GetActor failed: %v", err)
	}
	if got := getResp.GetActor().GetWorkerPoolName(); got != "pool-b" {
		t.Errorf("expected actor to land on pool-b, got worker_pool_name=%q", got)
	}
	if got := getResp.GetActor().GetStatus(); got != runtimeapipb.Actor_STATUS_RUNNING {
		t.Errorf("expected actor status RUNNING, got %v", got)
	}

	listResp, err := tc.client.ListWorkers(context.Background(), &runtimeapipb.ListWorkersRequest{})
	if err != nil {
		t.Fatalf("ListWorkers failed: %v", err)
	}
	for _, w := range listResp.GetWorkers() {
		if w.GetWorkerNamespace() != ns {
			continue
		}
		switch w.GetWorkerPool() {
		case "pool-a":
			if got := w.GetActorId(); got != "" {
				t.Errorf("expected worker-a (now-ineligible pool-a) to be released, got actor_id=%q", got)
			}
		case "pool-b":
			if got := w.GetActorId(); got != id {
				t.Errorf("expected worker-b to be claimed by %q, got actor_id=%q", id, got)
			}
		}
	}
}

// TestUpdateActor_ReassignsPoolAcrossSuspendResume verifies that updating an
// actor's worker_selector moves it onto a different eligible pool not just
// on the next fresh resume, but also across a full suspend/resume cycle of
// an already-running actor.
// Workflow:
//  1. Creates two WorkerPools, pool-a (tier=a) and pool-b (tier=b), both
//     under the template's gating selector.
//  2. Creates an actor narrowed to tier=a and resumes it; asserts it lands on
//     pool-a/worker-a.
//  3. Updates the actor's selector to tier=b while it's still running.
//  4. Suspends then resumes the actor; asserts it now lands on
//     pool-b/worker-b, proving the updated selector — not the one in effect
//     when it was first scheduled — governs the new placement.
func TestUpdateActor_ReassignsPoolAcrossSuspendResume(t *testing.T) {
	ns := namespaceForTest("ns-update-actor-suspend-resume")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createWorkerPool(t, tc, ns, "pool-a", map[string]string{"group": ns, "tier": "a"})
	createWorkerPool(t, tc, ns, "pool-b", map[string]string{"group": ns, "tier": "b"})
	createTemplateWithSelector(t, tc, ns, "tmpl1", &metav1.LabelSelector{
		MatchLabels: map[string]string{"group": ns},
	})

	createWorkerPod(t, tc, ns, "worker-a", "node1", "pool-a")
	createWorkerPod(t, tc, ns, "worker-b", "node1", "pool-b")

	id := "id1"
	_, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                id,
		WorkerSelector: &runtimeapipb.Selector{
			MatchLabels: map[string]string{"tier": "a"},
		},
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}

	if _, err := tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{ActorId: id}); err != nil {
		t.Fatalf("first ResumeActor failed: %v", err)
	}

	getResp, err := tc.client.GetActor(context.Background(), &runtimeapipb.GetActorRequest{ActorId: id})
	if err != nil {
		t.Fatalf("GetActor failed: %v", err)
	}
	if got := getResp.GetActor().GetWorkerPoolName(); got != "pool-a" {
		t.Fatalf("expected actor to first resume onto pool-a, got worker_pool_name=%q", got)
	}
	if got := getResp.GetActor().GetSandboxPodName(); got != "worker-a" {
		t.Fatalf("expected actor to first resume onto worker-a, got sandbox_pod_name=%q", got)
	}

	if _, err := tc.client.UpdateActor(context.Background(), &runtimeapipb.UpdateActorRequest{
		ActorId: id,
		WorkerSelector: &runtimeapipb.Selector{
			MatchLabels: map[string]string{"tier": "b"},
		},
	}); err != nil {
		t.Fatalf("UpdateActor failed: %v", err)
	}

	if _, err := tc.client.SuspendActor(context.Background(), &runtimeapipb.SuspendActorRequest{ActorId: id}); err != nil {
		t.Fatalf("SuspendActor failed: %v", err)
	}
	if _, err := tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{ActorId: id}); err != nil {
		t.Fatalf("second ResumeActor failed: %v", err)
	}

	getResp, err = tc.client.GetActor(context.Background(), &runtimeapipb.GetActorRequest{ActorId: id})
	if err != nil {
		t.Fatalf("GetActor failed: %v", err)
	}
	if got := getResp.GetActor().GetWorkerPoolName(); got != "pool-b" {
		t.Errorf("expected actor to resume onto pool-b after selector update, got worker_pool_name=%q", got)
	}
	if got := getResp.GetActor().GetSandboxPodName(); got != "worker-b" {
		t.Errorf("expected actor to resume onto worker-b after selector update, got sandbox_pod_name=%q", got)
	}
	if got := getResp.GetActor().GetStatus(); got != runtimeapipb.Actor_STATUS_RUNNING {
		t.Errorf("expected actor status RUNNING after second resume, got %v", got)
	}
}

// TestValidation tests the negative validation cases for all gRPC methods.
// Workflow:
// 1. Uses table-driven tests for each RPC method (CreateActor, GetActor, ResumeActor, SuspendActor).
// 2. Passes invalid requests (missing required fields).
// 3. Verifies that all requests fail with an error.
func TestValidation(t *testing.T) {
	ns := namespaceForTest("ns-validation")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	t.Run("CreateActor", func(t *testing.T) {
		tests := []struct {
			name    string
			req     *runtimeapipb.CreateActorRequest
			wantMsg string
		}{
			{
				"missing namespace",
				&runtimeapipb.CreateActorRequest{ActorTemplateName: "tmpl1", ActorId: "id1"},
				"actor_template_namespace is required"},
			{
				"missing template name",
				&runtimeapipb.CreateActorRequest{ActorTemplateNamespace: "ns1", ActorId: "id1"},
				"actor_template_name is required"},
			{
				"missing actor id",
				&runtimeapipb.CreateActorRequest{ActorTemplateNamespace: "ns1", ActorTemplateName: "tmpl1"},
				"actor_id is required"},
			{
				"invalid actor id (capitals)",
				&runtimeapipb.CreateActorRequest{ActorTemplateNamespace: "ns1", ActorTemplateName: "tmpl1", ActorId: "ID1"},
				"invalid actor_id: must start and end with a lower case alphanumeric character, and consist only of lower case alphanumeric characters or '-'"},
			{
				"invalid actor id (special chars)",
				&runtimeapipb.CreateActorRequest{ActorTemplateNamespace: "ns1", ActorTemplateName: "tmpl1", ActorId: "id_1"},
				"invalid actor_id: must start and end with a lower case alphanumeric character, and consist only of lower case alphanumeric characters or '-'"},
			{
				"invalid worker_selector label key",
				&runtimeapipb.CreateActorRequest{ActorTemplateNamespace: "ns1", ActorTemplateName: "tmpl1", ActorId: "id1", WorkerSelector: &runtimeapipb.Selector{MatchLabels: map[string]string{"bad key!": "x"}}},
				`invalid worker_selector label key "bad key!": name part must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyName',  or 'my.name',  or '123-abc', regex used for validation is '([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]')`,
			},
			{
				"invalid worker_selector label value",
				&runtimeapipb.CreateActorRequest{ActorTemplateNamespace: "ns1", ActorTemplateName: "tmpl1", ActorId: "id1", WorkerSelector: &runtimeapipb.Selector{MatchLabels: map[string]string{"tier": "not valid!"}}},
				`invalid worker_selector label value "not valid!" for key "tier": a valid label must be an empty string or consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyValue',  or 'my_value',  or '12345', regex used for validation is '(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?')`,
			},
			{
				"too many worker_selector match_labels",
				&runtimeapipb.CreateActorRequest{ActorTemplateNamespace: "ns1", ActorTemplateName: "tmpl1", ActorId: "id1", WorkerSelector: &runtimeapipb.Selector{MatchLabels: selectorLabelsOfSize(11)}},
				"worker_selector has 11 match_labels entries, exceeding the limit of 10",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := tc.client.CreateActor(context.Background(), tt.req)
				assertGrpcError(t, err, codes.InvalidArgument, tt.wantMsg)
			})
		}
	})

	t.Run("GetActor", func(t *testing.T) {
		tests := []struct {
			name    string
			req     *runtimeapipb.GetActorRequest
			wantMsg string
		}{
			{"missing id", &runtimeapipb.GetActorRequest{}, "id is required"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := tc.client.GetActor(context.Background(), tt.req)
				assertGrpcError(t, err, codes.InvalidArgument, tt.wantMsg)
			})
		}
	})

	t.Run("ResumeActor", func(t *testing.T) {
		tests := []struct {
			name    string
			req     *runtimeapipb.ResumeActorRequest
			wantMsg string
		}{
			{"missing id", &runtimeapipb.ResumeActorRequest{}, "id is required"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := tc.client.ResumeActor(context.Background(), tt.req)
				assertGrpcError(t, err, codes.InvalidArgument, tt.wantMsg)
			})
		}
	})

	t.Run("SuspendActor", func(t *testing.T) {
		tests := []struct {
			name    string
			req     *runtimeapipb.SuspendActorRequest
			wantMsg string
		}{
			{"missing id", &runtimeapipb.SuspendActorRequest{}, "id is required"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := tc.client.SuspendActor(context.Background(), tt.req)
				assertGrpcError(t, err, codes.InvalidArgument, tt.wantMsg)
			})
		}
	})

	t.Run("UpdateActor", func(t *testing.T) {
		tests := []struct {
			name    string
			req     *runtimeapipb.UpdateActorRequest
			wantMsg string
		}{
			{"missing id", &runtimeapipb.UpdateActorRequest{}, "actor_id is required"},
			{
				"invalid worker_selector label key",
				&runtimeapipb.UpdateActorRequest{ActorId: "id1", WorkerSelector: &runtimeapipb.Selector{MatchLabels: map[string]string{"bad key!": "x"}}},
				`invalid worker_selector label key "bad key!": name part must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyName',  or 'my.name',  or '123-abc', regex used for validation is '([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]')`,
			},
			{
				"invalid worker_selector label value",
				&runtimeapipb.UpdateActorRequest{ActorId: "id1", WorkerSelector: &runtimeapipb.Selector{MatchLabels: map[string]string{"tier": "not valid!"}}},
				`invalid worker_selector label value "not valid!" for key "tier": a valid label must be an empty string or consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyValue',  or 'my_value',  or '12345', regex used for validation is '(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?')`,
			},
			{
				"too many worker_selector match_labels",
				&runtimeapipb.UpdateActorRequest{ActorId: "id1", WorkerSelector: &runtimeapipb.Selector{MatchLabels: selectorLabelsOfSize(11)}},
				"worker_selector has 11 match_labels entries, exceeding the limit of 10",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := tc.client.UpdateActor(context.Background(), tt.req)
				assertGrpcError(t, err, codes.InvalidArgument, tt.wantMsg)
			})
		}
	})

	t.Run("DeleteActor", func(t *testing.T) {
		tests := []struct {
			name    string
			req     *runtimeapipb.DeleteActorRequest
			wantMsg string
		}{
			{"missing id", &runtimeapipb.DeleteActorRequest{}, "actor_id is required"},
			{"invalid actor id (capitals)", &runtimeapipb.DeleteActorRequest{ActorId: "ID1"}, "invalid actor_id: must start and end with a lower case alphanumeric character, and consist only of lower case alphanumeric characters or '-'"},
			{"invalid actor id (special chars)", &runtimeapipb.DeleteActorRequest{ActorId: "id_1"}, "invalid actor_id: must start and end with a lower case alphanumeric character, and consist only of lower case alphanumeric characters or '-'"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := tc.client.DeleteActor(context.Background(), tt.req)
				assertGrpcError(t, err, codes.InvalidArgument, tt.wantMsg)
			})
		}
	})
}

func TestResumeActor_LockConflict(t *testing.T) {
	ns := namespaceForTest("ns-resume-conflict")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createTemplate(t, tc, ns)

	createWorkerPod(t, tc, ns, "worker-1", "node1", "pool1")

	_, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}
	id := "id1"

	// Set a delay on the fake Worker to hold the lock
	tc.fakeWorker.RestoreDelay = 1 * time.Second

	// Launch Request A in a goroutine
	errChan := make(chan error, 1)
	go func() {
		_, err := tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{
			ActorId: id,
		})
		errChan <- err
	}()

	// Sleep a bit to ensure Request A acquired the lock
	time.Sleep(200 * time.Millisecond)

	// Launch Request B (should fail due to lock conflict)
	_, err = tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{
		ActorId: id,
	})
	assertGrpcError(t, err, codes.Aborted, "another operation is in progress for this actor")

	// Wait for Request A to finish
	if errA := <-errChan; errA != nil {
		t.Fatalf("Request A failed: %v", errA)
	}
}

func TestResumeActor_DanglingWorker(t *testing.T) {
	ns := namespaceForTest("ns-resume-dangling")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createTemplate(t, tc, ns)

	// 1. Create Worker Pod A
	createWorkerPod(t, tc, ns, "worker-a", "node1", "pool1")

	_, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}
	id := "id1"

	// 2. Configure fake Worker to FAIL on Restore!
	tc.fakeWorker.FailRestore = fmt.Errorf("mock runtime-worker failure")

	// 3. Call ResumeActor -> Expect failure
	_, err = tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{
		ActorId: id,
	})
	if err == nil {
		t.Fatalf("expected ResumeActor to fail due to runtime-worker error")
	}

	// Verify actor state is RESUMING with worker A assigned
	getResp, err := tc.client.GetActor(context.Background(), &runtimeapipb.GetActorRequest{
		ActorId: id,
	})
	if err != nil {
		t.Fatalf("GetActor failed: %v", err)
	}
	actor := getResp.GetActor()
	if actor.GetStatus() != runtimeapipb.Actor_STATUS_RESUMING {
		t.Fatalf("expected status RESUMING, got %v", actor.GetStatus())
	}
	if actor.GetSandboxPodName() != "worker-a" {
		t.Fatalf("expected worker-a assigned, got %v", actor.GetSandboxPodName())
	}

	deleteWorkerPod(t, tc, ns, "worker-a")

	// 6. Create Worker Pod B
	createWorkerPod(t, tc, ns, "worker-b", "node1", "pool1")

	// 7. Configure fake Worker to SUCCEED on Restore
	tc.fakeWorker.FailRestore = nil
	tc.fakeWorker.RestoreCalled = false // reset

	// 8. Call ResumeActor again -> Expect success and picking Worker B!
	_, err = tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{
		ActorId: id,
	})
	if err != nil {
		t.Fatalf("ResumeActor failed on retry: %v", err)
	}

	if !tc.fakeWorker.RestoreCalled {
		t.Errorf("expected Restore to be called on retry")
	}

	// Verify actor state is RUNNING with worker B assigned
	actor, err = tc.persistence.GetActor(context.Background(), id)
	if err != nil {
		t.Fatalf("failed to get actor from store: %v", err)
	}
	if actor.GetStatus() != runtimeapipb.Actor_STATUS_RUNNING {
		t.Errorf("expected status RUNNING, got %v", actor.GetStatus())
	}
	if actor.GetSandboxPodName() != "worker-b" {
		t.Errorf("expected worker-b assigned, got %v", actor.GetSandboxPodName())
	}
}

func TestSuspendActor_DanglingWorker(t *testing.T) {
	ns := namespaceForTest("ns-sd")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createTemplate(t, tc, ns)

	// 1. Create Worker Pod
	createWorkerPod(t, tc, ns, "worker-1", "node1", "pool1")

	_, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}
	id := "id1"

	// Resume first to make it running
	_, err = tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{
		ActorId: id,
	})
	if err != nil {
		t.Fatalf("ResumeActor failed: %v", err)
	}

	deleteWorkerPod(t, tc, ns, "worker-1")

	// 3. Call SuspendActor -> Should succeed (our fix skips missing pod execution)
	actors, _, _ := tc.persistence.ListActors(context.Background(), maxPageSize, "")
	t.Logf("Actors in Redis before Suspend: %d", len(actors))
	for _, a := range actors {
		t.Logf("  Actor: %s/%s/%s", a.GetActorTemplateNamespace(), a.GetActorTemplateName(), a.GetActorId())
	}

	_, err = tc.client.SuspendActor(context.Background(), &runtimeapipb.SuspendActorRequest{
		ActorId: id,
	})
	if err != nil {
		t.Fatalf("SuspendActor failed: %v", err)
	}

	// 4. Verify it becomes SUSPENDED in Redis
	getResp, err := tc.client.GetActor(context.Background(), &runtimeapipb.GetActorRequest{
		ActorId: id,
	})
	if err != nil {
		t.Fatalf("GetActor failed: %v", err)
	}
	if getResp.GetActor().GetStatus() != runtimeapipb.Actor_STATUS_SUSPENDED {
		t.Errorf("expected status SUSPENDED, got %v", getResp.GetActor().GetStatus())
	}
	if getResp.GetActor().GetSandboxPodNamespace() != "" {
		t.Errorf("expected sandbox_pod_namespace to be empty, got %v", getResp.GetActor().GetSandboxPodNamespace())
	}
}

func TestDeleteActor_Success(t *testing.T) {
	ns := namespaceForTest("ns-delete-success")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createTemplate(t, tc, ns)

	_, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}

	_, err = tc.client.DeleteActor(context.Background(), &runtimeapipb.DeleteActorRequest{
		ActorId: "id1",
	})
	if err != nil {
		t.Fatalf("DeleteActor failed: %v", err)
	}

	_, err = tc.client.GetActor(context.Background(), &runtimeapipb.GetActorRequest{
		ActorId: "id1",
	})
	assertGrpcError(t, err, codes.NotFound, "Actor id1 not found")
}

func TestDeleteActor_NotSuspended(t *testing.T) {
	ns := namespaceForTest("ns-delete-notsuspended")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	createTemplate(t, tc, ns)
	createWorkerPod(t, tc, ns, "worker-1", "node1", "pool1")

	_, err := tc.client.CreateActor(context.Background(), &runtimeapipb.CreateActorRequest{
		ActorTemplateNamespace: ns,
		ActorTemplateName:      "tmpl1",
		ActorId:                "id1",
	})
	if err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}

	_, err = tc.client.ResumeActor(context.Background(), &runtimeapipb.ResumeActorRequest{
		ActorId: "id1",
	})
	if err != nil {
		t.Fatalf("ResumeActor failed: %v", err)
	}

	_, err = tc.client.DeleteActor(context.Background(), &runtimeapipb.DeleteActorRequest{
		ActorId: "id1",
	})
	assertGrpcError(t, err, codes.FailedPrecondition, "Actor id1 is not suspended (status: STATUS_RUNNING)")
}

func TestDeleteActor_NotFound(t *testing.T) {
	ns := namespaceForTest("ns-delete-notfound")
	tc := setupTest(t, ns)
	defer tc.cleanup()

	_, err := tc.client.DeleteActor(context.Background(), &runtimeapipb.DeleteActorRequest{
		ActorId: "non-existent",
	})
	assertGrpcError(t, err, codes.NotFound, "Actor non-existent not found")
}

func assertGrpcError(t *testing.T, err error, wantCode codes.Code, wantMsg string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %v", err)
	}
	if st.Code() != wantCode {
		t.Errorf("expected status %v, got %v", wantCode, st.Code())
	}
	if st.Message() != wantMsg {
		t.Errorf("expected message %q, got %q", wantMsg, st.Message())
	}
}
