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
	"net/http"

	"github.com/actordock/actordock/internal/envd"
	"github.com/actordock/actordock/internal/logs"
	"github.com/actordock/actordock/internal/store"
)

type sandboxLogResponse = logs.LineEntry
type sandboxLogEntryResponse = logs.Entry

type sandboxLogsResponse = logs.V1Response
type sandboxLogsV2Response = logs.V2Response

func parseLogsV1Query(r *http.Request) error {
	return logs.ValidateV1Query(r)
}

func parseLogsV2Query(r *http.Request) error {
	return logs.ValidateV2Query(r)
}

func (s *Server) fetchSandboxLogEntries(ctx context.Context, sb store.Sandbox, rawQuery string) []logs.Entry {
	backend, err := s.actors.GetActorBackend(ctx, sb.ActorID, s.cfg.EnvdPort)
	if err != nil {
		s.logger.Warn("get sandbox logs backend", "sandbox_id", sb.SandboxID, "err", err)
		return nil
	}
	entries, err := envd.FetchLogs(ctx, "http://"+backend, rawQuery)
	if err != nil {
		s.logger.Warn("fetch sandbox logs from envd", "sandbox_id", sb.SandboxID, "err", err)
		return nil
	}
	return entries
}
