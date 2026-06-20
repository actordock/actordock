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
	"strings"
	"time"

	"connectrpc.com/connect"
	processv1 "github.com/actordock/actordock/pkg/envd/process"
	"github.com/actordock/actordock/pkg/envd/process/processv1connect"
)

const (
	HealthPath          = "/health"
	DefaultReadyTimeout = 60 * time.Second
	readyPollInterval   = 200 * time.Millisecond
)

// BackendAddrFunc resolves the host:port for a sandbox envd instance.
type BackendAddrFunc func(ctx context.Context) (string, error)

func normalizeBaseURL(baseURL string) string {
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return "http://" + baseURL
	}
	return baseURL
}

// probeProcess checks the envd process Connect API (same path as commands.run).
func probeProcess(ctx context.Context, baseURL string) error {
	client := processv1connect.NewProcessClient(newH2CHTTPClient(), normalizeBaseURL(baseURL))
	_, err := client.List(ctx, connect.NewRequest(&processv1.ListRequest{}))
	if err != nil {
		return fmt.Errorf("envd process list: %w", err)
	}
	return nil
}

// WaitForReady polls until the envd process Connect API responds.
func WaitForReady(ctx context.Context, baseURL string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = DefaultReadyTimeout
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		if err := probeProcess(ctx, baseURL); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for envd ready at %s: %w", normalizeBaseURL(baseURL), lastErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(readyPollInterval):
		}
	}
}

// WaitForBackendReady polls until backend resolves and envd process Connect API responds.
func WaitForBackendReady(ctx context.Context, backend BackendAddrFunc, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = DefaultReadyTimeout
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		addr, err := backend(ctx)
		if err != nil {
			lastErr = err
		} else if err := probeProcess(ctx, "http://"+addr); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for envd backend: %v", lastErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(readyPollInterval):
		}
	}
}
