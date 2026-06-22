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

package store

import "time"

const (
	StatusRunning = "running"
	StatusPending = "pending"
	StatusFailed  = "failed"
	StatusPaused  = "paused"
)

// Sandbox is persisted sandbox metadata for v0.0.2+ visibility APIs.
type Sandbox struct {
	SandboxID           string         `json:"sandbox_id"`
	ActorID             string         `json:"actor_id"`
	Template            string         `json:"template"`
	CreatedAt           time.Time      `json:"created_at"`
	ExpiresAt           time.Time      `json:"expires_at"`
	OnTimeout           string         `json:"on_timeout"`
	AutoResume          bool           `json:"auto_resume,omitempty"`
	Status              string         `json:"status"`
	Secure              bool           `json:"secure,omitempty"`
	EnvdAccessToken     string         `json:"envd_access_token,omitempty"`
	TrafficAccessToken  string         `json:"traffic_access_token,omitempty"`
	Network             *NetworkConfig `json:"network,omitempty"`
	AllowInternetAccess *bool          `json:"allow_internet_access,omitempty"`
	VolumeMounts        []VolumeMount  `json:"volume_mounts,omitempty"`
}
