//go:build linux

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

// Command runtime-sandbox-microvm is the kata + cloud-hypervisor micro-VM
// implementation of the runtimesandboxpb.Sandbox service, a peer to cmd/runtime-sandbox.
//
// It runs a actordock actor as a cloud-hypervisor micro-VM (launched via the
// kata guest model) and supports full suspend/resume by driving CH's native
// snapshot/restore underneath (see internal/ch).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"

	"cloud.google.com/go/compute/metadata"
	"github.com/actordock/runtime/internal/actorlog"
	"github.com/actordock/runtime/internal/proto/runtimesandboxpb"
	"github.com/actordock/runtime/internal/runtimeinterceptors"
	"github.com/actordock/runtime/internal/sandboxpath"
	"github.com/actordock/runtime/internal/serverboot"
	"github.com/actordock/runtime/internal/version"
	"github.com/hashicorp/go-reap"
	"github.com/vishvananda/netns"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	podUID      = flag.String("pod-uid", "", "The UID of the current pod")
	chBinary    = flag.String("cloud-hypervisor-binary", "cloud-hypervisor", "Path to the cloud-hypervisor binary (used to relaunch on restore).")
	kataConfig  = flag.String("kata-config", "", "Path to a kata configuration.toml (passed to the shim as KATA_CONF_FILE). Empty uses kata's default. runtime-worker generates one pointing at runtime-fetched assets.")
	kataDebug   = flag.Bool("kata-debug", false, "Verbose kata-agent debugging: raise the guest agent log level and forward the guest console (incl. agent logs) into the pod logs.")
	showVersion = flag.Bool("version", false, "Print version and exit.")

	// reapLock guards subprocess exec against the child reaper: runtime-sandbox-microvm
	// spawns the cloud-hypervisor process under it.
	reapLock sync.RWMutex
)

func main() {
	flag.Parse()
	if *showVersion {
		fmt.Println(version.String())
		return
	}
	ctx := context.Background()

	if err := do(ctx); err != nil {
		slog.ErrorContext(ctx, "Error while executing", slog.Any("err", err))
		os.Exit(1)
	}
}

func do(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Share one synchronized writer between the runtime logger and the actor-log
	// forwarder (created below) so the two log streams to the pod's stdout don't
	// interleave-corrupt each other's lines.
	logWriter := actorlog.NewSyncedWriter(os.Stdout)
	serverboot.InitLoggerWithWriter(logWriter)
	slog.InfoContext(ctx, "runtime-sandbox-microvm booting", slog.String("version", version.String()))

	tp, err := serverboot.InitTracing(ctx, serverboot.TracingOptions{
		ServiceName: "runtime-sandbox-microvm",
		Sampler:     sdktrace.ParentBased(sdktrace.NeverSample()),
	})
	if err != nil {
		serverboot.Fatal(ctx, "Failed to initialize tracing", err)
	}
	defer serverboot.ShutdownProvider("TracerProvider", tp.Shutdown)

	// Create runtime-sandbox dir.
	sandboxDir := sandboxpath.SandboxPath(*podUID)
	if err := os.MkdirAll(sandboxDir, 0o700); err != nil {
		return fmt.Errorf("in os.MkdirAll(%q): %w", sandboxDir, err)
	}

	// Reap children reparented to us (cloud-hypervisor), guarded so our own
	// exec.Cmd calls can take the wait.
	go reap.ReapChildren(nil, nil, nil, &reapLock)
	slog.InfoContext(ctx, "Child process reaper launched")

	// kata's virtio-fs sharing depends on mount propagation: it slave-binds
	// .../shared (served by virtiofsd) from .../mounts and expects the later
	// per-container rootfs bind under mounts/ to propagate across. That only
	// works if the underlying mount is SHARED. On a host systemd makes /
	// rshared, but a container rootfs is rprivate (runc default), so the
	// propagation silently never happens: the guest sees an empty rootfs and
	// createContainer fails ENOENT. Self-bind /run/kata-containers and mark it
	// rshared so kata's propagation chain works inside the pod.
	if err := ensureSharedPropagation(ctx, "/run/kata-containers"); err != nil {
		return fmt.Errorf("while making /run/kata-containers a shared mount: %w", err)
	}

	// Clean up any old socket.
	sockPath := sandboxpath.SandboxSocketPath(*podUID)
	if err := os.RemoveAll(sockPath); err != nil {
		return fmt.Errorf("while removing %q: %w", sockPath, err)
	}

	lis, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("while opening unix socket: %w", err)
	}

	// Networking: create a named interior netns; each activation builds a fresh
	// veth pair into it (see net.go) and points kata at it.
	interiorNetNS, err := createNetNSWithoutSwitching(sandboxpath.SandboxNetNSName(*podUID))
	if err != nil {
		return fmt.Errorf("while creating interior netns: %w", err)
	}

	// Forward the actor container's stdout/stderr to the worker pod's stdout as
	// JSON with actordock.dev/* labels (logging parity with runtime-sandbox). It shares
	// logWriter with the runtime logger so the two streams to os.Stdout are
	// serialized through one SyncedWriter and never interleave-corrupt lines.
	actorLogger := actorlog.NewActorLogger(logWriter, metadata.OnGCE())

	svr := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.UnaryInterceptor(runtimeinterceptors.ServerUnaryInterceptor),
	)
	runtimesandboxpb.RegisterSandboxServer(svr, NewService(*podUID, *chBinary, *kataConfig, *kataDebug, interiorNetNS, actorLogger))
	reflection.Register(svr)

	slog.InfoContext(ctx, "runtime-sandbox-microvm serving", slog.String("socket", sockPath))
	if err := svr.Serve(lis); err != nil {
		return fmt.Errorf("while serving: %w", err)
	}
	return nil
}

// ensureSharedPropagation makes path a mount point with rshared propagation
// (self-bind + MS_SHARED|MS_REC), so mounts created beneath it propagate to
// slave binds (kata's mounts/ -> shared/ chain). Idempotent: skips if path is
// already a shared mount point.
func ensureSharedPropagation(ctx context.Context, path string) error {
	if err := os.MkdirAll(path, 0o750); err != nil {
		return fmt.Errorf("creating %q: %w", path, err)
	}
	if b, err := os.ReadFile("/proc/self/mountinfo"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			// mountinfo: ID parentID major:minor root mountpoint opts optional... - fstype ...
			fields := strings.Fields(line)
			if len(fields) >= 7 && fields[4] == path && strings.Contains(line, "shared:") {
				slog.InfoContext(ctx, "Mount already shared", slog.String("path", path))
				return nil
			}
		}
	}
	if err := unix.Mount(path, path, "", unix.MS_BIND, ""); err != nil {
		return fmt.Errorf("self-binding %q: %w", path, err)
	}
	if err := unix.Mount("", path, "", unix.MS_SHARED|unix.MS_REC, ""); err != nil {
		return fmt.Errorf("marking %q rshared: %w", path, err)
	}
	slog.InfoContext(ctx, "Made mount rshared for kata virtio-fs propagation", slog.String("path", path))
	return nil
}

// SandboxService is the cloud-hypervisor implementation of runtimesandboxpb.SandboxServer.
type SandboxService struct {
	runtimesandboxpb.UnimplementedSandboxServer

	// lock serializes RPCs; like runtime-sandbox, the run/checkpoint/restore
	// lifecycle is not safe to drive concurrently.
	lock sync.Mutex

	podUID     string
	chBinary   string
	kataConfig string
	kataDebug  bool

	// interiorNetNS hosts the per-activation actor veth peer (see net.go);
	// kata is pointed at it.
	interiorNetNS netns.NsHandle

	// actorLogger forwards the actor container's stdout/stderr to the worker pod's
	// stdout as actordock.dev/*-labeled JSON and emits actor lifecycle events (parity
	// with runtime-sandbox).
	actorLogger *actorlog.ActorLogger

	// running maps actor id -> the live micro-VM, kept so CheckpointWorkload can
	// pause+snapshot+teardown the same sandbox (and RestoreWorkload can track the
	// CH it relaunched).
	running map[string]*runningActor
}

var _ runtimesandboxpb.SandboxServer = (*SandboxService)(nil)

// NewService creates a new SandboxService.
func NewService(podUID, chBinary, kataConfig string, kataDebug bool, interiorNetNS netns.NsHandle, actorLogger *actorlog.ActorLogger) *SandboxService {
	return &SandboxService{
		podUID:        podUID,
		chBinary:      chBinary,
		kataConfig:    kataConfig,
		kataDebug:     kataDebug,
		interiorNetNS: interiorNetNS,
		actorLogger:   actorLogger,
		running:       map[string]*runningActor{},
	}
}
