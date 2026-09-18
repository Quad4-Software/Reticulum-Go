// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live serial interop: Go SerialInterface against Python HDLC peer on a PTY.
// Matches RNS SerialInterface framing. Set RUN_LIVE_INTEROP=1.
// Requires pyserial.

//go:build !js

package interop

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
	rlink "github.com/Quad4-Software/Reticulum-Go/pkg/link"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

func TestLiveInteropSerialPythonEcho(t *testing.T) {
	liveOrSkip(t)
	if _, err := exec.LookPath(pythonExe()); err != nil {
		t.Skip("python not available")
	}
	if err := exec.Command(pythonExe(), "-c", "import serial").Run(); err != nil {
		t.Skip("pyserial not available")
	}

	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	defer master.Close()
	slavePath := slave.Name()
	_ = slave.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	script := pyScript(t, "serial_echo.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"SERIAL_DEVICE="+slavePath,
		"SERIAL_SPEED=115200",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start python: %v", err)
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	readyCh := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := stdout.Read(buf)
		readyCh <- buf[:n]
	}()
	select {
	case out := <-readyCh:
		if !bytes.Contains(out, []byte("READY")) {
			t.Fatalf("python READY missing: %q", out)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for python READY")
	}

	si, err := interfaces.NewSerialInterface("go-serial", true, interfaces.SerialOptions{
		Device:    "pty-master",
		Speed:     115200,
		FrameIdle: 100 * time.Millisecond,
		Open: func(interfaces.SerialOptions) (interfaces.SerialPort, error) {
			return master, nil
		},
	})
	if err != nil {
		t.Fatalf("NewSerialInterface: %v", err)
	}
	defer si.Stop()

	var received atomic.Int32
	var last []byte
	si.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		received.Add(1)
		last = append([]byte(nil), data...)
	})

	payload := []byte{0x10, 0x7e, 0x7d, 0x20, 0x30}
	if err := si.Send(payload, ""); err != nil {
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
		t.Fatal("expected echoed packet from python serial_echo.py")
	}
	if !bytes.Equal(last, payload) {
		t.Fatalf("payload = %x, want %x", last, payload)
	}
}

// writePythonSerialConfig writes a Python Reticulum config with a
// SerialInterface on the given device path.
func writePythonSerialConfig(t *testing.T, dir, device string) {
	t.Helper()
	cfg := strings.Join([]string{
		"[reticulum]",
		"enable_transport = false",
		"share_instance = no",
		"loglevel = 4",
		"",
		"[interfaces]",
		"",
		"[[py_serial]]",
		"type = SerialInterface",
		"enabled = yes",
		"port = " + device,
		"speed = 115200",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestLiveInteropSerialRNSSession runs a full RNS stack over a PTY pair: Go
// SerialInterface on the master, Python SerialInterface on the slave, with an
// announce and a link echo.
func TestLiveInteropSerialRNSSession(t *testing.T) {
	liveOrSkip(t)
	if _, err := exec.LookPath(pythonExe()); err != nil {
		t.Skip("python not available")
	}
	if err := exec.Command(pythonExe(), "-c", "import serial").Run(); err != nil {
		t.Skip("pyserial not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), pyProcShortTimeout)
	defer cancel()

	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	defer master.Close()
	slavePath := slave.Name()
	// Keep the slave fd open for the whole test: with no slave open, reads on
	// the master return EIO and the serial interface tears itself down before
	// Python can open the device.
	defer slave.Close()

	tr := transport.NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	si, err := interfaces.NewSerialInterface("go_serial", true, interfaces.SerialOptions{
		Device:    "pty-master",
		Speed:     115200,
		FrameIdle: 100 * time.Millisecond,
		Open: func(interfaces.SerialOptions) (interfaces.SerialPort, error) {
			return master, nil
		},
	})
	if err != nil {
		t.Fatalf("NewSerialInterface: %v", err)
	}
	if err := tr.RegisterInterface("go_serial", si); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := si.Start(); err != nil {
		t.Fatalf("start serial: %v", err)
	}
	if err := tr.InitializePathRequestHandler(); err != nil {
		t.Fatalf("path handler: %v", err)
	}

	pyCfg := t.TempDir()
	writePythonSerialConfig(t, pyCfg, slavePath)
	cmd, _, pyHash := startPythonEchoPeer(t, ctx, pyCfg)
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	if err := waitPathOn(ctx, tr, pyHash, "go_serial", 45*time.Second); err != nil {
		t.Fatalf("path to python over serial: %v", err)
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
	lnk := rlink.NewLink(destOut, tr, si, func(_ *rlink.Link) {
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
		t.Fatal("link establish timeout over serial")
	}
	lnk.Start()
	if err := lnk.SendPacket([]byte("serial-echo")); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case <-echoed:
	case <-time.After(30 * time.Second):
		t.Fatal("no echo from python over serial")
	}
}
