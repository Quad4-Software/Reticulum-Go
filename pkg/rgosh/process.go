// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rgosh

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

// LocalProcess runs a command with pipes or a PTY.
type LocalProcess struct {
	cmd    *exec.Cmd
	stdin  WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	ptyF   *os.File
	usePTY bool
	once   sync.Once
}

// StartLocalProcess starts cmdline with PTY unless any pipe flag is set.
func StartLocalProcess(req ExecRequest) (ProcessHandle, error) {
	if len(req.Cmdline) == 0 {
		return nil, fmt.Errorf("empty cmdline")
	}
	cmd := exec.Command(req.Cmdline[0], req.Cmdline[1:]...) // #nosec G204 -- operator allow-listed remote shell
	if req.Term != "" {
		cmd.Env = append(os.Environ(), "TERM="+req.Term)
	}
	usePTY := !req.PipeStdin && !req.PipeStdout && !req.PipeStderr
	lp := &LocalProcess{cmd: cmd, usePTY: usePTY}
	if usePTY {
		f, err := pty.StartWithSize(cmd, &pty.Winsize{
			Rows: uint16(clampU16(req.Rows)),
			Cols: uint16(clampU16(req.Cols)),
			X:    uint16(clampU16(req.HPix)),
			Y:    uint16(clampU16(req.VPix)),
		})
		if err != nil {
			return nil, err
		}
		lp.ptyF = f
		lp.stdin = f
		lp.stdout = f
		lp.stderr = io.NopCloser(emptyReader{})
		return lp, nil
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	lp.stdin = stdin
	lp.stdout = stdout
	lp.stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return lp, nil
}

func (p *LocalProcess) Stdin() WriteCloser { return p.stdin }
func (p *LocalProcess) Stdout() Reader     { return p.stdout }
func (p *LocalProcess) Stderr() Reader     { return p.stderr }

func (p *LocalProcess) SetWinSize(rows, cols, hpix, vpix int) error {
	if !p.usePTY || p.ptyF == nil {
		return nil
	}
	return pty.Setsize(p.ptyF, &pty.Winsize{
		Rows: uint16(clampU16(rows)),
		Cols: uint16(clampU16(cols)),
		X:    uint16(clampU16(hpix)),
		Y:    uint16(clampU16(vpix)),
	})
}

func (p *LocalProcess) Wait() (int, error) {
	if p.cmd.Process == nil {
		return 1, fmt.Errorf("not started")
	}
	// Use Process.Wait rather than Cmd.Wait. Cmd.Wait closes StdoutPipe/StderrPipe
	// parent ends and can drop unread output when the pump still needs those pipes.
	state, err := p.cmd.Process.Wait()
	p.once.Do(func() {
		if p.ptyF != nil {
			_ = p.ptyF.Close()
		}
	})
	if state != nil {
		p.cmd.ProcessState = state
	}
	if err != nil {
		return 1, err
	}
	if state != nil && !state.Success() {
		return state.ExitCode(), &exec.ExitError{ProcessState: state}
	}
	return 0, nil
}

func (p *LocalProcess) Kill() error {
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

type emptyReader struct{}

func (emptyReader) Read([]byte) (int, error) { return 0, io.EOF }

// DefaultShell returns the user shell or /bin/sh.
func DefaultShell() []string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return []string{sh}
	}
	return []string{"/bin/sh"}
}
