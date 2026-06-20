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
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/agent-substrate/substrate/pkg/proto/ateapipb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

var ErrNotFound = errors.New("actor not found")

type Client struct {
	conn *grpc.ClientConn
	api  ateapipb.ControlClient
}

func Dial(addr string) (*Client, error) {
	creds := credentials.NewTLS(&tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, //nolint:gosec // Kind dev cluster uses pod certs without public CA.
	})
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("dial ateapi %s: %w", addr, err)
	}
	return &Client{
		conn: conn,
		api:  ateapipb.NewControlClient(conn),
	}, nil
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) CreateAndResumeSandbox(
	ctx context.Context,
	actorID, templateNamespace, templateName string,
) error {
	_, err := c.api.CreateActor(ctx, &ateapipb.CreateActorRequest{
		ActorId:                actorID,
		ActorTemplateNamespace: templateNamespace,
		ActorTemplateName:      templateName,
	})
	if err != nil {
		return fmt.Errorf("create actor: %w", err)
	}
	_, err = c.api.ResumeActor(ctx, &ateapipb.ResumeActorRequest{
		ActorId: actorID,
	})
	if err != nil {
		return fmt.Errorf("resume actor: %w", err)
	}
	return nil
}

func (c *Client) ResumeSandboxBackend(ctx context.Context, actorID string, envdPort int) (string, error) {
	resp, err := c.api.ResumeActor(ctx, &ateapipb.ResumeActorRequest{
		ActorId: actorID,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("resume actor: %w", err)
	}

	ip := resp.GetActor().GetAteomPodIp()
	if ip == "" {
		return "", fmt.Errorf("actor %q has no worker assigned", actorID)
	}
	return net.JoinHostPort(ip, strconv.Itoa(envdPort)), nil
}

func (c *Client) SuspendSandbox(ctx context.Context, actorID string) error {
	resp, err := c.api.GetActor(ctx, &ateapipb.GetActorRequest{ActorId: actorID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return ErrNotFound
		}
		return fmt.Errorf("get actor: %w", err)
	}

	if resp.GetActor().GetStatus() == ateapipb.Actor_STATUS_SUSPENDED {
		return nil
	}

	_, err = c.api.SuspendActor(ctx, &ateapipb.SuspendActorRequest{ActorId: actorID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return ErrNotFound
		}
		return fmt.Errorf("suspend actor: %w", err)
	}
	return nil
}

func (c *Client) DeleteSandbox(ctx context.Context, actorID string) error {
	if err := c.SuspendSandbox(ctx, actorID); err != nil {
		return err
	}

	_, err := c.api.DeleteActor(ctx, &ateapipb.DeleteActorRequest{ActorId: actorID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return ErrNotFound
		}
		return fmt.Errorf("delete actor: %w", err)
	}
	return nil
}
