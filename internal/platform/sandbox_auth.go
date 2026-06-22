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
	"fmt"

	"github.com/actordock/actordock/internal/envd"
)

func (s *Server) syncSecureEnvdAuth(ctx context.Context, actorID, token string) error {
	if token == "" {
		return nil
	}
	if err := envd.WaitForBackendReady(ctx, func(ctx context.Context) (string, error) {
		return s.actors.GetActorBackend(ctx, actorID, s.cfg.EnvdPort)
	}, envd.DefaultReadyTimeout); err != nil {
		return fmt.Errorf("wait for envd: %w", err)
	}
	backend, err := s.actors.GetActorBackend(ctx, actorID, s.cfg.EnvdPort)
	if err != nil {
		return fmt.Errorf("resolve envd backend: %w", err)
	}
	if err := envd.ConfigureAccessToken(ctx, "http://"+backend, token); err != nil {
		return fmt.Errorf("configure envd access token: %w", err)
	}
	return nil
}
