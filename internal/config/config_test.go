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

func TestPlatformFromEnvRedisAddr(t *testing.T) {
	t.Setenv("PLATFORM_LISTEN_ADDR", "")
	t.Setenv("ACTORDOCK_REDIS_ADDR", "")

	cfg, err := PlatformFromEnv()
	if err != nil {
		t.Fatalf("PlatformFromEnv: %v", err)
	}
	if cfg.RedisAddr != "redis.actordock.svc:6379" {
		t.Fatalf("RedisAddr = %q, want redis.actordock.svc:6379", cfg.RedisAddr)
	}

	t.Setenv("ACTORDOCK_REDIS_ADDR", "redis:6379")
	cfg, err = PlatformFromEnv()
	if err != nil {
		t.Fatalf("PlatformFromEnv: %v", err)
	}
	if cfg.RedisAddr != "redis:6379" {
		t.Fatalf("RedisAddr = %q, want redis:6379", cfg.RedisAddr)
	}
}

func TestPlatformFromEnvDefaultSandboxTimeout(t *testing.T) {
	t.Setenv("PLATFORM_LISTEN_ADDR", "")
	t.Setenv("ACTORDOCK_REDIS_ADDR", "")
	t.Setenv("ACTORDOCK_DEFAULT_SANDBOX_TIMEOUT", "")

	cfg, err := PlatformFromEnv()
	if err != nil {
		t.Fatalf("PlatformFromEnv: %v", err)
	}
	if cfg.DefaultSandboxTimeout != 300 {
		t.Fatalf("DefaultSandboxTimeout = %d, want 300", cfg.DefaultSandboxTimeout)
	}

	t.Setenv("ACTORDOCK_DEFAULT_SANDBOX_TIMEOUT", "600")
	cfg, err = PlatformFromEnv()
	if err != nil {
		t.Fatalf("PlatformFromEnv: %v", err)
	}
	if cfg.DefaultSandboxTimeout != 600 {
		t.Fatalf("DefaultSandboxTimeout = %d, want 600", cfg.DefaultSandboxTimeout)
	}

	t.Setenv("ACTORDOCK_DEFAULT_SANDBOX_TIMEOUT", "bad")
	if _, err := PlatformFromEnv(); err == nil {
		t.Fatal("expected error for invalid ACTORDOCK_DEFAULT_SANDBOX_TIMEOUT")
	}
}
