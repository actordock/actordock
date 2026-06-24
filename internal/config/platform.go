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
	"strconv"

	"github.com/actordock/actordock/internal/store"
)

type Platform struct {
	Server
	APIKey                string
	RuntimeAPIAddr            string
	RedisAddr             string
	Domain                string
	TemplateNamespace     string
	TemplateName          string
	EnvdVersion           string
	EnvdPort              int
	ClientID              string
	DefaultSandboxTimeout int
	VolumeRoot            string
}

func PlatformFromEnv() (Platform, error) {
	server, err := ServerFromEnv("platform", ":8080")
	if err != nil {
		return Platform{}, err
	}
	cfg := Platform{
		Server:            server,
		APIKey:            envOrDefault("ACTORDOCK_API_KEY", "dev"),
		RuntimeAPIAddr:        envOrDefault("ACTORDOCK_RUNTIME_API_ADDR", "api.actordock-system.svc:443"),
		RedisAddr:         envOrDefault("ACTORDOCK_REDIS_ADDR", "redis.actordock.svc:6379"),
		Domain:            envOrDefault("ACTORDOCK_DOMAIN", "localhost"),
		TemplateNamespace: envOrDefault("ACTORDOCK_TEMPLATE_NAMESPACE", "actordock"),
		TemplateName:      envOrDefault("ACTORDOCK_TEMPLATE_NAME", "base"),
		EnvdVersion:       envOrDefault("ACTORDOCK_ENVD_VERSION", "0.1.0"),
		ClientID:          envOrDefault("ACTORDOCK_CLIENT_ID", "actordock"),
		VolumeRoot:        envOrDefault("ACTORDOCK_VOLUME_ROOT", "/var/lib/actordock/volumes"),
	}
	defaultTimeout, err := envIntOrDefault("ACTORDOCK_DEFAULT_SANDBOX_TIMEOUT", 300)
	if err != nil {
		return Platform{}, fmt.Errorf("ACTORDOCK_DEFAULT_SANDBOX_TIMEOUT: %w", err)
	}
	cfg.DefaultSandboxTimeout = defaultTimeout
	if err := store.ValidateTimeout(cfg.DefaultSandboxTimeout); err != nil {
		return Platform{}, fmt.Errorf("ACTORDOCK_DEFAULT_SANDBOX_TIMEOUT: %w", err)
	}
	envdPort, err := strconv.Atoi(envOrDefault("ACTORDOCK_ENVD_PORT", "80"))
	if err != nil || envdPort <= 0 || envdPort > 65535 {
		return Platform{}, fmt.Errorf("ACTORDOCK_ENVD_PORT must be a valid port")
	}
	cfg.EnvdPort = envdPort
	if cfg.TemplateName == "" {
		return Platform{}, fmt.Errorf("ACTORDOCK_TEMPLATE_NAME is required")
	}
	return cfg, nil
}
