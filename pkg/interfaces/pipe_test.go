// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestSplitPipeCommand(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"cat", []string{"cat"}},
		{"netcat -l 5757", []string{"netcat", "-l", "5757"}},
		{`echo "hello world"`, []string{"echo", "hello world"}},
	}
	for _, tc := range tests {
		got, err := splitPipeCommand(tc.in)
		if err != nil {
			t.Fatalf("splitPipeCommand(%q): %v", tc.in, err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("splitPipeCommand(%q) = %v, want %v", tc.in, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("splitPipeCommand(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

func TestNewPipeInterfaceRequiresCommand(t *testing.T) {
	_, err := NewPipeInterface("pipe", "", true, 0, false)
	if err == nil || !strings.Contains(err.Error(), "command") {
		t.Fatalf("expected command error, got %v", err)
	}
}

func TestPipeInterfaceHDLCRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not available")
	}
	pi, err := NewPipeInterface("test-pipe", "cat", true, time.Second, false)
	if err != nil {
		t.Fatalf("NewPipeInterface: %v", err)
	}
	defer pi.Stop()

	var received atomic.Int32
	var last []byte
	var mu sync.Mutex
	pi.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		received.Add(1)
		mu.Lock()
		last = append([]byte(nil), data...)
		mu.Unlock()
	})

	payload := []byte{0x01, 0x02, 0x7e, 0x03}
	if err := pi.Send(payload, ""); err != nil {
		t.Fatalf("Send: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if received.Load() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if received.Load() == 0 {
		t.Fatal("expected echoed packet")
	}
	mu.Lock()
	got := append([]byte(nil), last...)
	mu.Unlock()
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload = %x, want %x", got, payload)
	}
}

func TestPipeInterfaceRespawnAfterExit(t *testing.T) {
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not available")
	}
	pi, err := NewPipeInterface("respawn", "cat", true, 50*time.Millisecond, false)
	if err != nil {
		t.Fatalf("NewPipeInterface: %v", err)
	}
	defer pi.Stop()

	pi.killProcess()
	time.Sleep(150 * time.Millisecond)
	pi.Mutex.RLock()
	online := pi.Online
	pi.Mutex.RUnlock()
	if !online {
		t.Fatal("expected pipe to respawn and come back online")
	}
}

func TestNewFromConfigPipeInterface(t *testing.T) {
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not available")
	}
	iface, err := NewFromConfig("pipe", &common.InterfaceConfig{
		Type:    "PipeInterface",
		Enabled: true,
		Command: "cat",
	})
	if err != nil {
		t.Fatalf("NewFromConfig: %v", err)
	}
	pi, ok := iface.(*PipeInterface)
	if !ok {
		t.Fatalf("expected *PipeInterface, got %T", iface)
	}
	defer pi.Stop()
	if pi.MTU != pipeHWMTU {
		t.Fatalf("MTU = %d, want %d", pi.MTU, pipeHWMTU)
	}
}

func TestNewFromConfigLocalInterface(t *testing.T) {
	iface, err := NewFromConfig("local", &common.InterfaceConfig{
		Type:    "LocalInterface",
		Enabled: true,
		Port:    43777,
	})
	if err != nil {
		t.Fatalf("NewFromConfig: %v", err)
	}
	lc, ok := iface.(*LocalClientInterface)
	if !ok {
		t.Fatalf("expected *LocalClientInterface, got %T", iface)
	}
	if lc.targetPort != 43777 {
		t.Fatalf("targetPort = %d, want 43777", lc.targetPort)
	}
	if lc.ShouldIngressLimitPR() {
		t.Fatal("local client should not ingress-limit path requests")
	}
}

func TestPipeInterfaceReadLoopValidFrame(t *testing.T) {
	pi := &PipeInterface{
		BaseInterface: NewBaseInterface("valid", common.IFTypePipe, true),
		done:          make(chan struct{}),
	}
	pi.MTU = pipeHWMTU
	pi.Online = true

	readEnd, writeEnd := io.Pipe()
	pi.stdout = readEnd

	var frames atomic.Int32
	pi.SetPacketCallback(func([]byte, common.NetworkInterface) {
		frames.Add(1)
	})
	go pi.readLoop()

	if _, err := writeEnd.Write([]byte{HDLCFlag, 0x42, 0x43, HDLCFlag}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if frames.Load() != 1 {
		t.Fatalf("expected 1 frame, got %d", frames.Load())
	}
}

func TestPipeInterfaceReadLoopBombLimit(t *testing.T) {
	pi := &PipeInterface{
		BaseInterface: NewBaseInterface("bomb", common.IFTypePipe, true),
		done:          make(chan struct{}),
	}
	pi.MTU = pipeHWMTU
	pi.Online = true

	readEnd, writeEnd := io.Pipe()
	_, inW := io.Pipe()
	pi.stdout = readEnd
	pi.stdin = inW

	var frames atomic.Int32
	pi.SetPacketCallback(func([]byte, common.NetworkInterface) {
		frames.Add(1)
	})

	go pi.readLoop()

	bomb := bytes.Repeat([]byte{0xAA}, 4*1024)
	all := append([]byte{HDLCFlag}, bomb...)
	all = append(all, []byte{HDLCFlag, 0x42, 0x43, HDLCFlag}...)
	if _, err := writeEnd.Write(all); err != nil {
		t.Fatalf("write: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if frames.Load() != 1 {
		t.Fatalf("expected 1 valid frame, got %d", frames.Load())
	}
	pi.Enabled = false
	_ = writeEnd.Close()
}

func TestPipeInterfaceStopClosesDone(t *testing.T) {
	pi := &PipeInterface{
		BaseInterface: NewBaseInterface("stop", common.IFTypePipe, true),
		done:          make(chan struct{}),
	}
	if err := pi.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case <-pi.done:
	default:
		t.Fatal("done channel should be closed")
	}
}

func TestPipeInterfaceDisabledDoesNotStart(t *testing.T) {
	pi, err := NewPipeInterface("off", "cat", false, time.Second, false)
	if err != nil {
		t.Fatalf("NewPipeInterface: %v", err)
	}
	if pi.Online {
		t.Fatal("disabled pipe should not be online")
	}
}

func TestPipeCommandFromEnv(t *testing.T) {
	if os.Getenv("RUN_PIPE_ECHO") != "1" {
		t.Skip("set RUN_PIPE_ECHO=1 to run subprocess echo test")
	}
	pi, err := NewPipeInterface("echo", "cat", true, time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	defer pi.Stop()
}
