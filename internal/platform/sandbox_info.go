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

package platform

import (
	"time"

	"github.com/actordock/actordock/internal/config"
	"github.com/actordock/actordock/internal/store"
	"github.com/agent-substrate/substrate/pkg/proto/ateapipb"
)

const (
	defaultCPUCount   = 2
	defaultDiskSizeMB = 512
	defaultMemoryMB   = 512
)

type sandboxLifecycleResponse struct {
	OnTimeout  string `json:"onTimeout"`
	AutoResume bool   `json:"autoResume"`
}

type sandboxDetailResponse struct {
	ClientID    string                   `json:"clientID"`
	CPUCount    int                      `json:"cpuCount"`
	DiskSizeMB  int                      `json:"diskSizeMB"`
	EndAt       string                   `json:"endAt"`
	EnvdVersion string                   `json:"envdVersion"`
	MemoryMB    int                      `json:"memoryMB"`
	SandboxID   string                   `json:"sandboxID"`
	StartedAt   string                   `json:"startedAt"`
	State       string                   `json:"state"`
	TemplateID  string                   `json:"templateID"`
	Domain      string                   `json:"domain,omitempty"`
	Lifecycle   sandboxLifecycleResponse `json:"lifecycle"`
}

type listedSandboxResponse struct {
	ClientID    string                   `json:"clientID"`
	CPUCount    int                      `json:"cpuCount"`
	DiskSizeMB  int                      `json:"diskSizeMB"`
	EndAt       string                   `json:"endAt"`
	EnvdVersion string                   `json:"envdVersion"`
	MemoryMB    int                      `json:"memoryMB"`
	SandboxID   string                   `json:"sandboxID"`
	StartedAt   string                   `json:"startedAt"`
	State       string                   `json:"state"`
	TemplateID  string                   `json:"templateID"`
	Lifecycle   sandboxLifecycleResponse `json:"lifecycle"`
}

func buildSandboxDetail(cfg config.Platform, sb store.Sandbox, state string) sandboxDetailResponse {
	return sandboxDetailResponse{
		ClientID:    cfg.ClientID,
		CPUCount:    defaultCPUCount,
		DiskSizeMB:  defaultDiskSizeMB,
		EndAt:       sandboxEndAt(cfg, sb),
		EnvdVersion: cfg.EnvdVersion,
		MemoryMB:    defaultMemoryMB,
		SandboxID:   sb.SandboxID,
		StartedAt:   sb.CreatedAt.UTC().Format(time.RFC3339),
		State:       state,
		TemplateID:  sb.Template,
		Domain:      cfg.Domain,
		Lifecycle:   buildSandboxLifecycle(sb),
	}
}

func buildSandboxLifecycle(sb store.Sandbox) sandboxLifecycleResponse {
	onTimeout, _ := store.ResolveOnTimeout(sb.OnTimeout)
	return sandboxLifecycleResponse{
		OnTimeout:  onTimeout,
		AutoResume: sb.AutoResume,
	}
}

func buildSandboxResponse(cfg config.Platform, sb store.Sandbox) sandboxResponse {
	return sandboxResponse{
		ClientID:    cfg.ClientID,
		EnvdVersion: cfg.EnvdVersion,
		SandboxID:   sb.SandboxID,
		TemplateID:  sb.Template,
		Domain:      cfg.Domain,
	}
}

func sandboxEndAt(cfg config.Platform, sb store.Sandbox) string {
	expiresAt := sb.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = store.ExpiresAt(sb.CreatedAt, cfg.DefaultSandboxTimeout)
	}
	return expiresAt.UTC().Format(time.RFC3339)
}

func listedFromDetail(d sandboxDetailResponse) listedSandboxResponse {
	return listedSandboxResponse{
		ClientID:    d.ClientID,
		CPUCount:    d.CPUCount,
		DiskSizeMB:  d.DiskSizeMB,
		EndAt:       d.EndAt,
		EnvdVersion: d.EnvdVersion,
		MemoryMB:    d.MemoryMB,
		SandboxID:   d.SandboxID,
		StartedAt:   d.StartedAt,
		State:       d.State,
		TemplateID:  d.TemplateID,
		Lifecycle:   d.Lifecycle,
	}
}

func storeStatusFromActor(status ateapipb.Actor_Status) string {
	switch status {
	case ateapipb.Actor_STATUS_RESUMING:
		return store.StatusPending
	case ateapipb.Actor_STATUS_RUNNING:
		return store.StatusRunning
	case ateapipb.Actor_STATUS_SUSPENDED,
		ateapipb.Actor_STATUS_PAUSED,
		ateapipb.Actor_STATUS_SUSPENDING,
		ateapipb.Actor_STATUS_PAUSING:
		return store.StatusPaused
	default:
		return store.StatusRunning
	}
}
