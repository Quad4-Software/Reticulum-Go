// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live pipe interop: Go PipeInterface with Python HDLC echo helper.
// Set RUN_LIVE_INTEROP=1.

package interop

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
	rlink "github.com/Quad4-Software/Reticulum-Go/pkg/link"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

func TestLiveInteropPipeInterfacePythonEcho(t *testing.T) {
	liveOrSkip(t)
	if _, err := exec.LookPath(pythonExe()); err != nil {
		t.Skip("python not available")
	}

	echoScript := pyScript(t, "pipe_echo.py")
	pi, err := interfaces.NewPipeInterface("pipe", pythonExe()+" "+echoScript, true, 2*time.Second, false)
	if err != nil {
		t.Fatalf("NewPipeInterface: %v", err)
	}
	defer pi.Stop()

	var received atomic.Int32
	var last []byte
	pi.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		received.Add(1)
		last = append([]byte(nil), data...)
	})

	payload := []byte{0x01, 0x02, 0x03, 0x04}
	if err := pi.Send(payload, ""); err != nil {
		t.Fatalf("Send: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if received.Load() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if received.Load() == 0 {
		t.Fatal("expected echoed packet from python pipe_echo.py")
	}
	if !bytes.Equal(last, payload) {
		t.Fatalf("payload = %x, want %x", last, payload)
	}
}

// writePythonTCPServerConfig writes a Python Reticulum config whose only
// interface is a TCPServerInterface on 127.0.0.1:port.
func writePythonTCPServerConfig(t *testing.T, dir string, listenPort int) {
	t.Helper()
	cfg := strings.Join([]string{
		"[reticulum]",
		"enable_transport = false",
		"share_instance = no",
		"loglevel = 4",
		"",
		"[interfaces]",
		"",
		"[[py_tcp_srv]]",
		"type = TCPServerInterface",
		"enabled = yes",
		"listen_ip = 127.0.0.1",
		"listen_port = " + strconv.Itoa(listenPort),
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestLiveInteropPipeFullStackPython runs a full RNS session through the Go
// PipeInterface: the pipe subprocess shuttles HDLC bytes over TCP to a Python
// TCPServerInterface, then a Python announce is learned and a link echo
// round-trips.
func TestLiveInteropPipeFullStackPython(t *testing.T) {
	liveOrSkip(t)
	if _, err := exec.LookPath(pythonExe()); err != nil {
		t.Skip("python not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), pyProcShortTimeout)
	defer cancel()

	// Python RNS node listening on a TCP server interface.
	port := freeTCPPort(t)
	pyCfg := t.TempDir()
	writePythonTCPServerConfig(t, pyCfg, port)
	cmd, _, pyHash := startPythonEchoPeer(t, ctx, pyCfg)
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// Go PipeInterface spawns a bridge subprocess carrying its HDLC bytes to
	// the Python TCP server.
	bridge := pyScript(t, "pipe_tcp_bridge.py")
	pi, err := interfaces.NewPipeInterface("go_pipe",
		pythonExe()+" "+bridge+" 127.0.0.1 "+strconv.Itoa(port),
		true, 2*time.Second, false)
	if err != nil {
		t.Fatalf("NewPipeInterface: %v", err)
	}
	defer pi.Stop()

	tr := transport.NewTransport(&common.ReticulumConfig{})
	defer tr.Close()
	if err := tr.RegisterInterface("go_pipe", pi); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := tr.InitializePathRequestHandler(); err != nil {
		t.Fatalf("path handler: %v", err)
	}

	if err := waitPathOn(ctx, tr, pyHash, "go_pipe", 45*time.Second); err != nil {
		t.Fatalf("path to python over pipe: %v", err)
	}

	srvID, err := identity.Recall(pyHash)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	destOut, err := destination.FromHash(pyHash, srvID, destination.Single, tr)
	if err != nil {
		t.Fatalf("from hash: %v", err)
	}
	established := make(chan struct{})
	lnk := rlink.NewLink(destOut, tr, pi, func(_ *rlink.Link) {
		close(established)
	}, nil)
	defer lnk.Teardown()
	echoed := make(chan struct{})
	lnk.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		close(echoed)
	})
	if err := lnk.Establish(); err != nil {
		t.Fatalf("establish: %v", err)
	}
	select {
	case <-established:
	case <-time.After(45 * time.Second):
		t.Fatal("link establish timeout over pipe")
	}
	lnk.Start()
	if err := lnk.SendPacket([]byte("pipe-echo")); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case <-echoed:
	case <-time.After(30 * time.Second):
		t.Fatal("no echo from python over pipe")
	}
}
