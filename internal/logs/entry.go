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

package logs

// Entry is a structured sandbox log line (E2B SandboxLogEntry).
type Entry struct {
	Timestamp string            `json:"timestamp"`
	Message   string            `json:"message"`
	Level     string            `json:"level"`
	Fields    map[string]string `json:"fields"`
}

// Response is the envd internal GET /logs payload.
type Response struct {
	Logs []Entry `json:"logs"`
}

// V1Response maps entries to E2B SandboxLogs (logs + logEntries).
type V1Response struct {
	Logs       []LineEntry `json:"logs"`
	LogEntries []Entry     `json:"logEntries"`
}

// LineEntry is the v1 SandboxLog shape ({line}).
type LineEntry struct {
	Timestamp string `json:"timestamp"`
	Line      string `json:"line"`
}

// V2Response maps entries to E2B SandboxLogsV2Response.
type V2Response struct {
	Logs []Entry `json:"logs"`
}

// ToV1 converts structured entries to Platform v1 response fields.
func ToV1(entries []Entry) V1Response {
	if len(entries) == 0 {
		return EmptyV1()
	}
	lines := make([]LineEntry, len(entries))
	for i, e := range entries {
		lines[i] = LineEntry{Timestamp: e.Timestamp, Line: e.Message}
	}
	return V1Response{Logs: lines, LogEntries: entries}
}

// ToV2 converts structured entries to Platform v2 response fields.
func ToV2(entries []Entry) V2Response {
	if len(entries) == 0 {
		return EmptyV2()
	}
	return V2Response{Logs: entries}
}

// EmptyV1 returns empty v1 log response slices (not nil).
func EmptyV1() V1Response {
	return V1Response{Logs: []LineEntry{}, LogEntries: []Entry{}}
}

// EmptyV2 returns empty v2 log response slices (not nil).
func EmptyV2() V2Response {
	return V2Response{Logs: []Entry{}}
}
