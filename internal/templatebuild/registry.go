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

package templatebuild

import (
	"net"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
)

// RewriteLocalRegistry rewrites localhost/loopback registry refs for in-cluster pulls.
// replacement is typically kind-registry:5000 when KO_DOCKER_REPO is localhost:5001.
func RewriteLocalRegistry(ref, replacement string) string {
	replacement = strings.TrimSpace(replacement)
	if replacement == "" || !isLocalRegistryRef(ref) {
		return ref
	}
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) < 2 {
		return ref
	}
	return replacement + "/" + parts[1]
}

func isLocalRegistryRef(ref string) bool {
	return isLocalhostOrLoopback(registryHost(ref))
}

func registryHost(ref string) string {
	parts := strings.SplitN(ref, "/", 2)
	reg, err := name.NewRegistry(parts[0], name.Insecure)
	if err != nil {
		return ""
	}
	hostPart := reg.Name()
	if h, _, err := net.SplitHostPort(hostPart); err == nil {
		return h
	}
	return hostPart
}

func isLocalhostOrLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}

// RewriteRegistryPrefix swaps the registry host prefix in an image reference.
func RewriteRegistryPrefix(ref, fromPrefix, toPrefix string) string {
	fromPrefix = strings.TrimRight(strings.TrimSpace(fromPrefix), "/")
	toPrefix = strings.TrimRight(strings.TrimSpace(toPrefix), "/")
	if fromPrefix == "" || toPrefix == "" || fromPrefix == toPrefix {
		return ref
	}
	needle := fromPrefix + "/"
	if strings.HasPrefix(ref, needle) {
		return toPrefix + ref[len(fromPrefix):]
	}
	return ref
}
