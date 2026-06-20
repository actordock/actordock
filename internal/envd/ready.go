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

package envd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	HealthPath          = "/health"
	DefaultReadyTimeout = 60 * time.Second
	readyPollInterval   = 200 * time.Millisecond
)

// ProbeHealth checks GET /health once.
func ProbeHealth(ctx context.Context, client *http.Client, baseURL string) error {
	if client == nil {
		client = http.DefaultClient
	}
	healthURL := strings.TrimRight(baseURL, "/") + HealthPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return fmt.Errorf("envd health at %s returned %d", healthURL, resp.StatusCode)
}

// WaitForHealth polls GET /health on baseURL until envd responds with 204 No Content.
func WaitForHealth(ctx context.Context, client *http.Client, baseURL string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = DefaultReadyTimeout
	}
	deadline := time.Now().Add(timeout)
	for {
		if err := ProbeHealth(ctx, client, baseURL); err == nil {
			return nil
		} else if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for envd health at %s: %w", strings.TrimRight(baseURL, "/")+HealthPath, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(readyPollInterval):
		}
	}
}
