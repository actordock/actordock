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

package server

import (
	"testing"
)

func TestConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("ACTORDOCK_DASHBOARD_ADDR", "")
	t.Setenv("ACTORDOCK_PLATFORM_URL", "")
	t.Setenv("ACTORDOCK_API_KEY", "")
	t.Setenv("ACTORDOCK_DASHBOARD_PROXY_PLATFORM", "")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	if cfg.ListenAddr != ":3000" {
		t.Fatalf("ListenAddr = %q, want :3000", cfg.ListenAddr)
	}
	if cfg.PlatformURL != "http://platform:8080" {
		t.Fatalf("PlatformURL = %q", cfg.PlatformURL)
	}
	if cfg.APIKey != "dev" {
		t.Fatalf("APIKey = %q, want dev", cfg.APIKey)
	}
	if !cfg.ProxyPlatform {
		t.Fatal("ProxyPlatform = false, want true")
	}
}

func TestConfigFromEnvOverrides(t *testing.T) {
	t.Setenv("ACTORDOCK_DASHBOARD_ADDR", ":4000")
	t.Setenv("ACTORDOCK_PLATFORM_URL", "http://127.0.0.1:8080")
	t.Setenv("ACTORDOCK_API_KEY", "secret")
	t.Setenv("ACTORDOCK_DASHBOARD_PROXY_PLATFORM", "false")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	if cfg.ListenAddr != ":4000" || cfg.PlatformURL != "http://127.0.0.1:8080" {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	if cfg.APIKey != "secret" {
		t.Fatalf("APIKey = %q", cfg.APIKey)
	}
	if cfg.ProxyPlatform {
		t.Fatal("ProxyPlatform = true, want false")
	}
}
