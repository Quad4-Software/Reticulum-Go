// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func newTestTCPClient() *TCPClientInterface {
	tc := &TCPClientInterface{
		BaseInterface:  NewBaseInterface("hdlc-bomb-test", common.IFTypeTCP, true),
		kissFraming:    false,
		initiator:      false,
		neverConnected: false,
		packetBuffer:   make([]byte, 0),
		done:           make(chan struct{}),
	}
	tc.MTU = 1064
	tc.Online = true
	return tc
}

func TestTCPClient_HDLCFramingBombDoesNotOverflow(t *testing.T) {
	tc := newTestTCPClient()

	server, client := net.Pipe()
	tc.Mutex.Lock()
	tc.conn = server
	tc.Mutex.Unlock()

	var received atomic.Int64
	var lastSize atomic.Int64
	var mu sync.Mutex
	var lastPayload []byte
	tc.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		received.Add(1)
		lastSize.Store(int64(len(data)))
		mu.Lock()
		lastPayload = append([]byte(nil), data...)
		mu.Unlock()
	})

	doneRead := make(chan struct{})
	go func() {
		defer close(doneRead)
		tc.readLoop()
	}()

	const bombSize = 4 * 1024 * 1024
	bomb := make([]byte, bombSize)
	for i := range bomb {
		bomb[i] = 0xAA
	}
	open := []byte{HDLCFlag}

	memBefore := readAlloc()

	go func() {
		_, _ = client.Write(open)
		_, _ = client.Write(bomb)

		valid := append([]byte{HDLCFlag}, bytes.Repeat([]byte{0x42}, 20)...)
		valid = append(valid, HDLCFlag)
		_, _ = client.Write(valid)
		time.Sleep(50 * time.Millisecond)
		_ = client.Close()
	}()

	select {
	case <-doneRead:
	case <-time.After(10 * time.Second):
		t.Fatal("readLoop did not return after pipe close")
	}

	memAfter := readAlloc()

	maxHDLC := uint64(2*tc.MTU + 32)
	growth := uint64(0)
	if memAfter > memBefore {
		growth = memAfter - memBefore
	}
	memBudget := max(maxHDLC*8, 1<<20)
	if growth > memBudget {
		t.Fatalf("readLoop retained %d bytes after %d-byte bomb (budget=%d, maxHDLC=%d)",
			growth, bombSize, memBudget, maxHDLC)
	}

	if got := received.Load(); got != 1 {
		t.Fatalf("expected 1 valid packet after bomb, got %d", got)
	}
	mu.Lock()
	defer mu.Unlock()
	want := bytes.Repeat([]byte{0x42}, 20)
	if !bytes.Equal(lastPayload, want) {
		t.Fatalf("post-bomb frame mismatch: %x", lastPayload)
	}
	if lastSize.Load() != 20 {
		t.Fatalf("post-bomb frame size mismatch: %d", lastSize.Load())
	}
}

func TestTCPClient_HDLCEscapeBombDoesNotOverflow(t *testing.T) {
	tc := newTestTCPClient()

	server, client := net.Pipe()
	tc.Mutex.Lock()
	tc.conn = server
	tc.Mutex.Unlock()

	var received atomic.Int64
	tc.SetPacketCallback(func(_ []byte, _ common.NetworkInterface) {
		received.Add(1)
	})

	doneRead := make(chan struct{})
	go func() {
		defer close(doneRead)
		tc.readLoop()
	}()

	const bombSize = 1024 * 1024
	bomb := make([]byte, 0, bombSize*2)
	for range bombSize {
		bomb = append(bomb, HDLCEsc, 0x55^HDLCEscMask)
	}

	go func() {
		_, _ = client.Write([]byte{HDLCFlag})
		_, _ = client.Write(bomb)
		_, _ = client.Write([]byte{HDLCFlag})
		time.Sleep(50 * time.Millisecond)
		_ = client.Close()
	}()

	select {
	case <-doneRead:
	case <-time.After(10 * time.Second):
		t.Fatal("readLoop did not return after pipe close")
	}
}

func TestTCPServer_HDLCFramingBombDoesNotOverflow(t *testing.T) {
	ts := &TCPServerInterface{
		BaseInterface: NewBaseInterface("hdlc-bomb-server", common.IFTypeTCP, true),
		connections:   make(map[string]net.Conn),
		done:          make(chan struct{}),
	}
	ts.MTU = 1064
	ts.Online = true

	server, client := net.Pipe()

	doneRead := make(chan struct{})
	go func() {
		defer close(doneRead)
		ts.readFramedLoop(server)
	}()

	const bombSize = 4 * 1024 * 1024
	bomb := make([]byte, bombSize)
	for i := range bomb {
		bomb[i] = 0xCC
	}

	memBefore := readAlloc()

	go func() {
		_, _ = client.Write([]byte{HDLCFlag})
		_, _ = client.Write(bomb)
		_, _ = client.Write([]byte{HDLCFlag})
		time.Sleep(50 * time.Millisecond)
		_ = client.Close()
	}()

	select {
	case <-doneRead:
	case <-time.After(10 * time.Second):
		t.Fatal("readFramedLoop did not return after pipe close")
	}

	memAfter := readAlloc()
	maxHDLC := uint64(2*ts.MTU + 32)
	growth := uint64(0)
	if memAfter > memBefore {
		growth = memAfter - memBefore
	}
	memBudget := max(maxHDLC*8, 1<<20)
	if growth > memBudget {
		t.Fatalf("readFramedLoop retained %d bytes after %d-byte bomb (budget=%d, maxHDLC=%d)",
			growth, bombSize, memBudget, maxHDLC)
	}
}

func readAlloc() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}
