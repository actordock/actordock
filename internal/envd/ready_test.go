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

func TestWaitForReadyImmediate(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(NewStubHandler())
	t.Cleanup(srv.Close)

	if err := WaitForReady(context.Background(), srv.URL, time.Second); err != nil {
		t.Fatalf("WaitForReady() = %v, want nil", err)
	}
}

func TestWaitForReadyEventually(t *testing.T) {
	t.Parallel()
	var ready atomic.Bool
	stub := NewStubHandler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		stub.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	go func() {
		time.Sleep(300 * time.Millisecond)
		ready.Store(true)
	}()

	if err := WaitForReady(context.Background(), srv.URL, 2*time.Second); err != nil {
		t.Fatalf("WaitForReady() = %v, want nil", err)
	}
}

func TestWaitForReadyTimeout(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	err := WaitForReady(context.Background(), srv.URL, 200*time.Millisecond)
	if err == nil {
		t.Fatal("WaitForReady() = nil, want error")
	}
}

func TestWaitForBackendReady(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(NewStubHandler())
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().String()

	err := WaitForBackendReady(context.Background(), func(context.Context) (string, error) {
		return addr, nil
	}, time.Second)
	if err != nil {
		t.Fatalf("WaitForBackendReady() = %v, want nil", err)
	}
}
