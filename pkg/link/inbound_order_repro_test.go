// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
)

// TestInboundOrderBurst sends a burst of data packets over a real link and
// requires every payload to reach the responder packet callback in wire
// order. Transport workers dispatch packets concurrently, so per-link order
// must be enforced explicitly.
func TestInboundOrderBurst(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)

	nodeA := newPipeNode(t, "ordA")
	nodeB := newPipeNode(t, "ordB")
	defer nodeA.tr.Close()
	defer nodeB.tr.Close()
	pair(nodeA, nodeB)

	idA, _ := identity.New()
	destA, err := destination.New(idA, destination.In, destination.Single, "testapp", nodeA.tr, "test")
	if err != nil {
		t.Fatalf("destA: %v", err)
	}
	initiator, responder := establishLink(t, nodeA, nodeB, destA)

	const total = 400
	var got atomic.Int64
	var last atomic.Int64
	var order []int64
	outOfOrder := make(chan int, 1)
	responder.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		got.Add(1)
		if len(data) == 8 {
			var idx int64
			for i, b := range data {
				idx |= int64(b) << (8 * i)
			}
			order = append(order, idx)
			if prev := last.Swap(idx); idx < prev {
				select {
				case outOfOrder <- int(prev):
				default:
				}
			}
		}
	})

	for i := int64(0); i < total; i++ {
		var buf [8]byte
		for j := 0; j < 8; j++ {
			buf[j] = byte(i >> (8 * j))
		}
		if err := initiator.SendPacket(buf[:]); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}

	deadline := time.Now().Add(10 * time.Second)
	for got.Load() < total && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	select {
	case <-outOfOrder:
		t.Logf("order: %v", order)
		t.Fatalf("packets delivered out of order")
	default:
	}
	if got.Load() != total {
		t.Fatalf("delivered %d of %d packets", got.Load(), total)
	}
}

// TestIdentifyBeforeRequest sends Identify and an allowlisted request back
// to back. The request handler must see the remote identity, not nil. This
// is the ordering race that made authenticated callers look anonymous when
// the two packets were dispatched on different transport workers.
func TestIdentifyBeforeRequest(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)

	nodeA := newPipeNode(t, "idA")
	nodeB := newPipeNode(t, "idB")
	defer nodeA.tr.Close()
	defer nodeB.tr.Close()
	pair(nodeA, nodeB)

	idA, _ := identity.New()
	idB, _ := identity.New()
	destA, err := destination.New(idA, destination.In, destination.Single, "testapp", nodeA.tr, "test")
	if err != nil {
		t.Fatalf("destA: %v", err)
	}

	type sawIdentity struct {
		has    bool
		at     time.Time
		result string
	}
	saw := make(chan sawIdentity, 4)
	err = destA.RegisterRequestHandler("/who", func(_ string, _ []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) []byte {
		ok := remote != nil
		saw <- sawIdentity{has: ok, at: time.Now()}
		if !ok {
			return nil
		}
		return []byte("hello")
	}, destination.AllowList, [][]byte{idB.Hash()})
	if err != nil {
		t.Fatalf("register handler: %v", err)
	}

	initiator, _ := establishLink(t, nodeA, nodeB, destA)

	// Identify then immediately request: the allowlist requires a proven
	// remote identity, so any ordering slip yields a nil identity and a
	// dropped (nil) response.
	if err := initiator.Identify(idB); err != nil {
		t.Fatalf("identify: %v", err)
	}
	receipt, err := initiator.Request("/who", nil, 5*time.Second)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if receipt == nil {
		t.Fatal("nil receipt")
	}
	select {
	case s := <-saw:
		if !s.has {
			t.Fatal("handler saw nil remote identity after Identify")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("handler never invoked")
	}
}
