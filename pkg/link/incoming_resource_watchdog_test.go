// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package link

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/resource"
)

// TestIncomingResourceWatchdog_RecoversFromDroppedRequestPacket simulates
// the exact failure this test was written to reproduce: a resource-part
// request packet is lost on the wire, which without a stall watchdog leaves
// the receiver waiting forever for parts the sender was never told to
// (re)send. It asserts the transfer still completes once
// incomingResourceRetryInterval has elapsed, instead of hanging until the
// caller's much longer outer timeout gives up.
func TestIncomingResourceWatchdog_RecoversFromDroppedRequestPacket(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	if err := respLink.SetResourceStrategy(AcceptAll); err != nil {
		t.Fatalf("SetResourceStrategy: %v", err)
	}

	got := make(chan []byte, 1)
	respLink.SetResourceConcludedCallback(func(p any) {
		if b, ok := p.([]byte); ok {
			got <- append([]byte(nil), b...)
		}
	})

	respPipe, ok := respLink.networkInterface.(*PipeInterface)
	if !ok {
		t.Fatalf("expected responder link interface to be *PipeInterface, got %T", respLink.networkInterface)
	}

	var mu sync.Mutex
	dropped := false
	respPipe.dropOnce = func(data []byte) bool {
		mu.Lock()
		defer mu.Unlock()
		if dropped {
			return false
		}
		pkt := &packet.Packet{Raw: append([]byte(nil), data...)}
		if err := pkt.Unpack(); err != nil || pkt.Context != packet.ContextResourceReq {
			return false
		}
		dropped = true
		return true
	}

	payload := bytes.Repeat([]byte{0xB7}, 4000)
	res, err := resource.New(payload, false)
	if err != nil {
		t.Fatalf("resource.New: %v", err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- initLink.SendResource(res) }()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("SendResource error: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("SendResource timeout")
	}

	select {
	case received := <-got:
		if !bytes.Equal(received, payload) {
			t.Fatalf("payload mismatch: got %d bytes, want %d bytes", len(received), len(payload))
		}
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for resource despite watchdog retry; incoming resource stall recovery regressed")
	}

	mu.Lock()
	defer mu.Unlock()
	if !dropped {
		t.Fatal("test bug: never observed a resource-request-next packet to drop")
	}
}

func TestTickIncomingResourceWatchdog_StopsOnceResourceCleared(t *testing.T) {
	l := &Link{}
	rx := &incomingResourceAsm{lastProgressAt: time.Now().Add(-time.Hour)}
	l.incomingRx = nil // superseded/completed before the watchdog ever ticks

	if l.tickIncomingResourceWatchdog(rx) {
		t.Fatal("expected watchdog to stop once incomingRx no longer matches rx")
	}
}

func TestTickIncomingResourceWatchdog_SkipsWhenRecentProgress(t *testing.T) {
	l := &Link{}
	rx := &incomingResourceAsm{lastProgressAt: time.Now(), waitingForHmu: true}
	l.incomingRx = rx

	if !l.tickIncomingResourceWatchdog(rx) {
		t.Fatal("expected watchdog to keep waiting when progress was recent")
	}
	if !rx.waitingForHmu {
		t.Fatal("waitingForHmu should be left untouched when no retry was attempted")
	}
}

func TestTickIncomingResourceWatchdog_ResetsWaitingForHmuBeforeRetry(t *testing.T) {
	l := &Link{}
	rx := &incomingResourceAsm{
		lastProgressAt: time.Now().Add(-time.Hour),
		waitingForHmu:  true,
		totalParts:     1,
		mapHashes:      make([][]byte, 1),
		partSlots:      make([][]byte, 1),
	}
	l.incomingRx = rx

	// The link is not active, so sendIncomingResourceReqNext's eventual
	// SendPacketWithContext call fails fast with "link not active"; what
	// this asserts is that waitingForHmu was reset before that attempt, so
	// a real (active) link would recompute and resend the HMU/part request
	// instead of silently no-op'ing on the stale flag.
	l.tickIncomingResourceWatchdog(rx)
	if rx.waitingForHmu {
		t.Fatal("expected waitingForHmu to be reset before retrying")
	}
}
