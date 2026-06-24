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
)

type Router struct {
	Server
	RuntimeAPIAddr string
	RedisAddr      string
	Domain         string
	EnvdPort       int
}

func RouterFromEnv() (Router, error) {
	server, err := ServerFromEnv("router", ":8081")
	if err != nil {
		return Router{}, err
	}
	envdPort, err := strconv.Atoi(envOrDefault("ACTORDOCK_ENVD_PORT", "80"))
	if err != nil || envdPort <= 0 || envdPort > 65535 {
		return Router{}, fmt.Errorf("ACTORDOCK_ENVD_PORT must be a valid port")
	}
	return Router{
		Server:         server,
		RuntimeAPIAddr: envOrDefault("ACTORDOCK_RUNTIME_API_ADDR", "api.actordock-system.svc:443"),
		RedisAddr:      envOrDefault("ACTORDOCK_REDIS_ADDR", "redis.actordock.svc:6379"),
		Domain:         envOrDefault("ACTORDOCK_DOMAIN", "localhost"),
		EnvdPort:       envdPort,
	}, nil
}
