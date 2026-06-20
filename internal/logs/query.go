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

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
)

const (
	MaxV2Limit      = 1000
	MaxLogSearchLen = 256
)

var ErrInvalidQuery = errors.New("invalid logs query")

// Query holds parsed log filter parameters shared by Platform and envd.
type Query struct {
	Start     *int64
	Limit     int
	Cursor    *int64
	Direction string
	Level     string
	Search    string
}

// ValidateV1Query checks GET /sandboxes/{id}/logs query params.
func ValidateV1Query(r *http.Request) error {
	q := r.URL.Query()
	if raw := strings.TrimSpace(q.Get("start")); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			return ErrInvalidQuery
		}
	}
	return parseLimit(q.Get("limit"), 0)
}

// ValidateV2Query checks GET /v2/sandboxes/{id}/logs query params.
func ValidateV2Query(r *http.Request) error {
	q := r.URL.Query()
	if raw := strings.TrimSpace(q.Get("cursor")); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			return ErrInvalidQuery
		}
	}
	if err := parseLimit(q.Get("limit"), MaxV2Limit); err != nil {
		return err
	}
	if raw := strings.TrimSpace(q.Get("direction")); raw != "" {
		switch raw {
		case "forward", "backward":
		default:
			return ErrInvalidQuery
		}
	}
	if raw := strings.TrimSpace(q.Get("level")); raw != "" {
		switch raw {
		case "debug", "info", "warn", "error":
		default:
			return ErrInvalidQuery
		}
	}
	if search := q.Get("search"); len(search) > MaxLogSearchLen {
		return ErrInvalidQuery
	}
	return nil
}

// ParseEnvdQuery validates and parses envd GET /logs query params (v1 + v2 union).
func ParseEnvdQuery(r *http.Request) (Query, error) {
	q := r.URL.Query()
	out := Query{
		Direction: strings.TrimSpace(q.Get("direction")),
		Level:     strings.TrimSpace(q.Get("level")),
		Search:    q.Get("search"),
	}

	if raw := strings.TrimSpace(q.Get("start")); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			return Query{}, ErrInvalidQuery
		}
		out.Start = &v
	}
	if raw := strings.TrimSpace(q.Get("cursor")); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			return Query{}, ErrInvalidQuery
		}
		out.Cursor = &v
	}
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || v < 0 {
			return Query{}, ErrInvalidQuery
		}
		if int(v) > MaxV2Limit {
			return Query{}, ErrInvalidQuery
		}
		out.Limit = int(v)
	}
	if out.Direction != "" {
		switch out.Direction {
		case "forward", "backward":
		default:
			return Query{}, ErrInvalidQuery
		}
	}
	if out.Level != "" {
		switch out.Level {
		case "debug", "info", "warn", "error":
		default:
			return Query{}, ErrInvalidQuery
		}
	}
	if len(out.Search) > MaxLogSearchLen {
		return Query{}, ErrInvalidQuery
	}
	return out, nil
}

func parseLimit(raw string, maxLimit int) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || v < 0 {
		return ErrInvalidQuery
	}
	if maxLimit > 0 && int(v) > maxLimit {
		return ErrInvalidQuery
	}
	return nil
}
