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
	"sync"

	"connectrpc.com/connect"
	"github.com/actordock/actordock/internal/logs"
	processv1 "github.com/actordock/actordock/pkg/envd/process"
)

type processService struct {
	logger    *slog.Logger
	logs      *logs.Buffer
	processes sync.Map // uint32 -> *ptyHandler
}

func (s *processService) List(
	context.Context,
	*connect.Request[processv1.ListRequest],
) (*connect.Response[processv1.ListResponse], error) {
	return connect.NewResponse(&processv1.ListResponse{}), nil
}

func (s *processService) Connect(
	ctx context.Context,
	req *connect.Request[processv1.ConnectRequest],
	stream *connect.ServerStream[processv1.ConnectResponse],
) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	proc, err := s.getProcess(req.Msg.GetProcess())
	if err != nil {
		return err
	}

	exitChan := make(chan struct{})

	data, dataCancel := proc.data.Fork()
	defer dataCancel()

	end, endCancel := proc.end.Fork()
	defer endCancel()

	if err := stream.Send(connectStartResponse(proc.PID())); err != nil {
		return connect.NewError(connect.CodeUnknown, fmt.Errorf("send start event: %w", err))
	}

	go func() {
		defer close(exitChan)

	dataLoop:
		for {
			select {
			case <-ctx.Done():
				cancel(ctx.Err())
				return
			case event, ok := <-data:
				if !ok {
					break dataLoop
				}
				if event.Data == nil {
					continue
				}
				if err := stream.Send(connectDataResponse(event.Data)); err != nil {
					cancel(connect.NewError(connect.CodeUnknown, fmt.Errorf("send data event: %w", err)))
					return
				}
			}
		}

		select {
		case <-ctx.Done():
			cancel(ctx.Err())
			return
		case event, ok := <-end:
			if !ok {
				cancel(connect.NewError(connect.CodeUnknown, errors.New("end event channel closed")))
				return
			}
			if event.End == nil {
				return
			}
			if err := stream.Send(connectEndResponse(event.End)); err != nil {
				cancel(connect.NewError(connect.CodeUnknown, fmt.Errorf("send end event: %w", err)))
			}
		}
	}()

	<-exitChan
	if err := context.Cause(ctx); err != nil {
		var connectErr *connect.Error
		if errors.As(err, &connectErr) {
			return connectErr
		}
		return err
	}
	return nil
}

func (s *processService) Start(
	ctx context.Context,
	req *connect.Request[processv1.StartRequest],
	stream *connect.ServerStream[processv1.StartResponse],
) error {
	if req.Msg.GetPty() != nil {
		return s.startPTY(ctx, req, stream)
	}
	return s.startOneShot(ctx, req, stream)
}

func (s *processService) startOneShot(
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
			Event: &processv1.ProcessEvent_End{End: end},
		},
	})
}

func (s *processService) startPTY(
	ctx context.Context,
	req *connect.Request[processv1.StartRequest],
	stream *connect.ServerStream[processv1.StartResponse],
) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	procCtx := context.WithoutCancel(ctx)
	proc, err := newPTYHandler(procCtx, req.Msg)
	if err != nil {
		return err
	}

	pid := proc.PID()
	s.processes.Store(pid, proc)

	exitChan := make(chan struct{})

	data, dataCancel := proc.data.Fork()
	defer dataCancel()

	end, endCancel := proc.end.Fork()
	defer endCancel()

	go func() {
		defer close(exitChan)

		if err := stream.Send(&processv1.StartResponse{
			Event: &processv1.ProcessEvent{
				Event: &processv1.ProcessEvent_Start{
					Start: &processv1.ProcessEvent_StartEvent{Pid: pid},
				},
			},
		}); err != nil {
			cancel(connect.NewError(connect.CodeUnknown, fmt.Errorf("send start event: %w", err)))
			return
		}

	dataLoop:
		for {
			select {
			case <-ctx.Done():
				cancel(ctx.Err())
				return
			case event, ok := <-data:
				if !ok {
					break dataLoop
				}
				if event.Data == nil {
					continue
				}
				if err := stream.Send(&processv1.StartResponse{
					Event: &processv1.ProcessEvent{
						Event: &processv1.ProcessEvent_Data{Data: event.Data},
					},
				}); err != nil {
					cancel(connect.NewError(connect.CodeUnknown, fmt.Errorf("send data event: %w", err)))
					return
				}
			}
		}

		select {
		case <-ctx.Done():
			cancel(ctx.Err())
			return
		case event, ok := <-end:
			if !ok {
				cancel(connect.NewError(connect.CodeUnknown, errors.New("end event channel closed")))
				return
			}
			if err := stream.Send(&processv1.StartResponse{
				Event: &processv1.ProcessEvent{
					Event: &processv1.ProcessEvent_End{End: event.End},
				},
			}); err != nil {
				cancel(connect.NewError(connect.CodeUnknown, fmt.Errorf("send end event: %w", err)))
			}
		}
	}()

	go func() {
		defer s.processes.Delete(pid)
		proc.Wait()
	}()

	<-exitChan
	if err := context.Cause(ctx); err != nil {
		var connectErr *connect.Error
		if errors.As(err, &connectErr) {
			return connectErr
		}
		return err
	}
	return nil
}

func (s *processService) getProcess(selector *processv1.ProcessSelector) (*ptyHandler, error) {
	if selector == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("process selector is required"))
	}

	switch sel := selector.GetSelector().(type) {
	case *processv1.ProcessSelector_Pid:
		value, ok := s.processes.Load(sel.Pid)
		if !ok {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("process with pid %d not found", sel.Pid))
		}
		return value.(*ptyHandler), nil
	case *processv1.ProcessSelector_Tag:
		var found *ptyHandler
		s.processes.Range(func(_ any, value any) bool {
			proc := value.(*ptyHandler)
			if proc.Tag() == sel.Tag {
				found = proc
				return false
			}
			return true
		})
		if found == nil {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("process with tag %q not found", sel.Tag))
		}
		return found, nil
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("pid or tag is required"))
	}
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
	_ context.Context,
	req *connect.Request[processv1.SendInputRequest],
) (*connect.Response[processv1.SendInputResponse], error) {
	proc, err := s.getProcess(req.Msg.GetProcess())
	if err != nil {
		return nil, err
	}

	input := req.Msg.GetInput()
	if input == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("input is required"))
	}

	switch in := input.GetInput().(type) {
	case *processv1.ProcessInput_Pty:
		if err := proc.WritePTY(in.Pty); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("write pty: %w", err))
		}
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("pty input is required for connect sessions"))
	}

	return connect.NewResponse(&processv1.SendInputResponse{}), nil
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
