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
	"encoding/json"
	"fmt"
	"strings"
)

// NetworkConfig is persisted sandbox network settings (OpenAPI SandboxNetworkConfig).
type NetworkConfig struct {
	AllowPublicTraffic bool                     `json:"allowPublicTraffic,omitempty"`
	AllowOut           []string                 `json:"allowOut,omitempty"`
	DenyOut            []string                 `json:"denyOut,omitempty"`
	MaskRequestHost    string                   `json:"maskRequestHost,omitempty"`
	Rules              map[string][]NetworkRule `json:"rules,omitempty"`
}

// NetworkRule is a per-domain egress transform rule.
type NetworkRule struct {
	Transform *NetworkTransform `json:"transform,omitempty"`
}

// NetworkTransform holds header overrides for matching egress requests.
type NetworkTransform struct {
	Headers map[string]string `json:"headers,omitempty"`
}

// NetworkUpdate is a parsed PUT /sandboxes/{id}/network body.
// Omitted keys clear the corresponding stored fields.
type NetworkUpdate struct {
	AllowOut            []string
	DenyOut             []string
	Rules               map[string][]NetworkRule
	AllowInternetAccess *bool

	SetAllowOut      bool
	SetDenyOut       bool
	SetRules         bool
	SetAllowInternet bool
}

// ParseNetworkUpdate decodes SandboxNetworkUpdateConfig from JSON.
func ParseNetworkUpdate(body []byte) (NetworkUpdate, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return NetworkUpdate{}, fmt.Errorf("invalid JSON body: %w", err)
	}

	upd := NetworkUpdate{}
	var err error
	if v, ok := raw["allowOut"]; ok {
		upd.SetAllowOut = true
		upd.AllowOut, err = decodeStringList(v, "allowOut")
		if err != nil {
			return NetworkUpdate{}, err
		}
	}
	if v, ok := raw["denyOut"]; ok {
		upd.SetDenyOut = true
		upd.DenyOut, err = decodeStringList(v, "denyOut")
		if err != nil {
			return NetworkUpdate{}, err
		}
	}
	if v, ok := raw["rules"]; ok {
		upd.SetRules = true
		upd.Rules, err = decodeNetworkRules(v)
		if err != nil {
			return NetworkUpdate{}, err
		}
	}
	if v, ok := raw["allow_internet_access"]; ok {
		upd.SetAllowInternet = true
		upd.AllowInternetAccess, err = decodeOptionalBool(v, "allow_internet_access")
		if err != nil {
			return NetworkUpdate{}, err
		}
	}
	return upd, nil
}

// ValidateNetworkConfig checks a full SandboxNetworkConfig payload.
func ValidateNetworkConfig(nc *NetworkConfig) error {
	if nc == nil {
		return nil
	}
	if err := validateEgressList("allowOut", nc.AllowOut, false); err != nil {
		return err
	}
	if err := validateEgressList("denyOut", nc.DenyOut, true); err != nil {
		return err
	}
	for domain, rules := range nc.Rules {
		if strings.TrimSpace(domain) == "" {
			return fmt.Errorf("rules: domain key must be non-empty")
		}
		for i, rule := range rules {
			if rule.Transform == nil {
				return fmt.Errorf("rules[%q][%d]: transform is required", domain, i)
			}
		}
	}
	return nil
}

// NormalizeNetwork returns nil when nc is empty.
func NormalizeNetwork(nc *NetworkConfig) *NetworkConfig {
	if nc == nil || networkConfigEmpty(nc) {
		return nil
	}
	return nc
}

// ValidateNetworkUpdate checks update payload constraints.
func ValidateNetworkUpdate(upd NetworkUpdate) error {
	if upd.SetAllowOut {
		if err := validateEgressList("allowOut", upd.AllowOut, false); err != nil {
			return err
		}
	}
	if upd.SetDenyOut {
		if err := validateEgressList("denyOut", upd.DenyOut, true); err != nil {
			return err
		}
	}
	if upd.SetRules {
		for domain, rules := range upd.Rules {
			if strings.TrimSpace(domain) == "" {
				return fmt.Errorf("rules: domain key must be non-empty")
			}
			for i, rule := range rules {
				if rule.Transform == nil {
					return fmt.Errorf("rules[%q][%d]: transform is required", domain, i)
				}
			}
		}
	}
	return nil
}

// ApplyNetworkUpdate replaces egress-related fields on sb per E2B semantics.
func ApplyNetworkUpdate(sb *Sandbox, upd NetworkUpdate) {
	var nc *NetworkConfig
	if sb.Network != nil {
		copied := *sb.Network
		nc = &copied
	} else {
		nc = &NetworkConfig{}
	}

	if upd.SetAllowOut {
		nc.AllowOut = append([]string(nil), upd.AllowOut...)
	} else {
		nc.AllowOut = nil
	}
	if upd.SetDenyOut {
		nc.DenyOut = append([]string(nil), upd.DenyOut...)
	} else {
		nc.DenyOut = nil
	}
	if upd.SetRules {
		if len(upd.Rules) == 0 {
			nc.Rules = nil
		} else {
			nc.Rules = cloneNetworkRules(upd.Rules)
		}
	} else {
		nc.Rules = nil
	}

	if networkConfigEmpty(nc) {
		sb.Network = nil
	} else {
		sb.Network = nc
	}

	if upd.SetAllowInternet {
		sb.AllowInternetAccess = upd.AllowInternetAccess
	} else {
		sb.AllowInternetAccess = nil
	}
}

func decodeStringList(raw json.RawMessage, field string) ([]string, error) {
	if string(raw) == "null" {
		return nil, nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("%s: must be an array of strings", field)
	}
	for i, item := range list {
		if strings.TrimSpace(item) == "" {
			return nil, fmt.Errorf("%s[%d]: must be a non-empty string", field, i)
		}
	}
	return list, nil
}

func decodeOptionalBool(raw json.RawMessage, field string) (*bool, error) {
	if string(raw) == "null" {
		return nil, nil
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("%s: must be a boolean", field)
	}
	return &value, nil
}

func decodeNetworkRules(raw json.RawMessage) (map[string][]NetworkRule, error) {
	if string(raw) == "null" {
		return nil, nil
	}
	var rules map[string][]NetworkRule
	if err := json.Unmarshal(raw, &rules); err != nil {
		return nil, fmt.Errorf("rules: must be an object")
	}
	if rules == nil {
		return nil, nil
	}
	return rules, nil
}

func validateEgressList(field string, list []string, denyOnly bool) error {
	if list == nil {
		return nil
	}
	for i, item := range list {
		if strings.TrimSpace(item) == "" {
			return fmt.Errorf("%s[%d]: must be a non-empty string", field, i)
		}
		if denyOnly && looksLikeDomain(item) {
			return fmt.Errorf("%s[%d]: domain names are not supported for deny rules", field, i)
		}
	}
	return nil
}

func looksLikeDomain(value string) bool {
	if strings.Contains(value, "/") {
		return false
	}
	return strings.ContainsAny(value, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
}

func networkConfigEmpty(nc *NetworkConfig) bool {
	if nc == nil {
		return true
	}
	return !nc.AllowPublicTraffic &&
		len(nc.AllowOut) == 0 &&
		len(nc.DenyOut) == 0 &&
		nc.MaskRequestHost == "" &&
		len(nc.Rules) == 0
}

func cloneNetworkRules(rules map[string][]NetworkRule) map[string][]NetworkRule {
	out := make(map[string][]NetworkRule, len(rules))
	for domain, domainRules := range rules {
		copied := make([]NetworkRule, len(domainRules))
		for i, rule := range domainRules {
			copied[i] = rule
			if rule.Transform != nil && rule.Transform.Headers != nil {
				headers := make(map[string]string, len(rule.Transform.Headers))
				for k, v := range rule.Transform.Headers {
					headers[k] = v
				}
				if copied[i].Transform == nil {
					copied[i].Transform = &NetworkTransform{}
				}
				copied[i].Transform.Headers = headers
			}
		}
		out[domain] = copied
	}
	return out
}
