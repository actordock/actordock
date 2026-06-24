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

package router

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/actordock/actordock/internal/envd"
	"github.com/actordock/actordock/internal/runtimeapi"
	processv1 "github.com/actordock/actordock/pkg/envd/process"
	"github.com/actordock/actordock/pkg/envd/process/processv1connect"
	"golang.org/x/net/http2"
)

const testSandboxID = "connect-sbx"

func startH2CTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(h2cHandler(handler))
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

func connectHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, addr)
			},
		},
	}
}

func newRouterOverEnvd(t *testing.T, envdHandler http.Handler, waitEnvd bool) (*httptest.Server, *fakeBackend) {
	t.Helper()

	envdSrv := startH2CTestServer(t, envdHandler)
	actors := &fakeBackend{backend: envdSrv.Listener.Addr().String(), waitEnvd: waitEnvd}
	router := NewServer(testConfig(), actors, nil, slog.Default())

	routerSrv := startH2CTestServer(t, router.Handler())
	return routerSrv, actors
}

func withSandboxID() connect.ClientOption {
	return connect.WithInterceptors(newSandboxIDInterceptor(testSandboxID))
}

type sandboxIDInterceptor struct {
	sandboxID string
}

func newSandboxIDInterceptor(sandboxID string) connect.Interceptor {
	return &sandboxIDInterceptor{sandboxID: sandboxID}
}

func (i *sandboxIDInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		req.Header().Set(sandboxIDHeader, i.sandboxID)
		return next(ctx, req)
	}
}

func (i *sandboxIDInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		conn.RequestHeader().Set(sandboxIDHeader, i.sandboxID)
		return conn
	}
}

func (i *sandboxIDInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

func startPTYThroughRouter(t *testing.T, client processv1connect.ProcessClient) uint32 {
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
		t.Fatalf("Start PTY through router: %v", err)
	}

	var pid uint32
	for stream.Receive() {
		if start := stream.Msg().GetEvent().GetStart(); start != nil {
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

func TestProxyConnectRPC(t *testing.T) {
	t.Parallel()

	routerSrv, actors := newRouterOverEnvd(t, envd.NewStubHandler(), false)
	client := processv1connect.NewProcessClient(
		connectHTTPClient(),
		routerSrv.URL,
		withSandboxID(),
	)

	pid := startPTYThroughRouter(t, client)
	if actors.lastSandboxID != testSandboxID {
		t.Fatalf("ResumeSandboxBackend sandbox id = %q, want %q", actors.lastSandboxID, testSandboxID)
	}

	connectStream, err := client.Connect(context.Background(), connect.NewRequest(&processv1.ConnectRequest{
		Process: &processv1.ProcessSelector{
			Selector: &processv1.ProcessSelector_Pid{Pid: pid},
		},
	}))
	if err != nil {
		t.Fatalf("Connect through router: %v", err)
	}
	if !connectStream.Receive() || connectStream.Msg().GetEvent().GetStart() == nil {
		t.Fatal("Connect: missing start event")
	}

	_, err = client.SendInput(context.Background(), connect.NewRequest(&processv1.SendInputRequest{
		Process: &processv1.ProcessSelector{
			Selector: &processv1.ProcessSelector_Pid{Pid: pid},
		},
		Input: &processv1.ProcessInput{
			Input: &processv1.ProcessInput_Pty{Pty: []byte("echo router-connect-ok\nexit\n")},
		},
	}))
	if err != nil {
		t.Fatalf("SendInput through router: %v", err)
	}

	var output string
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for connect output, got %q", output)
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
	if !strings.Contains(output, "router-connect-ok") {
		t.Fatalf("Connect output = %q, want router-connect-ok", output)
	}
}

func TestProxyConnectResumesPausedSandbox(t *testing.T) {
	t.Parallel()

	routerSrv, actors := newRouterOverEnvd(t, envd.NewStubHandler(), true)
	client := processv1connect.NewProcessClient(
		connectHTTPClient(),
		routerSrv.URL,
		withSandboxID(),
	)

	pid := startPTYThroughRouter(t, client)
	if !actors.waitEnvd {
		t.Fatal("expected waitEnvd path")
	}
	if actors.lastSandboxID != testSandboxID {
		t.Fatalf("sandbox id = %q, want %q", actors.lastSandboxID, testSandboxID)
	}

	connectStream, err := client.Connect(context.Background(), connect.NewRequest(&processv1.ConnectRequest{
		Process: &processv1.ProcessSelector{
			Selector: &processv1.ProcessSelector_Pid{Pid: pid},
		},
	}))
	if err != nil {
		t.Fatalf("Connect after resume: %v", err)
	}
	if !connectStream.Receive() || connectStream.Msg().GetEvent().GetStart() == nil {
		t.Fatal("Connect after resume: missing start event")
	}
}

func TestProxyConnectSandboxNotFound(t *testing.T) {
	t.Parallel()

	router := NewServer(testConfig(), &fakeBackend{err: runtimeapi.ErrNotFound}, nil, slog.Default())
	routerSrv := startH2CTestServer(t, router.Handler())

	client := processv1connect.NewProcessClient(
		connectHTTPClient(),
		routerSrv.URL,
		withSandboxID(),
	)

	stream, err := client.Connect(context.Background(), connect.NewRequest(&processv1.ConnectRequest{
		Process: &processv1.ProcessSelector{
			Selector: &processv1.ProcessSelector_Pid{Pid: 1},
		},
	}))
	if err == nil {
		for stream.Receive() {
		}
		err = stream.Err()
	}
	if err == nil {
		t.Fatal("Connect: want error for missing sandbox")
	}
	if connect.CodeOf(err) != connect.CodeNotFound && !strings.Contains(err.Error(), "404") {
		t.Fatalf("Connect error = %v, want not found", err)
	}
}
