// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rgosh

import (
	"bytes"
	"context"
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
	mu     sync.Mutex
	killed bool
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
	f.mu.Lock()
	f.killed = true
	f.mu.Unlock()
	select {
	case <-f.done:
	default:
		close(f.done)
	}
	_ = f.stdout.Close()
	_ = f.stderr.Close()
	return nil
}

func (f *fakeProc) wasKilled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.killed
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
	spawned := false
	sess.StartProcess = func(req ExecRequest) (ProcessHandle, error) {
		spawned = true
		return nil, io.EOF
	}
	_ = sess.HandleMessage(&VersionMessage{ProtocolVersion: 1, SoftwareVersion: "t"})
	err := sess.HandleMessage(&ExecMessage{Cmdline: []string{"/bin/attacker", "arg"}})
	if err == nil {
		t.Fatal("expected reject")
	}
	if spawned {
		t.Fatal("must not spawn when remote cmdline is forbidden")
	}
	if send.ofType(NativeError) == 0 {
		t.Fatal("expected Error")
	}
	t.Log("PROVED -C rejects remote argv")
}

func TestOracleForcedCommandDefault(t *testing.T) {
	send := &memSender{}
	sess := NewSession(Config{
		Listener:      true,
		AllowAll:      true,
		ForcedCommand: true,
		DefaultCmd:    []string{"/bin/forced"},
	}, send)
	started := make(chan []string, 1)
	sess.StartProcess = func(req ExecRequest) (ProcessHandle, error) {
		started <- append([]string(nil), req.Cmdline...)
		fp := &fakeProc{
			stdin: newPipeBuf(), stdout: newPipeBuf(), stderr: newPipeBuf(),
			code: 0, done: make(chan struct{}),
		}
		close(fp.done)
		_ = fp.stdout.Close()
		_ = fp.stderr.Close()
		return fp, nil
	}
	_ = sess.HandleMessage(&VersionMessage{ProtocolVersion: 1, SoftwareVersion: "t"})
	if err := sess.HandleMessage(&ExecMessage{}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-started:
		if len(got) != 1 || got[0] != "/bin/forced" {
			t.Fatalf("%v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestOracleRemoteCmdAsArgs(t *testing.T) {
	send := &memSender{}
	sess := NewSession(Config{
		Listener:        true,
		AllowAll:        true,
		RemoteCmdAsArgs: true,
		DefaultCmd:      []string{"/bin/wrapper"},
	}, send)
	started := make(chan []string, 1)
	sess.StartProcess = func(req ExecRequest) (ProcessHandle, error) {
		started <- append([]string(nil), req.Cmdline...)
		fp := &fakeProc{
			stdin: newPipeBuf(), stdout: newPipeBuf(), stderr: newPipeBuf(),
			code: 0, done: make(chan struct{}),
		}
		close(fp.done)
		_ = fp.stdout.Close()
		_ = fp.stderr.Close()
		return fp, nil
	}
	_ = sess.HandleMessage(&VersionMessage{ProtocolVersion: 1, SoftwareVersion: "t"})
	if err := sess.HandleMessage(&ExecMessage{Cmdline: []string{"a", "b"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-started:
		want := []string{"/bin/wrapper", "a", "b"}
		if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
			t.Fatalf("%v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestOracleLongLivedNotKilled(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	send := &memSender{}
	sess := NewSession(Config{Listener: true, AllowAll: true, DefaultCmd: []string{"sleep"}}, send)
	fp := &fakeProc{
		stdin: newPipeBuf(), stdout: newPipeBuf(), stderr: newPipeBuf(),
		code: 0, done: make(chan struct{}),
	}
	sess.StartProcess = func(req ExecRequest) (ProcessHandle, error) {
		return fp, nil
	}
	_ = sess.HandleMessage(&VersionMessage{ProtocolVersion: 1, SoftwareVersion: "t"})
	if err := sess.HandleMessage(&ExecMessage{Cmdline: []string{"sleep"}}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(6 * time.Second)
	if fp.wasKilled() {
		t.Fatal("process killed before exit")
	}
	if sess.State() != StateRunning {
		t.Fatalf("state=%s", sess.State())
	}
	sess.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fp.wasKilled() {
			t.Log("PROVED long-lived process survives 6s then Close kills")
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("Close did not kill process")
}

func TestOraclePacingWaitReady(t *testing.T) {
	gate := make(chan struct{})
	var n atomicInt
	s := &paceSender{gate: gate, n: &n}
	sess := NewSession(Config{Listener: true, AllowAll: true, DefaultCmd: []string{"x"}}, s)
	fp := &fakeProc{
		stdin: newPipeBuf(), stdout: newPipeBuf(), stderr: newPipeBuf(),
		code: 0, done: make(chan struct{}),
	}
	sess.StartProcess = func(req ExecRequest) (ProcessHandle, error) {
		return fp, nil
	}
	_ = sess.HandleMessage(&VersionMessage{ProtocolVersion: 1, SoftwareVersion: "t"})
	_ = sess.HandleMessage(&ExecMessage{Cmdline: []string{"x"}, PipeStdout: true, PipeStderr: true, PipeStdin: true})
	base := n.load()
	_, _ = fp.stdout.Write(bytes.Repeat([]byte("a"), 200))
	time.Sleep(150 * time.Millisecond)
	if n.load() != base {
		t.Fatalf("sends=%d want %d while not ready", n.load(), base)
	}
	close(gate)
	_ = fp.stdout.Close()
	_ = fp.stderr.Close()
	close(fp.done)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && n.load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if n.load() == 0 {
		t.Fatal("no sends after ready")
	}
}

func TestCompressAdaptiveFitsMDU(t *testing.T) {
	buf := bytes.Repeat([]byte("hello world "), 400)
	maxData := 80
	chunk, n, _ := compressAdaptive(buf, maxData)
	if n < 1 {
		t.Fatal("consumed 0")
	}
	if len(chunk) > maxData-StreamHeaderSize {
		t.Fatalf("chunk %d > payload %d", len(chunk), maxData-StreamHeaderSize)
	}
}

type atomicInt struct {
	mu sync.Mutex
	v  int
}

func (a *atomicInt) add(d int) {
	a.mu.Lock()
	a.v += d
	a.mu.Unlock()
}

func (a *atomicInt) load() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.v
}

type paceSender struct {
	gate chan struct{}
	n    *atomicInt
	mu   sync.Mutex
	msgs []Message
}

func (p *paceSender) Send(msg Message) error {
	p.mu.Lock()
	p.msgs = append(p.msgs, msg)
	p.mu.Unlock()
	p.n.add(1)
	return nil
}

func (p *paceSender) WaitReady(ctx context.Context) error {
	select {
	case <-p.gate:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *paceSender) MDU() int { return 64 }

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
