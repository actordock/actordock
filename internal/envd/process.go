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
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"connectrpc.com/connect"
	"github.com/actordock/actordock/internal/logs"
	processv1 "github.com/actordock/actordock/pkg/envd/process"
)

type processService struct {
	logger *slog.Logger
	logs   *logs.Buffer
}

func (s *processService) List(
	context.Context,
	*connect.Request[processv1.ListRequest],
) (*connect.Response[processv1.ListResponse], error) {
	return connect.NewResponse(&processv1.ListResponse{}), nil
}

func (s *processService) Connect(
	context.Context,
	*connect.Request[processv1.ConnectRequest],
	*connect.ServerStream[processv1.ConnectResponse],
) error {
	return connect.NewError(connect.CodeUnimplemented, errors.New("connect not implemented"))
}

func (s *processService) Start(
	ctx context.Context,
	req *connect.Request[processv1.StartRequest],
	stream *connect.ServerStream[processv1.StartResponse],
) error {
	cfg := req.Msg.GetProcess()
	if cfg == nil || cfg.GetCmd() == "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("process.cmd is required"))
	}

	cmdPath, cmdArgs := resolveCommand(cfg.GetCmd(), cfg.GetArgs())
	cmd := exec.CommandContext(ctx, cmdPath, cmdArgs...)
	if cwd := cfg.GetCwd(); cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Env = append(os.Environ(), flattenEnvs(cfg.GetEnvs())...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("start process: %w", err))
	}

	pid := uint32(cmd.Process.Pid)
	if err := stream.Send(&processv1.StartResponse{
		Event: &processv1.ProcessEvent{
			Event: &processv1.ProcessEvent_Start{
				Start: &processv1.ProcessEvent_StartEvent{Pid: pid},
			},
		},
	}); err != nil {
		_ = cmd.Process.Kill()
		return err
	}

	runErr := cmd.Wait()
	if s.logs != nil {
		if stdout.Len() > 0 {
			s.logs.AppendOutput("stdout", stdout.Bytes())
		}
		if stderr.Len() > 0 {
			s.logs.AppendOutput("stderr", stderr.Bytes())
		}
	}
	if stdout.Len() > 0 {
		if err := stream.Send(dataResponse(stdout.Bytes(), true)); err != nil {
			return err
		}
	}
	if stderr.Len() > 0 {
		if err := stream.Send(dataResponse(stderr.Bytes(), false)); err != nil {
			return err
		}
	}

	exitCode := int32(0)
	exited := true
	status := "exited"
	var endErr string
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = int32(exitErr.ExitCode())
		} else {
			exitCode = -1
			endErr = runErr.Error()
		}
		status = "error"
	}

	end := &processv1.ProcessEvent_EndEvent{
		ExitCode: exitCode,
		Exited:   exited,
		Status:   status,
	}
	if endErr != "" {
		end.Error = &endErr
	}
	return stream.Send(&processv1.StartResponse{
		Event: &processv1.ProcessEvent{
			Event: &processv1.ProcessEvent_End{
				End: end,
			},
		},
	})
}

func dataResponse(data []byte, stdout bool) *processv1.StartResponse {
	dataEvt := &processv1.ProcessEvent_DataEvent{}
	if stdout {
		dataEvt.Output = &processv1.ProcessEvent_DataEvent_Stdout{Stdout: data}
	} else {
		dataEvt.Output = &processv1.ProcessEvent_DataEvent_Stderr{Stderr: data}
	}
	return &processv1.StartResponse{
		Event: &processv1.ProcessEvent{
			Event: &processv1.ProcessEvent_Data{Data: dataEvt},
		},
	}
}

func flattenEnvs(envs map[string]string) []string {
	out := make([]string, 0, len(envs))
	for k, v := range envs {
		out = append(out, k+"="+v)
	}
	return out
}

// resolveCommand maps E2B SDK's /bin/bash invocations to /bin/sh on Alpine.
func resolveCommand(cmd string, args []string) (string, []string) {
	if cmd != "/bin/bash" && cmd != "bash" {
		return cmd, args
	}
	for i, arg := range args {
		if arg == "-c" && i+1 < len(args) {
			return "/bin/sh", []string{"-c", args[i+1]}
		}
	}
	return "/bin/sh", args
}

func (s *processService) Update(
	context.Context,
	*connect.Request[processv1.UpdateRequest],
) (*connect.Response[processv1.UpdateResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("update not implemented"))
}

func (s *processService) StreamInput(
	context.Context,
	*connect.ClientStream[processv1.StreamInputRequest],
) (*connect.Response[processv1.StreamInputResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("stream input not implemented"))
}

func (s *processService) SendInput(
	context.Context,
	*connect.Request[processv1.SendInputRequest],
) (*connect.Response[processv1.SendInputResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("send input not implemented"))
}

func (s *processService) SendSignal(
	context.Context,
	*connect.Request[processv1.SendSignalRequest],
) (*connect.Response[processv1.SendSignalResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("send signal not implemented"))
}

func (s *processService) CloseStdin(
	context.Context,
	*connect.Request[processv1.CloseStdinRequest],
) (*connect.Response[processv1.CloseStdinResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("close stdin not implemented"))
}
