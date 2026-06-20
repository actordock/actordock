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

package metrics

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
)

var ErrInvalidQuery = errors.New("invalid metrics query")

// HistoryQuery holds parsed history filter parameters.
type HistoryQuery struct {
	Start *int64
	End   *int64
}

// ValidateIntervalQuery checks Platform GET /sandboxes/{id}/metrics query params.
func ValidateIntervalQuery(r *http.Request) error {
	_, err := ParseHistoryQuery(r)
	return err
}

// ParseHistoryQuery validates and parses envd GET /metrics/history query params.
func ParseHistoryQuery(r *http.Request) (HistoryQuery, error) {
	q := r.URL.Query()
	out := HistoryQuery{}
	for _, key := range []string{"start", "end"} {
		raw := strings.TrimSpace(q.Get(key))
		if raw == "" {
			continue
		}
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			return HistoryQuery{}, ErrInvalidQuery
		}
		switch key {
		case "start":
			out.Start = &v
		case "end":
			out.End = &v
		}
	}
	return out, nil
}

// FilterHistory returns samples within [start, end] unix seconds when set.
func FilterHistory(samples []Sample, q HistoryQuery) []Sample {
	if q.Start == nil && q.End == nil {
		return samples
	}
	out := make([]Sample, 0, len(samples))
	for _, s := range samples {
		if q.Start != nil && s.TimestampUnix < *q.Start {
			continue
		}
		if q.End != nil && s.TimestampUnix > *q.End {
			continue
		}
		out = append(out, s)
	}
	return out
}
