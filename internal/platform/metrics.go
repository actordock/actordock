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
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/actordock/actordock/internal/envd"
	"github.com/actordock/actordock/internal/metrics"
	"github.com/actordock/actordock/internal/store"
)

const maxSandboxMetricsIDs = 100

var (
	errMissingSandboxIDs = errors.New("sandbox_ids is required")
	errInvalidSandboxIDs = errors.New("invalid sandbox_ids")
)

type sandboxMetricResponse = metrics.Sample

type sandboxesWithMetricsResponse struct {
	Sandboxes map[string]sandboxMetricResponse `json:"sandboxes"`
}

func parseMetricsIntervalQuery(r *http.Request) error {
	return metrics.ValidateIntervalQuery(r)
}

func parseSandboxIDs(r *http.Request) ([]string, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("sandbox_ids"))
	if raw == "" {
		return nil, errMissingSandboxIDs
	}

	parts := strings.Split(raw, ",")
	ids := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" {
			return nil, errInvalidSandboxIDs
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, errInvalidSandboxIDs
	}
	if len(ids) > maxSandboxMetricsIDs {
		return nil, errInvalidSandboxIDs
	}
	return ids, nil
}

func buildStubSandboxMetric(now time.Time) sandboxMetricResponse {
	return metrics.FallbackSample(now)
}

func (s *Server) fetchSandboxMetric(ctx context.Context, sb store.Sandbox) sandboxMetricResponse {
	backend, err := s.actors.GetActorBackend(ctx, sb.ActorID, s.cfg.EnvdPort)
	if err != nil {
		s.logger.Warn("get sandbox metrics backend", "sandbox_id", sb.SandboxID, "err", err)
		return metrics.FallbackSample(s.nowFunc())
	}
	sample, err := envd.FetchMetric(ctx, "http://"+backend)
	if err != nil {
		s.logger.Warn("fetch sandbox metrics from envd", "sandbox_id", sb.SandboxID, "err", err)
		return metrics.FallbackSample(s.nowFunc())
	}
	return sample
}

func (s *Server) fetchSandboxMetricsHistory(ctx context.Context, sb store.Sandbox, rawQuery string) []sandboxMetricResponse {
	backend, err := s.actors.GetActorBackend(ctx, sb.ActorID, s.cfg.EnvdPort)
	if err != nil {
		s.logger.Warn("get sandbox metrics history backend", "sandbox_id", sb.SandboxID, "err", err)
		return nil
	}
	samples, err := envd.FetchMetricHistory(ctx, "http://"+backend, rawQuery)
	if err != nil {
		s.logger.Warn("fetch sandbox metrics history from envd", "sandbox_id", sb.SandboxID, "err", err)
		return nil
	}
	return samples
}

func metricsIntervalRequested(r *http.Request) bool {
	q := r.URL.Query()
	return strings.TrimSpace(q.Get("start")) != "" || strings.TrimSpace(q.Get("end")) != ""
}
