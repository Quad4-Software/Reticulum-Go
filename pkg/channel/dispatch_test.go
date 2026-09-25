// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package channel

import (
	"context"
	"encoding/binary"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

func inboundFrame(t *testing.T, seq uint16) []byte {
	t.Helper()
	body := []byte{byte(seq), byte(seq >> 8)}
	raw := make([]byte, ChannelHeaderSize+len(body))
	binary.BigEndian.PutUint16(raw[0:2], 1)
	binary.BigEndian.PutUint16(raw[2:4], seq)
	binary.BigEndian.PutUint16(raw[4:6], uint16(len(body)))
	copy(raw[ChannelHeaderSize:], body)
	return raw
}

// Transport workers can race to drain the RX ring. Handler dispatch must stay
// serialized and in sequence order per channel.
func TestHandleInboundDispatchSerializedAndOrdered(t *testing.T) {
	link := &mockLink{status: transport.StatusActive}
	c := NewChannel(link)
	defer func() { _ = c.Close() }()

	var mu sync.Mutex
	var got []uint16
	var inFlight atomic.Int32
	var overlap atomic.Bool
	c.AddMessageHandler(func(m MessageBase) bool {
		if inFlight.Add(1) != 1 {
			overlap.Store(true)
		}
		g := m.(*GenericMessage)
		mu.Lock()
		got = append(got, g.Seq)
		mu.Unlock()
		time.Sleep(200 * time.Microsecond)
		inFlight.Add(-1)
		return true
	})

	// Bursts stay under WindowMax: sequences farther ahead are dropped by the
	// Python-matching far-ahead check, which is out of scope for ordering.
	const waves, perWave = 6, 40
	var wg sync.WaitGroup
	for w := range waves {
		base := uint16(w * perWave)
		start := make(chan struct{})
		for i := range perWave {
			wg.Add(1)
			go func(seq uint16) {
				defer wg.Done()
				<-start
				if err := c.HandleInbound(inboundFrame(t, seq)); err != nil {
					t.Error(err)
				}
			}(base + uint16(i))
		}
		close(start)
		wg.Wait()
		deadline := time.Now().Add(10 * time.Second)
		for {
			mu.Lock()
			n := len(got)
			mu.Unlock()
			if n >= (w+1)*perWave {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("wave %d: only %d of %d messages dispatched", w, n, (w+1)*perWave)
			}
			time.Sleep(time.Millisecond)
		}
	}
	wg.Wait()

	if overlap.Load() {
		t.Fatal("handlers ran concurrently on one channel")
	}
	if len(got) != waves*perWave {
		t.Fatalf("dispatched %d messages, want %d", len(got), waves*perWave)
	}
	for i, seq := range got {
		if seq != uint16(i) {
			t.Fatalf("dispatch out of order at %d: got seq %d", i, seq)
		}
	}
}

func fillWindow(t *testing.T, c *Channel, link *mockLink) {
	t.Helper()
	for c.IsReadyToSend() {
		if err := c.Send(&testMessage{data: []byte("x")}); err != nil {
			t.Fatalf("send: %v", err)
		}
	}
}

func TestWaitReadyWakesOnDelivered(t *testing.T) {
	link := &mockLink{status: transport.StatusActive, rtt: 0.5}
	c := NewChannel(link)
	defer func() { _ = c.Close() }()
	fillWindow(t, c, link)

	done := make(chan error, 1)
	go func() { done <- c.WaitReady(context.Background()) }()

	select {
	case <-done:
		t.Fatal("WaitReady returned while window full")
	case <-time.After(30 * time.Millisecond):
	}

	start := time.Now()
	deliverOne(link)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitReady: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitReady did not wake on delivery")
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("wake took %v, expected prompt delivery wakeup", elapsed)
	}
}

func TestWaitReadyWakesOnTerminalTimeout(t *testing.T) {
	link := &mockLink{status: transport.StatusActive, rtt: 0.5}
	c := NewChannel(link)
	defer func() { _ = c.Close() }()
	c.mutex.Lock()
	c.maxTries = 1
	c.mutex.Unlock()
	fillWindow(t, c, link)

	done := make(chan error, 1)
	go func() { done <- c.WaitReady(context.Background()) }()

	for p, cb := range link.timeouts {
		cb(p)
		break
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitReady: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitReady did not wake on terminal timeout")
	}
}

func TestWaitReadyWakesOnNotifyClosed(t *testing.T) {
	link := &mockLink{status: transport.StatusActive, rtt: 0.5}
	c := NewChannel(link)
	defer func() { _ = c.Close() }()
	fillWindow(t, c, link)

	done := make(chan error, 1)
	go func() { done <- c.WaitReady(context.Background()) }()

	link.setStatus(0)
	c.NotifyClosed()
	select {
	case err := <-done:
		if err != ErrLinkNotReady {
			t.Fatalf("WaitReady = %v, want ErrLinkNotReady", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitReady did not wake on NotifyClosed")
	}
}

func TestWaitReadyMultipleWaiters(t *testing.T) {
	link := &mockLink{status: transport.StatusActive, rtt: 0.5}
	c := NewChannel(link)
	defer func() { _ = c.Close() }()
	fillWindow(t, c, link)

	const waiters = 4
	done := make(chan error, waiters)
	for range waiters {
		go func() { done <- c.WaitReady(context.Background()) }()
	}
	time.Sleep(20 * time.Millisecond)
	deliverOne(link)
	for range waiters {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("WaitReady: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("waiter did not wake")
		}
	}
}

func TestWaitReadyCancel(t *testing.T) {
	link := &mockLink{status: transport.StatusActive, rtt: 0.5}
	c := NewChannel(link)
	defer func() { _ = c.Close() }()
	fillWindow(t, c, link)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.WaitReady(ctx) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("WaitReady = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitReady did not observe cancellation")
	}
}
