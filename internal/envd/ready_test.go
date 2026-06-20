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
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestProbeHealthImmediate(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != HealthPath {
			t.Fatalf("path = %q, want %q", r.URL.Path, HealthPath)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	if err := ProbeHealth(context.Background(), nil, srv.URL); err != nil {
		t.Fatalf("ProbeHealth() = %v, want nil", err)
	}
}

func TestWaitForHealthImmediate(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != HealthPath {
			t.Fatalf("path = %q, want %q", r.URL.Path, HealthPath)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	if err := WaitForHealth(context.Background(), nil, srv.URL, time.Second); err != nil {
		t.Fatalf("WaitForHealth() = %v, want nil", err)
	}
}

func TestWaitForHealthEventually(t *testing.T) {
	t.Parallel()
	var ready atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != HealthPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if ready.Load() {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	go func() {
		time.Sleep(300 * time.Millisecond)
		ready.Store(true)
	}()

	if err := WaitForHealth(context.Background(), nil, srv.URL, 2*time.Second); err != nil {
		t.Fatalf("WaitForHealth() = %v, want nil", err)
	}
}

func TestWaitForHealthTimeout(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	err := WaitForHealth(context.Background(), nil, srv.URL, 200*time.Millisecond)
	if err == nil {
		t.Fatal("WaitForHealth() = nil, want error")
	}
}
