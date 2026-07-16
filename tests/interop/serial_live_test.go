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
	"sync/atomic"
	"testing"
	"time"

	"github.com/creack/pty"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/interfaces"
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
