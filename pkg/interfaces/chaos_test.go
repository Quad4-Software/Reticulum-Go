// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"io"
	"math/rand/v2"
	"net"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/backbone"
	"quad4/reticulum-go/pkg/common"
)

func skipIfaceChaosIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping interface chaos in -short mode")
	}
}

func writeHDLCFrame(w io.Writer, payload []byte) error {
	frame := appendFrameHDLC(nil, payload)
	_, err := w.Write(frame)
	return err
}

// TestIfaceChaosTCPCorruptHDLC injects bit-flips, truncates, and duplicate
// flags into a TCP client HDLC stream mixed with good frames.
func TestIfaceChaosTCPCorruptHDLC(t *testing.T) {
	skipIfaceChaosIfShort(t)

	tc := newTestTCPClient()
	server, client := net.Pipe()
	tc.Mutex.Lock()
	tc.conn = server
	tc.Mutex.Unlock()

	var received atomic.Int64
	var mu sync.Mutex
	seen := make(map[string]int)
	tc.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		received.Add(1)
		mu.Lock()
		seen[string(data)]++
		mu.Unlock()
	})

	doneRead := make(chan struct{})
	go func() {
		defer close(doneRead)
		tc.readLoop()
	}()

	rng := rand.New(rand.NewPCG(0x1face001, 0x1face002))
	const goodN = 12
	good := make([][]byte, goodN)
	for i := range good {
		// TCP HDLC drops frames below reticulumHeaderMinSize (19).
		good[i] = bytes.Repeat([]byte{byte(0x40 + i)}, 20+i)
	}

	go func() {
		defer client.Close()
		for i, payload := range good {
			if rng.Float64() < 0.4 {
				// Truncated / junk before a good frame.
				junk := make([]byte, 1+rng.IntN(24))
				for j := range junk {
					junk[j] = byte(rng.IntN(256))
				}
				_, _ = client.Write(junk)
			}
			if rng.Float64() < 0.3 {
				_, _ = client.Write([]byte{HDLCFlag, HDLCFlag})
			}
			frame := appendFrameHDLC(nil, payload)
			if rng.Float64() < 0.35 && len(frame) > 4 {
				cp := append([]byte(nil), frame...)
				cp[2+rng.IntN(len(cp)-3)] ^= byte(1 + rng.IntN(255))
				_, _ = client.Write(cp)
			}
			if err := writeHDLCFrame(client, payload); err != nil {
				return
			}
			if i%3 == 0 {
				time.Sleep(2 * time.Millisecond)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}()

	select {
	case <-doneRead:
	case <-time.After(10 * time.Second):
		t.Fatal("readLoop did not return after pipe close")
	}

	if got := received.Load(); got < int64(goodN/2) {
		t.Fatalf("expected at least %d good frames after corrupt stream, got %d", goodN/2, got)
	}
	mu.Lock()
	defer mu.Unlock()
	matched := 0
	for _, payload := range good {
		if seen[string(payload)] > 0 {
			matched++
		}
	}
	if matched < goodN/2 {
		t.Fatalf("only %d/%d known payloads survived corrupt HDLC", matched, goodN)
	}
}

// TestIfaceChaosTCPReorderBurst buffers framed packets and flushes them in
// shuffled order with interleaved junk so the decoder must resync.
func TestIfaceChaosTCPReorderBurst(t *testing.T) {
	skipIfaceChaosIfShort(t)

	tc := newTestTCPClient()
	server, client := net.Pipe()
	tc.Mutex.Lock()
	tc.conn = server
	tc.Mutex.Unlock()

	var received atomic.Int64
	got := make(chan []byte, 64)
	tc.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		received.Add(1)
		select {
		case got <- append([]byte(nil), data...):
		default:
		}
	})

	doneRead := make(chan struct{})
	go func() {
		defer close(doneRead)
		tc.readLoop()
	}()

	payloads := [][]byte{
		bytes.Repeat([]byte{0x01}, 24),
		bytes.Repeat([]byte{0x2B}, 32),
		bytes.Repeat([]byte{0x3D}, 24),
		bytes.Repeat([]byte{0x55}, 64),
	}

	go func() {
		defer client.Close()
		frames := make([][]byte, len(payloads))
		for i, payload := range payloads {
			frames[i] = appendFrameHDLC(nil, payload)
		}
		rng := rand.New(rand.NewPCG(0x1face011, 0x1face012))
		rng.Shuffle(len(frames), func(i, j int) { frames[i], frames[j] = frames[j], frames[i] })
		for i, frame := range frames {
			if i > 0 {
				_, _ = client.Write([]byte{0x00, 0xFF, HDLCEsc, 0x11})
			}
			if _, err := client.Write(frame); err != nil {
				return
			}
		}
		time.Sleep(30 * time.Millisecond)
	}()

	deadline := time.After(5 * time.Second)
	seen := 0
	want := make(map[string]bool, len(payloads))
	for _, p := range payloads {
		want[string(p)] = false
	}
	for seen < len(payloads) {
		select {
		case data := <-got:
			if marked, ok := want[string(data)]; ok && !marked {
				want[string(data)] = true
				seen++
			}
		case <-deadline:
			t.Fatalf("timeout waiting for reordered HDLC frames (got %d, rx=%d)", seen, received.Load())
		}
	}

	select {
	case <-doneRead:
	case <-time.After(5 * time.Second):
		t.Fatal("readLoop did not exit")
	}
}

// TestIfaceChaosPipeFlapRespawn kills the pipe subprocess mid-traffic and
// asserts respawn restores round-trip delivery.
func TestIfaceChaosPipeFlapRespawn(t *testing.T) {
	skipIfaceChaosIfShort(t)
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not available")
	}

	pi, err := NewPipeInterface("chaos-pipe", "cat", true, 50*time.Millisecond, false)
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

	payload := []byte{0x01, 0x02, HDLCFlag, 0x03}
	if err := pi.Send(payload, ""); err != nil {
		t.Fatalf("initial Send: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && received.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if received.Load() == 0 {
		t.Fatal("expected initial echo before flap")
	}

	pi.killProcess()
	// killProcess does not clear Online. Wait for the read-loop exit path to
	// mark offline, then for respawn to bring the pipe back.
	deadline = time.Now().Add(3 * time.Second)
	sawOffline := false
	for time.Now().Before(deadline) {
		pi.Mutex.RLock()
		online := pi.Online
		pi.Mutex.RUnlock()
		if !online {
			sawOffline = true
		}
		if sawOffline && online {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	pi.Mutex.RLock()
	online := pi.Online
	pi.Mutex.RUnlock()
	if !sawOffline {
		t.Fatal("expected pipe to go offline after kill before respawn")
	}
	if !online {
		t.Fatal("pipe did not come back online after flap")
	}

	received.Store(0)
	// First byte must clear 0x80 so ApplyIFACInbound does not
	// treat the frame as IFAC without an identity configured.
	after := []byte{0x4A, 0x7E, 0x4B, 0x4C}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := pi.Send(after, ""); err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if received.Load() > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if received.Load() == 0 {
		t.Fatal("no echo after pipe respawn")
	}
	mu.Lock()
	defer mu.Unlock()
	if !bytes.Equal(last, after) {
		t.Fatalf("post-respawn payload = %x, want %x", last, after)
	}
}

// TestIfaceChaosLocalCorruptFrame feeds truncated and corrupt HDLC then a
// good frame into a local client read loop.
func TestIfaceChaosLocalCorruptFrame(t *testing.T) {
	skipIfaceChaosIfShort(t)

	readEnd, writeEnd := net.Pipe()
	lc := newLocalClientFromConn("chaos-local", readEnd, nil, false)
	lc.In = true
	lc.MTU = 262144

	got := make(chan []byte, 4)
	lc.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got <- append([]byte(nil), data...)
	})
	go lc.readLoop()

	// Incomplete frame (no closing flag), then a clean frame.
	_, _ = writeEnd.Write([]byte{HDLCFlag, 0xDE, 0xAD, 0xBE})
	good := []byte{0x42, 0x43, 0x44}
	if err := writeHDLCFrame(writeEnd, good); err != nil {
		t.Fatalf("write good frame: %v", err)
	}

	deadline := time.After(3 * time.Second)
	for {
		select {
		case rcv := <-got:
			if bytes.Equal(rcv, good) {
				_ = lc.Stop()
				_ = writeEnd.Close()
				return
			}
		case <-deadline:
			t.Fatal("timeout waiting for good frame after corrupt HDLC")
		}
	}
}

// TestIfaceChaosBackboneClientFlap drops the client connection mid-session
// and expects reconnect plus a successful round-trip afterward.
func TestIfaceChaosBackboneClientFlap(t *testing.T) {
	skipIfaceChaosIfShort(t)
	hub := testBackboneHubBackend(t, backbone.BackendGo)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	server, err := NewBackboneInterface("chaos-bb-srv", &common.InterfaceConfig{
		Enabled: true,
		Address: "127.0.0.1",
		Port:    port,
	}, hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Stop() })

	var received atomic.Int64
	server.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		if len(data) >= 1 && data[0] == 0x42 {
			received.Add(1)
		}
	})

	client, err := NewBackboneClientInterface("chaos-bb-cli", &common.InterfaceConfig{
		Enabled:        true,
		TargetHost:     "127.0.0.1",
		TargetPort:     port,
		MaxReconnTries: 20,
	}, hub)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Stop() })

	waitInterfaceOnline(t, client, 5*time.Second)
	payload := bytes.Repeat([]byte{0x42, 0x43, 0x44}, 8)
	if err := client.Send(payload, ""); err != nil {
		t.Fatalf("pre-flap Send: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && received.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if received.Load() == 0 {
		t.Fatal("server did not receive pre-flap packet")
	}

	client.Mutex.Lock()
	conn := client.conn
	client.Mutex.Unlock()
	if conn != nil {
		_ = conn.Close()
	}

	waitInterfaceOnline(t, client, 10*time.Second)
	received.Store(0)
	deadline = time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		_ = client.Send(payload, "")
		if received.Load() > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if received.Load() == 0 {
		t.Fatal("server did not receive packet after backbone client flap")
	}
}
