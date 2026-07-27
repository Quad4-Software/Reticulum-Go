// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rgosh

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"
)

type fakeProc struct {
	stdin  *pipeBuf
	stdout *pipeBuf
	stderr *pipeBuf
	code   int
	cmd    []string
	done   chan struct{}
}

type pipeBuf struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	closed bool
	wait   chan struct{}
}

func newPipeBuf() *pipeBuf {
	return &pipeBuf{wait: make(chan struct{}, 8)}
}

func (p *pipeBuf) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0, io.ErrClosedPipe
	}
	n, _ := p.buf.Write(b)
	select {
	case p.wait <- struct{}{}:
	default:
	}
	return n, nil
}

func (p *pipeBuf) Read(b []byte) (int, error) {
	for {
		p.mu.Lock()
		if p.buf.Len() > 0 {
			n, err := p.buf.Read(b)
			p.mu.Unlock()
			return n, err
		}
		closed := p.closed
		p.mu.Unlock()
		if closed {
			return 0, io.EOF
		}
		select {
		case <-p.wait:
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func (p *pipeBuf) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	select {
	case p.wait <- struct{}{}:
	default:
	}
	return nil
}

func (f *fakeProc) Stdin() WriteCloser                  { return f.stdin }
func (f *fakeProc) Stdout() Reader                      { return f.stdout }
func (f *fakeProc) Stderr() Reader                      { return f.stderr }
func (f *fakeProc) SetWinSize(int, int, int, int) error { return nil }
func (f *fakeProc) Wait() (int, error) {
	<-f.done
	return f.code, nil
}
func (f *fakeProc) Kill() error {
	close(f.done)
	_ = f.stdout.Close()
	_ = f.stderr.Close()
	return nil
}

func TestOracleAuthDenyNoExec(t *testing.T) {
	send := &memSender{}
	hashOK := []byte{1, 2, 3, 4}
	hashBad := []byte{9, 9, 9, 9}
	sess := NewSession(Config{
		Listener:   true,
		Allowed:    [][]byte{hashOK},
		DefaultCmd: []string{"/bin/true"},
	}, send)
	spawned := false
	sess.StartProcess = func(req ExecRequest) (ProcessHandle, error) {
		spawned = true
		return nil, io.EOF
	}
	if sess.SetRemoteIdentity(hashBad) {
		t.Fatal("expected deny")
	}
	if sess.State() != StateTeardown {
		t.Fatalf("state=%s", sess.State())
	}
	_ = sess.HandleMessage(&VersionMessage{ProtocolVersion: 1})
	_ = sess.HandleMessage(&ExecMessage{Cmdline: []string{"/bin/evil"}})
	if spawned || sess.Spawned() {
		t.Fatal("process must not start after auth deny")
	}
	if send.ofType(NativeAuthDenied) == 0 {
		t.Fatal("expected AuthDenied")
	}
	t.Log("PROVED auth deny never reaches exec")
}

func TestOracleArgvIsolation(t *testing.T) {
	base := Config{DefaultCmd: []string{"/bin/sh", "-l"}, Listener: true, AllowAll: true}
	s1 := NewSession(base, &memSender{})
	s2 := NewSession(base, &memSender{})
	s1.MutateDefaultCmdAppend("POLLUTE")
	if got := s2.ConfigSnapshot().DefaultCmd; len(got) != 2 || got[1] != "-l" {
		t.Fatalf("pollution leaked into s2: %#v", got)
	}
	if len(base.DefaultCmd) != 2 {
		t.Fatalf("base polluted: %#v", base.DefaultCmd)
	}
	t.Log("PROVED argv isolation across sessions")
}

func TestOracleForcedCommand(t *testing.T) {
	send := &memSender{}
	sess := NewSession(Config{
		Listener:      true,
		AllowAll:      true,
		ForcedCommand: true,
		DefaultCmd:    []string{"/bin/forced"},
	}, send)
	var got []string
	done := make(chan struct{})
	sess.StartProcess = func(req ExecRequest) (ProcessHandle, error) {
		got = append([]string(nil), req.Cmdline...)
		fp := &fakeProc{
			stdin:  newPipeBuf(),
			stdout: newPipeBuf(),
			stderr: newPipeBuf(),
			code:   0,
			done:   make(chan struct{}),
		}
		close(fp.done)
		_ = fp.stdout.Close()
		_ = fp.stderr.Close()
		close(done)
		return fp, nil
	}
	_ = sess.HandleMessage(&VersionMessage{ProtocolVersion: 1, SoftwareVersion: "t"})
	_ = sess.HandleMessage(&ExecMessage{Cmdline: []string{"/bin/attacker", "arg"}})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	if len(got) != 1 || got[0] != "/bin/forced" {
		t.Fatalf("forced command ignored remote argv incorrectly: %#v", got)
	}
	t.Log("PROVED forced command ignores remote argv")
}

func TestOracleStreamBombRejected(t *testing.T) {
	// Directly test decompressBounded with oversize.
	big := bytes.Repeat([]byte("Z"), MaxDecompressed+64)
	comp, ok := compressMaybe(big)
	if !ok {
		// force raw path: Unpack with compressed bit and invalid/oversize inflate
		t.Skip("compressor did not shrink payload")
	}
	_, err := decompressBounded(comp, MaxDecompressed)
	if err != ErrDecompressBomb {
		t.Fatalf("want ErrDecompressBomb got %v", err)
	}
	t.Log("PROVED stream bomb rejected")
}

func TestSessionHappyPathListener(t *testing.T) {
	send := &memSender{}
	hash := []byte{0xaa, 0xbb}
	sess := NewSession(Config{
		Listener:   true,
		Allowed:    [][]byte{hash},
		DefaultCmd: []string{"echo"},
	}, send)
	started := make(chan ExecRequest, 1)
	sess.StartProcess = func(req ExecRequest) (ProcessHandle, error) {
		started <- req
		fp := &fakeProc{
			stdin: newPipeBuf(), stdout: newPipeBuf(), stderr: newPipeBuf(),
			code: 0, done: make(chan struct{}),
		}
		_, _ = fp.stdout.Write([]byte("ok\n"))
		_ = fp.stdout.Close()
		_ = fp.stderr.Close()
		close(fp.done)
		return fp, nil
	}
	if !sess.SetRemoteIdentity(hash) {
		t.Fatal("allow failed")
	}
	if err := sess.HandleMessage(&VersionMessage{ProtocolVersion: 1, SoftwareVersion: "c"}); err != nil {
		t.Fatal(err)
	}
	if sess.State() != StateWaitCmd {
		t.Fatalf("state=%s", sess.State())
	}
	if err := sess.HandleMessage(&ExecMessage{Cmdline: []string{"echo", "hi"}, PipeStdin: true, PipeStdout: true, PipeStderr: true}); err != nil {
		t.Fatal(err)
	}
	select {
	case req := <-started:
		if req.Cmdline[0] != "echo" {
			t.Fatalf("%v", req.Cmdline)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no start")
	}
}
