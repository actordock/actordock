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
	"fmt"
	"net/url"
	"os"
	"strconv"
)

type Config struct {
	ListenAddr    string
	PlatformURL   string
	RouterURL     string
	APIKey        string
	ProxyPlatform bool
	ProxyRouter   bool
	LogLevel      string
}

func ConfigFromEnv() (Config, error) {
	proxyPlatform, err := envBoolOrDefault("ACTORDOCK_DASHBOARD_PROXY_PLATFORM", true)
	if err != nil {
		return Config{}, fmt.Errorf("ACTORDOCK_DASHBOARD_PROXY_PLATFORM: %w", err)
	}
	proxyRouter, err := envBoolOrDefault("ACTORDOCK_DASHBOARD_PROXY_ROUTER", true)
	if err != nil {
		return Config{}, fmt.Errorf("ACTORDOCK_DASHBOARD_PROXY_ROUTER: %w", err)
	}

	cfg := Config{
		ListenAddr:    envOrDefault("ACTORDOCK_DASHBOARD_ADDR", ":3000"),
		PlatformURL:   envOrDefault("ACTORDOCK_PLATFORM_URL", "http://platform:8080"),
		RouterURL:     envOrDefault("ACTORDOCK_ROUTER_URL", "http://localhost:8081"),
		APIKey:        envOrDefault("ACTORDOCK_API_KEY", "dev"),
		ProxyPlatform: proxyPlatform,
		ProxyRouter:   proxyRouter,
		LogLevel:      envOrDefault("ACTORDOCK_DASHBOARD_LOG_LEVEL", envOrDefault("ACTORDOCK_LOG_LEVEL", "info")),
	}
	if cfg.ListenAddr == "" {
		return Config{}, fmt.Errorf("ACTORDOCK_DASHBOARD_ADDR is required")
	}
	if _, err := url.Parse(cfg.PlatformURL); err != nil {
		return Config{}, fmt.Errorf("ACTORDOCK_PLATFORM_URL: %w", err)
	}
	if _, err := url.Parse(cfg.RouterURL); err != nil {
		return Config{}, fmt.Errorf("ACTORDOCK_ROUTER_URL: %w", err)
	}
	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBoolOrDefault(key string, fallback bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("parse %q: %w", v, err)
	}
	return b, nil
}
