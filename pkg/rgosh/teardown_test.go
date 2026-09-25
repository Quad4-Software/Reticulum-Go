package rgosh

import (
	"testing"
	"time"
)

// TestDenyFiresTeardown verifies every protocol-deny path releases the link:
// without OnTeardown, an unauthenticated peer can park registered links until
// the node's incoming-link ceiling is reached.
func TestDenyFiresTeardown(t *testing.T) {
	send := &memSender{}
	torn := false
	sess := NewSession(Config{
		Listener:   true,
		Allowed:    [][]byte{{1, 2, 3, 4}},
		DefaultCmd: []string{"/bin/true"},
	}, send)
	sess.OnTeardown = func() { torn = true }

	// An exec attempt with no identity hits denyProtocolLocked directly.
	_ = sess.HandleMessage(&ExecMessage{Cmdline: []string{"/bin/evil"}})
	if !torn {
		t.Fatal("denied session never invoked OnTeardown; link would stay registered")
	}
	if sess.State() != StateTeardown {
		t.Fatalf("state=%s", sess.State())
	}
}

// TestListenerExitFiresTeardown verifies a finished listener session releases
// the link instead of leaving it registered until the peer disconnects.
func TestListenerExitFiresTeardown(t *testing.T) {
	send := &memSender{}
	exited := make(chan int, 1)
	torn := make(chan struct{})
	proc := &fakeProc{
		stdin:  newPipeBuf(),
		stdout: newPipeBuf(),
		stderr: newPipeBuf(),
		done:   make(chan struct{}),
	}
	close(proc.done)
	_ = proc.stdout.Close()
	_ = proc.stderr.Close()

	sess := NewSession(Config{
		Listener:   true,
		AllowAll:   true,
		DefaultCmd: []string{"/bin/true"},
	}, send)
	sess.OnExit = func(code int) { exited <- code }
	sess.OnTeardown = func() { close(torn) }
	sess.StartProcess = func(ExecRequest) (ProcessHandle, error) {
		return proc, nil
	}

	_ = sess.HandleMessage(&VersionMessage{ProtocolVersion: 1})
	_ = sess.HandleMessage(&ExecMessage{})
	select {
	case <-torn:
	case <-time.After(5 * time.Second):
		t.Fatal("listener process exit never invoked OnTeardown")
	}
}
