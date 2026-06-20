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

import (
	"testing"
)

func TestParseNetworkUpdateClearsOmittedFields(t *testing.T) {
	t.Parallel()
	upd, err := ParseNetworkUpdate([]byte(`{}`))
	if err != nil {
		t.Fatalf("ParseNetworkUpdate: %v", err)
	}
	if upd.SetAllowOut || upd.SetDenyOut || upd.SetRules || upd.SetAllowInternet {
		t.Fatalf("empty body should not set any fields: %+v", upd)
	}

	sb := Sandbox{
		Network: &NetworkConfig{
			AllowOut: []string{"1.1.1.1"},
			DenyOut:  []string{"8.8.8.8"},
		},
		AllowInternetAccess: boolPtr(false),
	}
	ApplyNetworkUpdate(&sb, upd)
	if sb.Network != nil {
		t.Fatalf("network = %+v, want nil", sb.Network)
	}
	if sb.AllowInternetAccess != nil {
		t.Fatalf("allowInternetAccess = %v, want nil", *sb.AllowInternetAccess)
	}
}

func TestApplyNetworkUpdateRoundTrip(t *testing.T) {
	t.Parallel()
	body := []byte(`{
		"allowOut":["1.1.1.1","example.com"],
		"denyOut":["8.8.8.8"],
		"allow_internet_access":false,
		"rules":{"api.example.com":[{"transform":{"headers":{"X-Test":"1"}}}]}
	}`)
	upd, err := ParseNetworkUpdate(body)
	if err != nil {
		t.Fatalf("ParseNetworkUpdate: %v", err)
	}
	if err := ValidateNetworkUpdate(upd); err != nil {
		t.Fatalf("ValidateNetworkUpdate: %v", err)
	}

	sb := Sandbox{SandboxID: "sb-1"}
	ApplyNetworkUpdate(&sb, upd)

	if len(sb.Network.AllowOut) != 2 || sb.Network.AllowOut[0] != "1.1.1.1" {
		t.Fatalf("allowOut = %v", sb.Network.AllowOut)
	}
	if len(sb.Network.DenyOut) != 1 || sb.Network.DenyOut[0] != "8.8.8.8" {
		t.Fatalf("denyOut = %v", sb.Network.DenyOut)
	}
	if sb.AllowInternetAccess == nil || *sb.AllowInternetAccess {
		t.Fatalf("allowInternetAccess = %v, want false", sb.AllowInternetAccess)
	}
	rules := sb.Network.Rules["api.example.com"]
	if len(rules) != 1 || rules[0].Transform.Headers["X-Test"] != "1" {
		t.Fatalf("rules = %+v", sb.Network.Rules)
	}
}

func TestValidateNetworkUpdateDenyDomain(t *testing.T) {
	t.Parallel()
	upd := NetworkUpdate{
		SetDenyOut: true,
		DenyOut:    []string{"example.com"},
	}
	if err := ValidateNetworkUpdate(upd); err == nil {
		t.Fatal("expected denyOut domain validation error")
	}
}

func boolPtr(v bool) *bool { return &v }
