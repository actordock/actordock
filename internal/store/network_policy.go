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
	"net"
	"strings"
)

const (
	denyAllIPv4 = "0.0.0.0/0"
	denyAllIPv6 = "::/0"
)

// InternetAccessAllowed reports whether sandbox egress to the public internet is permitted.
// Unset allowInternetAccess defaults to allowed unless denyOut blocks all traffic.
func InternetAccessAllowed(sb Sandbox) bool {
	if sb.AllowInternetAccess != nil {
		return *sb.AllowInternetAccess
	}
	if sb.Network != nil {
		for _, deny := range sb.Network.DenyOut {
			if deny == denyAllIPv4 || deny == denyAllIPv6 {
				return false
			}
		}
	}
	return true
}

// IsInternalHost reports whether host is loopback, RFC1918, link-local, or cluster-local.
func IsInternalHost(host string) bool {
	host = stripHostPort(host)
	if host == "" {
		return false
	}
	if host == "localhost" || strings.HasSuffix(host, ".cluster.local") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func stripHostPort(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}
	return host
}
