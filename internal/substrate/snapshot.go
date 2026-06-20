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
	"errors"
	"fmt"

	"github.com/agent-substrate/substrate/pkg/proto/ateapipb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrInvalidState = errors.New("actor is not in a snapshotable state")

// SnapshotResult is metadata returned after a Substrate checkpoint.
type SnapshotResult struct {
	SnapshotURI  string
	SnapshotType string
}

func (c *Client) CreateSnapshot(ctx context.Context, actorID string) (SnapshotResult, error) {
	resp, err := c.api.GetActor(ctx, &ateapipb.GetActorRequest{ActorId: actorID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return SnapshotResult{}, ErrNotFound
		}
		return SnapshotResult{}, fmt.Errorf("get actor: %w", err)
	}

	actor := resp.GetActor()
	if actor.GetStatus() != ateapipb.Actor_STATUS_RUNNING {
		return SnapshotResult{}, ErrInvalidState
	}

	suspendResp, err := c.api.SuspendActor(ctx, &ateapipb.SuspendActorRequest{ActorId: actorID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return SnapshotResult{}, ErrNotFound
		}
		return SnapshotResult{}, fmt.Errorf("suspend actor: %w", err)
	}

	info := suspendResp.GetActor().GetLatestSnapshotInfo()
	if info == nil {
		return SnapshotResult{}, fmt.Errorf("suspend actor: missing snapshot info")
	}

	result := SnapshotResult{SnapshotType: info.GetType().String()}
	switch data := info.GetData().(type) {
	case *ateapipb.SnapshotInfo_External:
		result.SnapshotURI = data.External.GetSnapshotUriPrefix()
	case *ateapipb.SnapshotInfo_Local:
		result.SnapshotURI = data.Local.GetSnapshotPrefix()
	default:
		return SnapshotResult{}, fmt.Errorf("suspend actor: unsupported snapshot type")
	}
	if result.SnapshotURI == "" {
		return SnapshotResult{}, fmt.Errorf("suspend actor: empty snapshot location")
	}
	return result, nil
}
