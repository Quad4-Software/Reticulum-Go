// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
)

// TestTeardownStorm hammers Teardown from many goroutines while sends are in
// flight. Teardown must be idempotent, must not panic on send-after-close of
// internal channels, and must fire the closed callback exactly once.
func TestTeardownStorm(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	initLink, respLink, mesh := establishChaosLink(t, nil, 1, 15*time.Second)
	defer mesh.close()

	respLink.SetLinkClosedCallback(func(*Link) {})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = initLink.SendPacket([]byte(fmt.Sprintf("storm-%d-%d", i, j)))
			}
		}(i)
	}
	// Race teardowns against the sends.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			initLink.Teardown()
		}()
	}
	wg.Wait()

	if st := initLink.GetStatus(); st != StatusClosed {
		t.Fatalf("initiator status=%d want Closed", st)
	}
}

// TestPanicInPacketCallback verifies a panicking user callback cannot kill the
// link or the transport worker serving it: the next packet must still be
// delivered to the surviving callback chain.
func TestPanicInPacketCallback(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	initLink, respLink, mesh := establishChaosLink(t, nil, 1, 15*time.Second)
	defer mesh.close()

	good := make(chan string, 4)
	calls := 0
	respLink.SetPacketCallback(func(data []byte, pkt *packet.Packet) {
		calls++
		if calls == 1 {
			panic("user callback boom")
		}
		select {
		case good <- string(data):
		default:
		}
	})

	// First packet detonates the callback panic.
	_ = initLink.SendPacket([]byte("detonate"))
	time.Sleep(300 * time.Millisecond)

	// Second packet must still be delivered; the panic must not have taken
	// the worker or the link down.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := initLink.SendPacket([]byte("survivor")); err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		select {
		case got := <-good:
			if got != "survivor" {
				t.Fatalf("got %q want survivor", got)
			}
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
	t.Fatal("callback panic killed packet delivery")
}

// TestLinkStartStopCycleLeak hammers transport/interface start-stop cycles and
// asserts goroutines return to baseline. Deliberate lifecycle abuse.
func TestLinkStartStopCycleLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("lifecycle stress")
	}
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	base := runtime.NumGoroutine()

	for i := 0; i < 12; i++ {
		mesh := newChaosMesh(t)
		init, resp, ok := mesh.establish(t, 10*time.Second)
		if !ok {
			mesh.close()
			t.Fatalf("cycle %d: establish failed", i)
		}
		_ = init
		resp.Teardown()
		init.Teardown()
		mesh.close()
	}

	// maintainLink exits on its 1s tick; allow a tick plus slack.
	runtime.GC()
	time.Sleep(1500 * time.Millisecond)
	got := runtime.NumGoroutine()
	if got > base+10 {
		t.Fatalf("goroutine leak across cycles: baseline=%d after=%d", base, got)
	}
}
