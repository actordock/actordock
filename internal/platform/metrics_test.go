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

package platform

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseSandboxIDs(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/metrics?sandbox_ids=a,b,c", nil)
	ids, err := parseSandboxIDs(req)
	if err != nil {
		t.Fatalf("parseSandboxIDs() = %v, want nil", err)
	}
	if len(ids) != 3 || ids[0] != "a" || ids[1] != "b" || ids[2] != "c" {
		t.Fatalf("ids = %v", ids)
	}

	dupReq := httptest.NewRequest(http.MethodGet, "/sandboxes/metrics?sandbox_ids=a,a,b", nil)
	dupIDs, err := parseSandboxIDs(dupReq)
	if err != nil {
		t.Fatalf("parseSandboxIDs(dup) = %v, want nil", err)
	}
	if len(dupIDs) != 2 || dupIDs[0] != "a" || dupIDs[1] != "b" {
		t.Fatalf("dup ids = %v", dupIDs)
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/sandboxes/metrics", nil)
	if _, err := parseSandboxIDs(missingReq); err == nil {
		t.Fatal("parseSandboxIDs(missing) = nil, want error")
	}

	tooMany := make([]string, maxSandboxMetricsIDs+1)
	for i := range tooMany {
		tooMany[i] = "sb-" + strconv.Itoa(i)
	}
	tooManyReq := httptest.NewRequest(http.MethodGet, "/sandboxes/metrics?sandbox_ids="+strings.Join(tooMany, ","), nil)
	if _, err := parseSandboxIDs(tooManyReq); err == nil {
		t.Fatal("parseSandboxIDs(too many) = nil, want error")
	}
}

func TestBuildStubSandboxMetric(t *testing.T) {
	t.Parallel()

	m := buildStubSandboxMetric(mustTime("2026-06-20T12:00:00Z"))
	if m.Timestamp != "2026-06-20T12:00:00Z" {
		t.Fatalf("timestamp = %q", m.Timestamp)
	}
	if m.TimestampUnix != mustTime("2026-06-20T12:00:00Z").Unix() {
		t.Fatalf("timestampUnix = %d", m.TimestampUnix)
	}
	if m.CPUCount != defaultCPUCount {
		t.Fatalf("cpuCount = %d", m.CPUCount)
	}
	if m.MemTotal != int64(defaultMemoryMB)*1024*1024 {
		t.Fatalf("memTotal = %d", m.MemTotal)
	}
	if m.DiskTotal != int64(defaultDiskSizeMB)*1024*1024 {
		t.Fatalf("diskTotal = %d", m.DiskTotal)
	}
}

func mustTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return t
}
