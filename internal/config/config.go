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
	"fmt"
	"os"
	"strconv"
)

type Server struct {
	ListenAddr string
	LogLevel   string
}

func ServerFromEnv(service string, defaultAddr string) (Server, error) {
	prefix := envKey(service)
	cfg := Server{
		ListenAddr: envOrDefault(prefix+"_LISTEN_ADDR", defaultAddr),
		LogLevel:   envOrDefault(prefix+"_LOG_LEVEL", envOrDefault("ACTORDOCK_LOG_LEVEL", "info")),
	}
	if cfg.ListenAddr == "" {
		return Server{}, fmt.Errorf("%s listen address is required", service)
	}
	return cfg, nil
}

func envKey(service string) string {
	switch service {
	case "platform":
		return "PLATFORM"
	case "router":
		return "ROUTER"
	case "envd":
		return "ENVD"
	default:
		return "ACTORDOCK"
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", v, err)
	}
	return n, nil
}
