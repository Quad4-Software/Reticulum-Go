// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live pipe interop: Go PipeInterface with Python HDLC echo helper.
// Set RUN_LIVE_INTEROP=1.

package interop

import (
	"bytes"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/interfaces"
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
