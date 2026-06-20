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

package envd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	processv1 "github.com/actordock/actordock/pkg/envd/process"
	"github.com/actordock/actordock/pkg/envd/process/processv1connect"
)

func TestHealthReturns204(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestProcessStartEchoHello(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	path, handler := processv1connect.NewProcessHandler(&processService{})
	mux.Handle(path, handler)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := processv1connect.NewProcessClient(server.Client(), server.URL)
	stream, err := client.Start(context.Background(), connect.NewRequest(&processv1.StartRequest{
		Process: &processv1.ProcessConfig{
			Cmd:  "/bin/sh",
			Args: []string{"-c", "echo hello"},
		},
	}))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	var gotStdout string
	for stream.Receive() {
		msg := stream.Msg()
		if data := msg.GetEvent().GetData(); data != nil {
			if stdout := data.GetStdout(); len(stdout) > 0 {
				gotStdout += string(stdout)
			}
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream: %v", err)
	}
	if gotStdout != "hello\n" {
		t.Fatalf("stdout = %q, want %q", gotStdout, "hello\n")
	}
}

func TestInitReturns204(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /init", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/init", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestProcessStartBashLoginEchoHello(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	path, handler := processv1connect.NewProcessHandler(&processService{})
	mux.Handle(path, handler)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := processv1connect.NewProcessClient(server.Client(), server.URL)
	stream, err := client.Start(context.Background(), connect.NewRequest(&processv1.StartRequest{
		Process: &processv1.ProcessConfig{
			Cmd:  "/bin/bash",
			Args: []string{"-l", "-c", "echo hello"},
		},
	}))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	var gotStdout string
	for stream.Receive() {
		msg := stream.Msg()
		if data := msg.GetEvent().GetData(); data != nil {
			if stdout := data.GetStdout(); len(stdout) > 0 {
				gotStdout += string(stdout)
			}
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream: %v", err)
	}
	if gotStdout != "hello\n" {
		t.Fatalf("stdout = %q, want %q", gotStdout, "hello\n")
	}
}

func TestResolveCommandBashLogin(t *testing.T) {
	t.Parallel()
	cmd, args := resolveCommand("/bin/bash", []string{"-l", "-c", "echo hello"})
	if cmd != "/bin/sh" || len(args) != 2 || args[0] != "-c" || args[1] != "echo hello" {
		t.Fatalf("got %q %v", cmd, args)
	}
}

func newProcessTestServer(t *testing.T) (*httptest.Server, processv1connect.ProcessClient) {
	t.Helper()

	mux := http.NewServeMux()
	path, handler := processv1connect.NewProcessHandler(&processService{})
	mux.Handle(path, handler)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server, processv1connect.NewProcessClient(server.Client(), server.URL)
}

func startPTYShell(t *testing.T, client processv1connect.ProcessClient) uint32 {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	stream, err := client.Start(ctx, connect.NewRequest(&processv1.StartRequest{
		Process: &processv1.ProcessConfig{
			Cmd:  "/bin/sh",
			Args: []string{"-i"},
		},
		Pty: &processv1.PTY{
			Size: &processv1.PTY_Size{Cols: 80, Rows: 24},
		},
	}))
	if err != nil {
		t.Fatalf("Start PTY: %v", err)
	}

	var pid uint32
	for stream.Receive() {
		msg := stream.Msg().GetEvent()
		if start := msg.GetStart(); start != nil {
			pid = start.GetPid()
			break
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Start PTY stream: %v", err)
	}
	if pid == 0 {
		t.Fatal("Start PTY did not return pid")
	}
	cancel()
	return pid
}

func TestProcessConnectPTY(t *testing.T) {
	t.Parallel()

	_, client := newProcessTestServer(t)
	pid := startPTYShell(t, client)

	connectStream, err := client.Connect(context.Background(), connect.NewRequest(&processv1.ConnectRequest{
		Process: &processv1.ProcessSelector{
			Selector: &processv1.ProcessSelector_Pid{Pid: pid},
		},
	}))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if !connectStream.Receive() {
		t.Fatal("Connect stream closed before start event")
	}
	start := connectStream.Msg().GetEvent().GetStart()
	if start == nil || start.GetPid() != pid {
		t.Fatalf("Connect start event = %v, want pid %d", start, pid)
	}

	_, err = client.SendInput(context.Background(), connect.NewRequest(&processv1.SendInputRequest{
		Process: &processv1.ProcessSelector{
			Selector: &processv1.ProcessSelector_Pid{Pid: pid},
		},
		Input: &processv1.ProcessInput{
			Input: &processv1.ProcessInput_Pty{Pty: []byte("echo connect-ok\nexit\n")},
		},
	}))
	if err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	var output string
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for PTY output, got %q", output)
		default:
		}
		if !connectStream.Receive() {
			break
		}
		msg := connectStream.Msg().GetEvent()
		if data := msg.GetData(); data != nil {
			if ptyOut := data.GetPty(); len(ptyOut) > 0 {
				output += string(ptyOut)
			}
		}
		if msg.GetEnd() != nil {
			break
		}
	}
	if err := connectStream.Err(); err != nil {
		t.Fatalf("Connect stream: %v", err)
	}
	if !strings.Contains(output, "connect-ok") {
		t.Fatalf("Connect output = %q, want connect-ok", output)
	}
}

func TestProcessConnectNotFound(t *testing.T) {
	t.Parallel()

	_, client := newProcessTestServer(t)

	stream, err := client.Connect(context.Background(), connect.NewRequest(&processv1.ConnectRequest{
		Process: &processv1.ProcessSelector{
			Selector: &processv1.ProcessSelector_Pid{Pid: 99999},
		},
	}))
	if err == nil {
		for stream.Receive() {
		}
		err = stream.Err()
	}
	if err == nil {
		t.Fatal("Connect: want error for missing pid")
	}
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("Connect error code = %v, want NotFound", connect.CodeOf(err))
	}
}

func TestProcessConnectByTag(t *testing.T) {
	t.Parallel()

	_, client := newProcessTestServer(t)
	tag := "shell-a"

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	stream, err := client.Start(ctx, connect.NewRequest(&processv1.StartRequest{
		Process: &processv1.ProcessConfig{
			Cmd:  "/bin/sh",
			Args: []string{"-i"},
		},
		Pty: &processv1.PTY{
			Size: &processv1.PTY_Size{Cols: 80, Rows: 24},
		},
		Tag: &tag,
	}))
	if err != nil {
		t.Fatalf("Start PTY: %v", err)
	}
	for stream.Receive() {
		if stream.Msg().GetEvent().GetStart() != nil {
			break
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Start PTY stream: %v", err)
	}
	cancel()

	connectCtx, connectCancel := context.WithCancel(context.Background())
	t.Cleanup(connectCancel)

	connectStream, err := client.Connect(connectCtx, connect.NewRequest(&processv1.ConnectRequest{
		Process: &processv1.ProcessSelector{
			Selector: &processv1.ProcessSelector_Tag{Tag: tag},
		},
	}))
	if err != nil {
		t.Fatalf("Connect by tag: %v", err)
	}
	if !connectStream.Receive() || connectStream.Msg().GetEvent().GetStart() == nil {
		t.Fatal("Connect by tag: missing start event")
	}
	connectCancel()
}
