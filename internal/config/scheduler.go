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
	"time"
)

const defaultSchedulerPollInterval = 5 * time.Second

type Scheduler struct {
	LogLevel     string
	RedisAddr    string
	RuntimeAPIAddr   string
	PollInterval time.Duration
}

func SchedulerFromEnv() (Scheduler, error) {
	pollInterval, err := envDurationOrDefault("SCHEDULER_POLL_INTERVAL", defaultSchedulerPollInterval)
	if err != nil {
		return Scheduler{}, fmt.Errorf("SCHEDULER_POLL_INTERVAL: %w", err)
	}
	if pollInterval <= 0 {
		return Scheduler{}, fmt.Errorf("SCHEDULER_POLL_INTERVAL must be positive")
	}
	return Scheduler{
		LogLevel:     envOrDefault("SCHEDULER_LOG_LEVEL", envOrDefault("ACTORDOCK_LOG_LEVEL", "info")),
		RedisAddr:    envOrDefault("SCHEDULER_REDIS_ADDR", "redis.actordock.svc:6379"),
		RuntimeAPIAddr:   envOrDefault("SCHEDULER_RUNTIME_API_ADDR", "api.actordock-system.svc:443"),
		PollInterval: pollInterval,
	}, nil
}

func envDurationOrDefault(key string, fallback time.Duration) (time.Duration, error) {
	v := envOrDefault(key, "")
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", v, err)
	}
	return d, nil
}
