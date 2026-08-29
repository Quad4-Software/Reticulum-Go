// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package harness

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Session holds per-test artifact and event state.
type Session struct {
	Dir    string
	Events *EventLog
}

// Begin creates an artifact directory and optional event log for t.
func Begin(t *testing.T) *Session {
	t.Helper()
	dir := ArtifactsDir(t)
	s := &Session{Dir: dir}
	ev, err := NewEventLog(dir)
	if err != nil {
		t.Fatalf("event log: %v", err)
	}
	s.Events = ev
	t.Cleanup(func() {
		s.finish(t)
	})
	return s
}

func (s *Session) finish(t *testing.T) {
	t.Helper()
	if s == nil {
		return
	}
	keep := t.Failed() || AlwaysArtifacts()
	if !keep {
		return
	}
	_ = WriteEnvSnapshot(s.Dir, nil)
	LogArtifacts(t, s.Dir, s.Events)
}

// Emit records a Go-side timeline event when events are enabled.
func (s *Session) Emit(event string, kind Kind, detail string) {
	if s == nil || s.Events == nil {
		return
	}
	s.Events.Emit(event, kind.String(), detail, nil)
}

// Probe is a Python peer subprocess with stdout line waits and stderr capture.
type Probe struct {
	Cmd          *exec.Cmd
	Stdout       *bufio.Reader
	Events       *EventLog
	ArtifactsDir string

	mu        sync.Mutex
	stderrBuf bytes.Buffer
	done      chan error
	waitOnce  sync.Once
}

// ProbeOpts configures StartPython.
type ProbeOpts struct {
	Ctx          context.Context
	Python       string
	Script       string
	Env          []string
	Dir          string
	Events       *EventLog
	ArtifactsDir string
}

// PythonExe returns PYTHON_INTEROP, a local .venv or pipx rns interpreter, or python3.
func PythonExe() string {
	if p := os.Getenv("PYTHON_INTEROP"); p != "" {
		return p
	}
	for _, cand := range pythonInteropCandidates() {
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand
		}
	}
	return "python3"
}

func pythonInteropCandidates() []string {
	home, _ := os.UserHomeDir()
	cands := []string{
		filepath.Join(".venv", "bin", "python"),
		filepath.Join(".venv", "bin", "python3"),
		filepath.Join(".venv", "Scripts", "python.exe"),
	}
	if home != "" {
		pipxHome := os.Getenv("PIPX_HOME")
		if pipxHome == "" {
			pipxHome = filepath.Join(home, ".local", "share", "pipx")
		}
		cands = append(cands,
			filepath.Join(pipxHome, "venvs", "rns", "bin", "python"),
			filepath.Join(pipxHome, "venvs", "rns", "Scripts", "python.exe"),
		)
	}
	return cands
}

// StartPython launches a Python probe script with stdout token protocol.
func StartPython(t *testing.T, opts ProbeOpts) *Probe {
	t.Helper()
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}
	if opts.Python == "" {
		opts.Python = PythonExe()
	}
	if opts.Script == "" {
		t.Fatal("harness.StartPython: Script is required")
	}

	cmd := exec.CommandContext(opts.Ctx, opts.Python, opts.Script) // #nosec G204 -- interop harness uses controlled PYTHON_INTEROP and script paths
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	cmd.Env = append(os.Environ(), opts.Env...)
	if opts.Events != nil && opts.Events.Path() != "" {
		cmd.Env = append(cmd.Env,
			"INTEROP_EVENTS_PATH="+opts.Events.Path(),
			"INTEROP_EVENTS_GO_OWNED=1",
		)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	p := &Probe{
		Cmd:          cmd,
		Stdout:       bufio.NewReaderSize(stdout, 1<<20),
		Events:       opts.Events,
		ArtifactsDir: opts.ArtifactsDir,
		done:         make(chan error, 1),
	}

	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	cmd.Stderr = stderrW

	if err := cmd.Start(); err != nil {
		_ = stderrW.Close()
		_ = stderrR.Close()
		t.Fatalf("start python: %v", err)
	}
	_ = stderrW.Close()

	go p.drainStderr(stderrR)
	go func() {
		err := cmd.Wait()
		p.waitOnce.Do(func() { p.done <- err })
	}()

	t.Cleanup(func() {
		p.Kill(3 * time.Second)
		if t.Failed() || AlwaysArtifacts() {
			p.dumpArtifacts(t)
		}
	})

	if opts.Events != nil {
		opts.Events.Emit("spawn", KindSpawn.String(), filepath.Base(opts.Script), map[string]any{
			"script": opts.Script,
		})
	}
	return p
}

func (p *Probe) drainStderr(r io.ReadCloser) {
	defer r.Close()
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			p.mu.Lock()
			_, _ = p.stderrBuf.WriteString(line)
			p.mu.Unlock()
			trimmed := strings.TrimRight(line, "\r\n")
			if p.Events != nil {
				_ = p.Events.IngestPythonLine(trimmed)
			}
			if !strings.HasPrefix(trimmed, "INTEROP_EVENT ") {
				_, _ = os.Stderr.WriteString(line)
			}
		}
		if err != nil {
			return
		}
	}
}

// Done returns the Wait channel for the subprocess.
func (p *Probe) Done() <-chan error {
	return p.done
}

// StderrText returns captured stderr so far.
func (p *Probe) StderrText() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stderrBuf.String()
}

// Kill terminates the process and waits up to d.
func (p *Probe) Kill(d time.Duration) {
	if p == nil || p.Cmd == nil || p.Cmd.Process == nil {
		return
	}
	_ = p.Cmd.Process.Kill()
	if d <= 0 {
		d = 3 * time.Second
	}
	select {
	case <-p.done:
	case <-time.After(d):
	}
}

// ReadLine reads one stdout line with timeout.
func (p *Probe) ReadLine(ctx context.Context, d time.Duration) (string, error) {
	return ReadLineTimeout(ctx, p.Stdout, d)
}

// WaitToken reads stdout until TrimSpace(line) equals token or deadline.
func (p *Probe) WaitToken(ctx context.Context, token string, d time.Duration) (string, error) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		remain := time.Until(deadline)
		if remain <= 0 {
			break
		}
		chunk := min(remain, 5*time.Second)
		line, err := p.ReadLine(ctx, chunk)
		if err != nil {
			if err == context.DeadlineExceeded {
				continue
			}
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			select {
			case waitErr := <-p.done:
				if waitErr != nil {
					return "", waitErr
				}
				return "", context.DeadlineExceeded
			default:
				continue
			}
		}
		line = strings.TrimSpace(line)
		if line == token {
			return line, nil
		}
		if strings.HasPrefix(line, token+" ") {
			return line, nil
		}
	}
	return "", context.DeadlineExceeded
}

// WaitExact fails the test unless the next meaningful line matches token.
func (p *Probe) WaitExact(t *testing.T, ctx context.Context, token string, d time.Duration, kind Kind) string {
	t.Helper()
	line, err := p.WaitToken(ctx, token, d)
	if err != nil {
		if p.Events != nil {
			p.Events.Emit("fail", kind.String(), "wait "+token+": "+err.Error(), nil)
		}
		t.Fatalf("wait %s: %v", token, err)
	}
	if p.Events != nil {
		evName := strings.ToLower(token)
		p.Events.Emit(evName, "", line, nil)
	}
	return line
}

func (p *Probe) dumpArtifacts(t *testing.T) {
	t.Helper()
	dir := p.ArtifactsDir
	if dir == "" {
		return
	}
	_ = WriteStderrCapture(dir, p.StderrText())
	_ = WriteEnvSnapshot(dir, nil)
}
