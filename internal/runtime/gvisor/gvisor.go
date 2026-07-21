// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

// Package gvisor implements Runtime via runsc, aligned with Substrate ateom-gvisor flags.
package gvisor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/actordock/actordock/internal/runtime"
)

// Config controls runsc layout on a Worker.
type Config struct {
	RunscPath string
	StateDir  string // parent for per-sandbox runsc --root
	BundleDir string
	Rootfs    string
	Platform  string
	Network   string // default none (Kind-safe)
	Timeout   time.Duration
}

// Runtime drives runsc create/start/checkpoint/restore/delete.
type Runtime struct {
	cfg Config
	mu  sync.Mutex
}

func New(cfg Config) (*Runtime, error) {
	if cfg.RunscPath == "" {
		cfg.RunscPath = "runsc"
	}
	if cfg.StateDir == "" {
		cfg.StateDir = "/var/lib/actordock/runsc"
	}
	if cfg.BundleDir == "" {
		cfg.BundleDir = "/var/lib/actordock/bundles"
	}
	if cfg.Rootfs == "" {
		cfg.Rootfs = "/opt/actordock/rootfs"
	}
	if cfg.Platform == "" {
		cfg.Platform = "systrap"
	}
	if cfg.Network == "" {
		cfg.Network = "none"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 45 * time.Second
	}
	for _, d := range []string{cfg.StateDir, cfg.BundleDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	if _, err := os.Stat(cfg.Rootfs); err != nil {
		return nil, fmt.Errorf("rootfs %q: %w", cfg.Rootfs, err)
	}
	if _, err := exec.LookPath(cfg.RunscPath); err != nil {
		return nil, fmt.Errorf("runsc binary %q: %w", cfg.RunscPath, err)
	}
	return &Runtime{cfg: cfg}, nil
}

func (r *Runtime) rootFor(id string) string {
	return filepath.Join(r.cfg.StateDir, id)
}

func (r *Runtime) bundlePath(id string) string {
	return filepath.Join(r.cfg.BundleDir, id)
}

// prepareRootfs copies the shared template into a per-sandbox rootfs.
// Sharing one host rootfs across sandboxes leaves .gvisor.filestore.<id> files
// that break the next restore of the same id (overlay "repeated submounts").
// Substrate uses a per-actor OCI rootfs for the same reason.
func (r *Runtime) prepareRootfs(id, template string) (string, error) {
	if template == "" {
		template = r.cfg.Rootfs
	}
	dest := filepath.Join(r.bundlePath(id), "rootfs")
	if err := os.RemoveAll(dest); err != nil {
		return "", err
	}
	if err := os.MkdirAll(r.bundlePath(id), 0o755); err != nil {
		return "", err
	}
	cmd := exec.Command("cp", "-a", template, dest)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("cp rootfs %q -> %q: %w (%s)", template, dest, err, strings.TrimSpace(string(out)))
	}
	return dest, nil
}

func (r *Runtime) writeBundle(id, template string) error {
	rootfs, err := r.prepareRootfs(id, template)
	if err != nil {
		return err
	}
	dir := r.bundlePath(id)
	spec := map[string]any{
		"ociVersion": "1.0.2",
		"process": map[string]any{
			"terminal": false,
			"user":     map[string]any{"uid": 0, "gid": 0},
			"args":     []string{"/bin/sleep", "infinity"},
			"env":      []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
			"cwd":      "/",
		},
		"root": map[string]any{
			"path":     rootfs,
			"readonly": false,
		},
		"hostname": id,
		"linux": map[string]any{
			"namespaces": []map[string]string{
				{"type": "pid"},
				{"type": "ipc"},
				{"type": "uts"},
				{"type": "mount"},
			},
		},
	}
	b, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), b, 0o644)
}

func (r *Runtime) runsc(ctx context.Context, args ...string) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.cfg.Timeout)
		defer cancel()
	}
	full := append([]string{
		"-log-format", "json",
		"--alsologtostderr",
		"-platform", r.cfg.Platform,
		"-network", r.cfg.Network,
	}, args...)
	// Inherit stdio like Substrate ateom-gvisor: CombinedOutput pipes can deadlock
	// with runsc gofer children holding the write end.
	cmd := exec.CommandContext(ctx, r.cfg.RunscPath, full...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("runsc %s: %w", strings.Join(args, " "), err)
	}
	slog.InfoContext(ctx, "runsc ok", "args", args)
	return nil
}

func (r *Runtime) Boot(ctx context.Context, spec runtime.SandboxSpec) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	bootCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.cfg.Timeout)
	defer cancel()

	root := r.rootFor(spec.ID)
	_ = os.RemoveAll(root)
	_ = os.RemoveAll(r.bundlePath(spec.ID))
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if err := r.writeBundle(spec.ID, spec.Rootfs); err != nil {
		return err
	}
	bundle := r.bundlePath(spec.ID)
	slog.InfoContext(bootCtx, "runsc boot create", "id", spec.ID)
	if err := r.runsc(bootCtx, "-root", root, "create", "-bundle", bundle, spec.ID); err != nil {
		return err
	}
	slog.InfoContext(bootCtx, "runsc boot start", "id", spec.ID)
	if err := r.runsc(bootCtx, "-root", root, "-allow-connected-on-save", "start", spec.ID); err != nil {
		_ = r.runsc(context.Background(), "-root", root, "delete", "-force", spec.ID)
		return err
	}
	slog.InfoContext(bootCtx, "runsc boot ok", "id", spec.ID)
	return nil
}

func (r *Runtime) Checkpoint(ctx context.Context, id, imagePath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.cfg.Timeout)
	defer cancel()

	// runsc refuses to overwrite an existing checkpoint.img. Restore reuses the
	// same image-path, so clear it before writing a new checkpoint (Substrate
	// uses separate CheckpointStateDir / RestoreStateDir for the same reason).
	if err := os.RemoveAll(imagePath); err != nil {
		return fmt.Errorf("clear image path %q: %w", imagePath, err)
	}
	if err := os.MkdirAll(imagePath, 0o755); err != nil {
		return err
	}
	root := r.rootFor(id)
	if err := r.runsc(opCtx, "-root", root, "checkpoint", "-image-path", imagePath, id); err != nil {
		return err
	}
	_ = r.runsc(context.Background(), "-root", root, "delete", "-force", id)
	return nil
}

func (r *Runtime) Restore(ctx context.Context, id, imagePath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.cfg.Timeout)
	defer cancel()

	root := r.rootFor(id)
	_ = os.RemoveAll(root)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	// Always rebuild per-sandbox rootfs so leftover .gvisor.filestore.* cannot
	// block restore of the same id after pause/checkpoint.
	if err := r.writeBundle(id, ""); err != nil {
		return err
	}
	bundle := r.bundlePath(id)
	return r.runsc(opCtx,
		"-root", root,
		"-allow-connected-on-save",
		"restore",
		"-bundle", bundle,
		"-image-path", imagePath,
		"-background",
		"-direct",
		"-detach",
		id,
	)
}

func (r *Runtime) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()

	root := r.rootFor(id)
	_ = r.runsc(opCtx, "-root", root, "kill", id, "SIGKILL")
	_ = r.runsc(opCtx, "-root", root, "delete", "-force", id)
	_ = os.RemoveAll(root)
	_ = os.RemoveAll(r.bundlePath(id))
	return nil
}

func (r *Runtime) Exists(ctx context.Context, id string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	root := r.rootFor(id)
	cmd := exec.CommandContext(ctx, r.cfg.RunscPath,
		"-platform", r.cfg.Platform,
		"-network", r.cfg.Network,
		"-root", root,
		"state", id,
	)
	if err := cmd.Run(); err != nil {
		return false, nil
	}
	return true, nil
}

func (r *Runtime) Exec(ctx context.Context, id string, argv []string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(argv) == 0 {
		return "", fmt.Errorf("argv required")
	}
	opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.cfg.Timeout)
	defer cancel()

	args := append([]string{
		"-platform", r.cfg.Platform,
		"-network", r.cfg.Network,
		"-root", r.rootFor(id),
		"exec", id,
	}, argv...)
	cmd := exec.CommandContext(opCtx, r.cfg.RunscPath, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("runsc exec %s %v: %w (%s)", id, argv, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
