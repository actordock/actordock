// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package workerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client talks to a Worker agent (slim atelet/ateom HTTP surface).
type Client struct {
	http *http.Client
}

func New() *Client {
	return &Client{http: &http.Client{Timeout: 3 * time.Minute}}
}

type Status struct {
	WorkerID  string `json:"workerID"`
	MaxSlots  int    `json:"maxSlots"`
	UsedSlots int    `json:"usedSlots"`
	Healthy   bool   `json:"healthy"`
}

type CheckpointOpts struct {
	ImagePath string
	ObjectKey string // empty = Pause (local); set = Suspend (upload)
}

type RestoreOpts struct {
	ImagePath string
	ObjectKey string // download if local missing
}

func (c *Client) Status(ctx context.Context, base string) (Status, error) {
	var st Status
	if err := c.do(ctx, http.MethodGet, base+"/status", nil, &st); err != nil {
		return Status{}, err
	}
	return st, nil
}

func (c *Client) Boot(ctx context.Context, base, id string) error {
	return c.do(ctx, http.MethodPost, base+"/sandboxes/"+id+"/boot", map[string]string{"id": id}, nil)
}

func (c *Client) Checkpoint(ctx context.Context, base, id string, opts CheckpointOpts) error {
	return c.do(ctx, http.MethodPost, base+"/sandboxes/"+id+"/checkpoint", opts, nil)
}

func (c *Client) Restore(ctx context.Context, base, id string, opts RestoreOpts) error {
	return c.do(ctx, http.MethodPost, base+"/sandboxes/"+id+"/restore", opts, nil)
}

func (c *Client) LocalSnapshotExists(ctx context.Context, base, path string) (bool, error) {
	var out struct {
		Exists bool `json:"exists"`
	}
	if err := c.do(ctx, http.MethodGet, base+"/local-snapshot?path="+path, nil, &out); err != nil {
		return false, err
	}
	return out.Exists, nil
}

func (c *Client) Delete(ctx context.Context, base, id string) error {
	return c.do(ctx, http.MethodDelete, base+"/sandboxes/"+id, nil, nil)
}

func (c *Client) Exec(ctx context.Context, base, id string, argv []string) (string, error) {
	var out struct {
		Stdout string `json:"stdout"`
	}
	if err := c.do(ctx, http.MethodPost, base+"/sandboxes/"+id+"/exec", map[string]any{"argv": argv}, &out); err != nil {
		return "", err
	}
	return out.Stdout, nil
}

func (c *Client) do(ctx context.Context, method, rawURL string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s: %s", method, rawURL, resp.Status, bytes.TrimSpace(raw))
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}
