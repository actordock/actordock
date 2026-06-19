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

	"github.com/actordock/actordock/internal/store"
)

type Platform struct {
	Server
	APIKey                string
	ATEAPIAddr            string
	RedisAddr             string
	Domain                string
	TemplateNamespace     string
	TemplateName          string
	EnvdVersion           string
	ClientID              string
	DefaultSandboxTimeout int
}

func PlatformFromEnv() (Platform, error) {
	server, err := ServerFromEnv("platform", ":8080")
	if err != nil {
		return Platform{}, err
	}
	cfg := Platform{
		Server:            server,
		APIKey:            envOrDefault("ACTORDOCK_API_KEY", "dev"),
		ATEAPIAddr:        envOrDefault("ACTORDOCK_ATEAPI_ADDR", "api.ate-system.svc:443"),
		RedisAddr:         envOrDefault("ACTORDOCK_REDIS_ADDR", "redis.actordock.svc:6379"),
		Domain:            envOrDefault("ACTORDOCK_DOMAIN", "localhost"),
		TemplateNamespace: envOrDefault("ACTORDOCK_TEMPLATE_NAMESPACE", "actordock"),
		TemplateName:      envOrDefault("ACTORDOCK_TEMPLATE_NAME", "base"),
		EnvdVersion:       envOrDefault("ACTORDOCK_ENVD_VERSION", "0.1.0"),
		ClientID:          envOrDefault("ACTORDOCK_CLIENT_ID", "actordock"),
	}
	defaultTimeout, err := envIntOrDefault("ACTORDOCK_DEFAULT_SANDBOX_TIMEOUT", 300)
	if err != nil {
		return Platform{}, fmt.Errorf("ACTORDOCK_DEFAULT_SANDBOX_TIMEOUT: %w", err)
	}
	cfg.DefaultSandboxTimeout = defaultTimeout
	if err := store.ValidateTimeout(cfg.DefaultSandboxTimeout); err != nil {
		return Platform{}, fmt.Errorf("ACTORDOCK_DEFAULT_SANDBOX_TIMEOUT: %w", err)
	}
	if cfg.TemplateName == "" {
		return Platform{}, fmt.Errorf("ACTORDOCK_TEMPLATE_NAME is required")
	}
	return cfg, nil
}
