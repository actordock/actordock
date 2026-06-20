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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"sync"
	"syscall"

	"connectrpc.com/connect"
	processv1 "github.com/actordock/actordock/pkg/envd/process"
	"github.com/creack/pty"
)

const (
	ptyReadChunkSize  = 16 << 10
	defaultPTYCols    = 80
	defaultPTYRows    = 24
	processDataBuffer = 64
)

type ptyHandler struct {
	tag  *string
	cmd  *exec.Cmd
	tty  *os.File
	data *MultiplexedChannel[processv1.ProcessEvent_Data]
	end  *MultiplexedChannel[processv1.ProcessEvent_End]

	outCtx    context.Context
	outCancel context.CancelFunc
	outWG     sync.WaitGroup
}

func newPTYHandler(ctx context.Context, req *processv1.StartRequest) (*ptyHandler, error) {
	cfg := req.GetProcess()
	if cfg == nil || cfg.GetCmd() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("process.cmd is required"))
	}
	if req.GetPty() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("pty is required"))
	}

	cols := req.GetPty().GetSize().GetCols()
	rows := req.GetPty().GetSize().GetRows()
	if cols == 0 {
		cols = defaultPTYCols
	}
	if rows == 0 {
		rows = defaultPTYRows
	}

	cmdPath, cmdArgs := resolveCommand(cfg.GetCmd(), cfg.GetArgs())
	cmd := exec.CommandContext(ctx, cmdPath, cmdArgs...)
	if cwd := cfg.GetCwd(); cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Env = append(os.Environ(), flattenEnvs(cfg.GetEnvs())...)

	tty, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("start pty: %w", err))
	}

	outMultiplex := NewMultiplexedChannel[processv1.ProcessEvent_Data](processDataBuffer)
	outCtx, outCancel := context.WithCancel(ctx)

	h := &ptyHandler{
		tag:       req.Tag,
		cmd:       cmd,
		tty:       tty,
		data:      outMultiplex,
		end:       NewMultiplexedChannel[processv1.ProcessEvent_End](0),
		outCtx:    outCtx,
		outCancel: outCancel,
	}

	h.outWG.Go(func() {
		readBuf := make([]byte, ptyReadChunkSize)
		for {
			n, readErr := tty.Read(readBuf)
			if n > 0 && outMultiplex.HasSubscribers() {
				data := slices.Clone(readBuf[:n])
				outMultiplex.Source <- processv1.ProcessEvent_Data{
					Data: &processv1.ProcessEvent_DataEvent{
						Output: &processv1.ProcessEvent_DataEvent_Pty{Pty: data},
					},
				}
			}
			if errors.Is(readErr, io.EOF) || errors.Is(readErr, syscall.EIO) {
				break
			}
			if readErr != nil {
				break
			}
		}
	})

	go func() {
		h.outWG.Wait()
		close(outMultiplex.Source)
		outCancel()
	}()

	return h, nil
}

func (h *ptyHandler) PID() uint32 {
	if h.cmd.Process == nil {
		return 0
	}
	return uint32(h.cmd.Process.Pid)
}

func (h *ptyHandler) Tag() string {
	if h.tag == nil {
		return ""
	}
	return *h.tag
}

func (h *ptyHandler) WritePTY(data []byte) error {
	if h.tty == nil {
		return errors.New("tty not assigned")
	}
	_, err := h.tty.Write(data)
	return err
}

func (h *ptyHandler) Wait() {
	<-h.outCtx.Done()

	err := h.cmd.Wait()
	_ = h.tty.Close()

	var errMsg *string
	if err != nil {
		msg := err.Error()
		errMsg = &msg
	}

	exitCode := int32(0)
	exited := true
	status := "exited"
	if h.cmd.ProcessState != nil {
		exitCode = int32(h.cmd.ProcessState.ExitCode())
		exited = h.cmd.ProcessState.Exited()
		status = h.cmd.ProcessState.String()
	}

	h.end.Source <- processv1.ProcessEvent_End{
		End: &processv1.ProcessEvent_EndEvent{
			ExitCode: exitCode,
			Exited:   exited,
			Status:   status,
			Error:    errMsg,
		},
	}
	close(h.end.Source)
}

func connectDataResponse(data *processv1.ProcessEvent_DataEvent) *processv1.ConnectResponse {
	return &processv1.ConnectResponse{
		Event: &processv1.ProcessEvent{
			Event: &processv1.ProcessEvent_Data{Data: data},
		},
	}
}

func connectEndResponse(end *processv1.ProcessEvent_EndEvent) *processv1.ConnectResponse {
	return &processv1.ConnectResponse{
		Event: &processv1.ProcessEvent{
			Event: &processv1.ProcessEvent_End{End: end},
		},
	}
}

func connectStartResponse(pid uint32) *processv1.ConnectResponse {
	return &processv1.ConnectResponse{
		Event: &processv1.ProcessEvent{
			Event: &processv1.ProcessEvent_Start{
				Start: &processv1.ProcessEvent_StartEvent{Pid: pid},
			},
		},
	}
}
