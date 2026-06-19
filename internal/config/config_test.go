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

package config

import (
	"testing"
)

func TestServerFromEnvDefaults(t *testing.T) {
	t.Setenv("PLATFORM_LISTEN_ADDR", "")
	t.Setenv("PLATFORM_LOG_LEVEL", "")
	t.Setenv("ACTORDOCK_LOG_LEVEL", "")

	cfg, err := ServerFromEnv("platform", ":8080")
	if err != nil {
		t.Fatalf("ServerFromEnv: %v", err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Fatalf("ListenAddr = %q, want :8080", cfg.ListenAddr)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want info", cfg.LogLevel)
	}
}

func TestServerFromEnvOverrides(t *testing.T) {
	t.Setenv("ROUTER_LISTEN_ADDR", ":9090")
	t.Setenv("ROUTER_LOG_LEVEL", "debug")

	cfg, err := ServerFromEnv("router", ":8081")
	if err != nil {
		t.Fatalf("ServerFromEnv: %v", err)
	}
	if cfg.ListenAddr != ":9090" {
		t.Fatalf("ListenAddr = %q, want :9090", cfg.ListenAddr)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}
