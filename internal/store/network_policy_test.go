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

package store

import "testing"

func TestInternetAccessAllowed(t *testing.T) {
	t.Parallel()
	if !InternetAccessAllowed(Sandbox{}) {
		t.Fatal("default should allow internet")
	}
	disabled := false
	if InternetAccessAllowed(Sandbox{AllowInternetAccess: &disabled}) {
		t.Fatal("explicit false should deny internet")
	}
	enabled := true
	if !InternetAccessAllowed(Sandbox{AllowInternetAccess: &enabled}) {
		t.Fatal("explicit true should allow internet")
	}
	if InternetAccessAllowed(Sandbox{
		Network: &NetworkConfig{DenyOut: []string{denyAllIPv4}},
	}) {
		t.Fatal("deny all ipv4 should deny internet")
	}
}

func TestIsInternalHost(t *testing.T) {
	t.Parallel()
	cases := []struct {
		host string
		want bool
	}{
		{host: "127.0.0.1", want: true},
		{host: "127.0.0.1:8080", want: true},
		{host: "10.0.0.5", want: true},
		{host: "192.168.1.2", want: true},
		{host: "redis.actordock.svc.cluster.local", want: true},
		{host: "example.com", want: false},
		{host: "8.8.8.8", want: false},
	}
	for _, tc := range cases {
		if got := IsInternalHost(tc.host); got != tc.want {
			t.Fatalf("IsInternalHost(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}
