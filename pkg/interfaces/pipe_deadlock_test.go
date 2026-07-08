// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"io"
	"net"
	"os/exec"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestPipeInterfaceStopDoesNotDeadlockWithReadLoop(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not available")
	}
	pi, err := NewPipeInterface("deadlock", "cat", true, time.Second, false)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = pi.Stop()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop blocked >5s (possible deadlock with readLoop)")
	}
}

func TestLocalClientStopDoesNotDeadlockWithReadLoop(t *testing.T) {
	server, client := net.Pipe()
	lc := &LocalClientInterface{
		BaseInterface: NewBaseInterface("local-deadlock", common.IFTypeUnix, true),
		conn:          client,
		done:          make(chan struct{}),
	}
	lc.MTU = 262144
	lc.Online = true

	go lc.readLoop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = lc.Stop()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("LocalClient Stop blocked >5s")
	}
	_ = server.Close()
}

func TestPipeInterfaceRespawnDoesNotDeadlock(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	pr, pw := io.Pipe()
	pi := &PipeInterface{
		BaseInterface: NewBaseInterface("respawn-deadlock", common.IFTypePipe, true),
		command:       "cat",
		respawnDelay:  10 * time.Millisecond,
		done:          make(chan struct{}),
		stdout:        pr,
		stdin:         pw,
	}
	pi.Online = true

	var wg sync.WaitGroup
	wg.Go(func() {
		pi.readLoop()
	})
	_ = pw.Close()

	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("readLoop blocked >5s after pipe close")
	}
	pi.Enabled = false
	_ = pi.Stop()
}

func TestLocalServerSpawnHookDoesNotDeadlock(t *testing.T) {
	ln, err := NewLocalServerInterface(0, "", false, func(*LocalClientInterface) {}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ln.spawnHook == nil {
		t.Fatal("expected spawn hook")
	}
}
