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

package substrate

import (
	"context"
	"fmt"

	"github.com/agent-substrate/substrate/pkg/proto/ateapipb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *Client) GetActor(ctx context.Context, actorID string) (ateapipb.Actor_Status, error) {
	resp, err := c.api.GetActor(ctx, &ateapipb.GetActorRequest{ActorId: actorID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return ateapipb.Actor_STATUS_UNSPECIFIED, ErrNotFound
		}
		return ateapipb.Actor_STATUS_UNSPECIFIED, fmt.Errorf("get actor: %w", err)
	}
	return resp.GetActor().GetStatus(), nil
}

// ActorStateE2B maps Substrate actor status to E2B sandbox state.
func ActorStateE2B(actorStatus ateapipb.Actor_Status) string {
	switch actorStatus {
	case ateapipb.Actor_STATUS_SUSPENDED,
		ateapipb.Actor_STATUS_PAUSED,
		ateapipb.Actor_STATUS_SUSPENDING,
		ateapipb.Actor_STATUS_PAUSING:
		return "paused"
	default:
		return "running"
	}
}
