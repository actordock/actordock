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
	"errors"
	"net/http"
	"strconv"
	"strings"
)

const (
	maxLogsV2Limit  = 1000
	maxLogSearchLen = 256
)

var errInvalidLogsQuery = errors.New("invalid logs query")

type sandboxLogResponse struct {
	Timestamp string `json:"timestamp"`
	Line      string `json:"line"`
}

type sandboxLogEntryResponse struct {
	Timestamp string            `json:"timestamp"`
	Message   string            `json:"message"`
	Level     string            `json:"level"`
	Fields    map[string]string `json:"fields"`
}

type sandboxLogsResponse struct {
	Logs       []sandboxLogResponse      `json:"logs"`
	LogEntries []sandboxLogEntryResponse `json:"logEntries"`
}

type sandboxLogsV2Response struct {
	Logs []sandboxLogEntryResponse `json:"logs"`
}

func buildStubSandboxLogs() sandboxLogsResponse {
	return sandboxLogsResponse{
		Logs:       []sandboxLogResponse{},
		LogEntries: []sandboxLogEntryResponse{},
	}
}

func buildStubSandboxLogsV2() sandboxLogsV2Response {
	return sandboxLogsV2Response{
		Logs: []sandboxLogEntryResponse{},
	}
}

func parseLogsV1Query(r *http.Request) error {
	q := r.URL.Query()
	if raw := strings.TrimSpace(q.Get("start")); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			return errInvalidLogsQuery
		}
	}
	if err := parseLogsLimit(q.Get("limit"), 0); err != nil {
		return err
	}
	return nil
}

func parseLogsV2Query(r *http.Request) error {
	q := r.URL.Query()
	if raw := strings.TrimSpace(q.Get("cursor")); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			return errInvalidLogsQuery
		}
	}
	if err := parseLogsLimit(q.Get("limit"), maxLogsV2Limit); err != nil {
		return err
	}
	if raw := strings.TrimSpace(q.Get("direction")); raw != "" {
		switch raw {
		case "forward", "backward":
		default:
			return errInvalidLogsQuery
		}
	}
	if raw := strings.TrimSpace(q.Get("level")); raw != "" {
		switch raw {
		case "debug", "info", "warn", "error":
		default:
			return errInvalidLogsQuery
		}
	}
	if search := q.Get("search"); len(search) > maxLogSearchLen {
		return errInvalidLogsQuery
	}
	return nil
}

func parseLogsLimit(raw string, maxLimit int) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || v < 0 {
		return errInvalidLogsQuery
	}
	if maxLimit > 0 && int(v) > maxLimit {
		return errInvalidLogsQuery
	}
	return nil
}
