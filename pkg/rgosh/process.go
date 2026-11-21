// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rgosh

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

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

// ptyStdinEOFGrace bounds how long a PTY child may ignore a remote stdin EOF
// before the master closes and SIGHUPs it, matching Python rnsh's close_stdin
// plus _ensure_terminate window (~250ms).
const ptyStdinEOFGrace = 250 * time.Millisecond

// ptyStdin wraps the PTY master fd used for child stdin. Closing the master
// SIGHUPs the child, so a remote stdin EOF that lands while a short-lived
// command is still exiting would kill it and surface a signal exit code
// upstream. Write EOT first so canonical-mode readers see a natural EOF, then
// fall back to closing the master after a grace window like Python's
// close_stdin + ensure_terminate does for children that linger.
type ptyStdin struct {
	f    *os.File
	proc *LocalProcess
}

func (s *ptyStdin) Write(b []byte) (int, error) { return s.f.Write(b) }

func (s *ptyStdin) Close() error {
	_, _ = s.f.Write([]byte{0x04})
	time.AfterFunc(ptyStdinEOFGrace, s.proc.closePTY)
	return nil
}

// StartLocalProcess starts cmdline with PTY unless any pipe flag is set.
// On Windows, or if PTY open returns pty.ErrUnsupported, it falls back to pipes.
func StartLocalProcess(req ExecRequest) (ProcessHandle, error) {
	if len(req.Cmdline) == 0 {
		return nil, fmt.Errorf("empty cmdline")
	}
	cmd := exec.Command(req.Cmdline[0], req.Cmdline[1:]...) // #nosec G204 -- operator allow-listed remote shell
	term := req.Term
	if term == "" {
		term = "xterm"
	}
	env := append(os.Environ(), "TERM="+term)
	if len(req.RemoteIdentity) > 0 {
		env = append(env, "RNS_REMOTE_IDENTITY=<"+hex.EncodeToString(req.RemoteIdentity)+">")
	}
	cmd.Env = env
	usePTY := !req.PipeStdin && !req.PipeStdout && !req.PipeStderr
	lp := &LocalProcess{cmd: cmd, usePTY: usePTY}
	if usePTY {
		f, err := pty.StartWithSize(cmd, &pty.Winsize{
			Rows: clampU16(req.Rows),
			Cols: clampU16(req.Cols),
			X:    clampU16(req.HPix),
			Y:    clampU16(req.VPix),
		})
		if err == nil {
			lp.ptyF = f
			lp.stdin = &ptyStdin{f: f, proc: lp}
			lp.stdout = f
			lp.stderr = io.NopCloser(emptyReader{})
			return lp, nil
		}
		if !errors.Is(err, pty.ErrUnsupported) && runtime.GOOS != "windows" {
			return nil, err
		}
		lp.usePTY = false
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
		Rows: clampU16(rows),
		Cols: clampU16(cols),
		X:    clampU16(hpix),
		Y:    clampU16(vpix),
	})
}

func (p *LocalProcess) Wait() (int, error) {
	if p.cmd.Process == nil {
		return 1, fmt.Errorf("not started")
	}
	// Use Process.Wait rather than Cmd.Wait. Cmd.Wait closes StdoutPipe/StderrPipe
	// parent ends and can drop unread output when the pump still needs those pipes.
	state, err := p.cmd.Process.Wait()
	p.closePTY()
	if state != nil {
		p.cmd.ProcessState = state
	}
	if err != nil {
		return 1, err
	}
	if state != nil && !state.Success() {
		return waitStatusCode(state, state.ExitCode()), &exec.ExitError{ProcessState: state}
	}
	return 0, nil
}

// closePTY closes the PTY master once, shared by Wait cleanup and the deferred
// stdin-EOF close scheduled by ptyStdin.Close.
func (p *LocalProcess) closePTY() {
	p.once.Do(func() {
		if p.ptyF != nil {
			_ = p.ptyF.Close()
		}
	})
}

func (p *LocalProcess) Kill() error {
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

type emptyReader struct{}

func (emptyReader) Read([]byte) (int, error) { return 0, io.EOF }

// DefaultShell returns the user shell, or a platform fallback.
func DefaultShell() []string {
	if runtime.GOOS == "windows" {
		if c := os.Getenv("ComSpec"); c != "" {
			return []string{c}
		}
		return []string{"cmd.exe"}
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return []string{sh}
	}
	return []string{"/bin/sh"}
}
