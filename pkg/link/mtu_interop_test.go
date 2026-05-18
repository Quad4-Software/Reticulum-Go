// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package link

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

func establishInteropLink(t *testing.T) (*Link, *Link, func()) {
	return establishInteropLinkPipe(t, false)
}

func establishInteropLinkAsync(t *testing.T) (*Link, *Link, func()) {
	return establishInteropLinkPipe(t, true)
}

func establishInteropLinkPipe(t *testing.T, asyncDelivery bool) (*Link, *Link, func()) {
	t.Helper()
	skipHeavyLinkTestsIfShort(t)

	cfgA := &common.ReticulumConfig{}
	trA := transport.NewTransport(cfgA)
	idA, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New A: %v", err)
	}

	cfgB := &common.ReticulumConfig{}
	trB := transport.NewTransport(cfgB)

	var pipeA, pipeB common.NetworkInterface
	var nameA, nameB string
	if asyncDelivery {
		ap := newAsyncPipe("apipeA-mtu", "apipeB-mtu")
		pipeA, pipeB = ap.A, ap.B
		ap.A.tr = trA
		ap.B.tr = trB
		nameA, nameB = ap.A.Name, ap.B.Name
	} else {
		pa := NewPipeInterface("pipeA-mtu")
		pb := NewPipeInterface("pipeB-mtu")
		pa.peer = pb
		pb.peer = pa
		pa.tr = trA
		pb.tr = trB
		pipeA, pipeB = pa, pb
		nameA, nameB = pa.Name, pb.Name
	}

	if err := trA.RegisterInterface(nameA, pipeA); err != nil {
		t.Fatalf("RegisterInterface A: %v", err)
	}
	if err := trB.RegisterInterface(nameB, pipeB); err != nil {
		t.Fatalf("RegisterInterface B: %v", err)
	}

	destA, err := destination.New(idA, destination.In, destination.Single, "mtuapp", trA, "service")
	if err != nil {
		t.Fatalf("destination.New: %v", err)
	}
	destA.AcceptsLinks(true)

	var (
		responderLink *Link
		respMu        sync.Mutex
	)
	estA := make(chan struct{}, 1)
	destA.SetLinkEstablishedCallback(func(l any) {
		if lnk, ok := l.(*Link); ok {
			respMu.Lock()
			responderLink = lnk
			respMu.Unlock()
			select {
			case estA <- struct{}{}:
			default:
			}
		}
	})

	if err := destA.Announce(false, nil, nil); err != nil {
		t.Fatalf("Announce: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	estB := make(chan struct{}, 1)
	initiatorLink := NewLink(destA, trB, pipeB, func(*Link) {
		select {
		case estB <- struct{}{}:
		default:
		}
	}, nil)

	if err := initiatorLink.Establish(); err != nil {
		t.Fatalf("Establish: %v", err)
	}

	select {
	case <-estB:
	case <-time.After(3 * time.Second):
		t.Fatal("initiator link establishment timeout")
	}
	select {
	case <-estA:
	case <-time.After(3 * time.Second):
		t.Fatal("responder link establishment timeout")
	}

	respMu.Lock()
	r := responderLink
	respMu.Unlock()

	if r == nil {
		t.Fatal("nil responder link")
	}

	cleanup := func() {
		trA.Close()
		trB.Close()
	}
	return initiatorLink, r, cleanup
}

func TestLinkInterop_NegotiatedMTUNeverExceedsPacketMTU(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	if initLink.mtu > packet.MTU {
		t.Fatalf("initiator l.mtu=%d exceeds packet.MTU=%d", initLink.mtu, packet.MTU)
	}
	if respLink.mtu > packet.MTU {
		t.Fatalf("responder l.mtu=%d exceeds packet.MTU=%d", respLink.mtu, packet.MTU)
	}
	if initLink.mdu <= 0 || respLink.mdu <= 0 {
		t.Fatalf("non-positive mdu: init=%d resp=%d", initLink.mdu, respLink.mdu)
	}
	if initLink.mdu != respLink.mdu {
		t.Fatalf("mdu mismatch between peers: init=%d resp=%d", initLink.mdu, respLink.mdu)
	}
}

func TestLinkInterop_PacketRoundTripAtBoundarySizes(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	mdu := initLink.mdu
	sizes := []int{1, 16, 64, mdu / 2, mdu - 1, mdu}

	for _, size := range sizes {
		t.Run(boundaryName(size, mdu), func(t *testing.T) {
			payload := bytes.Repeat([]byte{0xAB}, size)

			var got []byte
			done := make(chan struct{}, 1)
			respLink.SetPacketCallback(func(data []byte, _ *packet.Packet) {
				got = append([]byte(nil), data...)
				select {
				case done <- struct{}{}:
				default:
				}
			})

			if err := initLink.SendPacket(payload); err != nil {
				t.Fatalf("SendPacket(%d bytes) failed: %v", size, err)
			}

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatalf("timeout waiting for %d-byte payload", size)
			}

			if !bytes.Equal(got, payload) {
				t.Fatalf("payload mismatch at %d bytes: got %d bytes", size, len(got))
			}
		})
	}
}

func TestLinkInterop_RPiHighMTUScenario(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	initLink.mutex.Lock()
	initLink.mtu = 1196
	initLink.updateMDU()
	initMtu, initMdu := initLink.mtu, initLink.mdu
	initLink.mutex.Unlock()

	respLink.mutex.Lock()
	respLink.mtu = 1196
	respLink.updateMDU()
	respMtu, respMdu := respLink.mtu, respLink.mdu
	respLink.mutex.Unlock()

	if initMtu > packet.MTU {
		t.Fatalf("after RPi-style MTU=1196: initiator mtu=%d still exceeds packet.MTU=%d",
			initMtu, packet.MTU)
	}
	if respMtu > packet.MTU {
		t.Fatalf("after RPi-style MTU=1196: responder mtu=%d still exceeds packet.MTU=%d",
			respMtu, packet.MTU)
	}
	if initMdu != respMdu {
		t.Fatalf("after clamp, mdu mismatch: init=%d resp=%d", initMdu, respMdu)
	}

	payload := bytes.Repeat([]byte{0xCC}, initMdu)
	got := make(chan []byte, 1)
	respLink.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		got <- append([]byte(nil), data...)
	})

	if err := initLink.SendPacket(payload); err != nil {
		t.Fatalf("RPi-scenario SendPacket(mdu=%d) failed: %v", initMdu, err)
	}

	select {
	case received := <-got:
		if !bytes.Equal(received, payload) {
			t.Fatalf("RPi-scenario payload mismatch: got %d bytes, want %d", len(received), len(payload))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RPi-scenario payload not received within 2s")
	}
}

func TestLinkInterop_OversizedPayloadRejectedNotPanicked(t *testing.T) {
	initLink, _, cleanup := establishInteropLink(t)
	defer cleanup()

	huge := bytes.Repeat([]byte{0xEF}, 10000)
	err := initLink.SendPacket(huge)
	if err == nil {
		t.Fatal("expected error sending payload >> packet.MTU, got nil")
	}
}

func TestLinkInterop_MultipleRoundTripsStable(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	const iterations = 50
	mdu := initLink.mdu

	var (
		mu       sync.Mutex
		received [][]byte
	)
	doneCh := make(chan struct{}, iterations)
	respLink.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		mu.Lock()
		received = append(received, append([]byte(nil), data...))
		mu.Unlock()
		doneCh <- struct{}{}
	})

	for i := range iterations {
		payload := bytes.Repeat([]byte{byte(i)}, (mdu/2)+1)
		if err := initLink.SendPacket(payload); err != nil {
			t.Fatalf("iter %d SendPacket: %v", i, err)
		}
	}

	deadline := time.After(5 * time.Second)
	count := 0
	for count < iterations {
		select {
		case <-doneCh:
			count++
		case <-deadline:
			t.Fatalf("timeout: only received %d/%d packets", count, iterations)
		}
	}
}

func boundaryName(size, mdu int) string {
	switch {
	case size == 1:
		return "size_1"
	case size == mdu:
		return "size_mdu"
	case size == mdu-1:
		return "size_mdu_minus_1"
	case size == mdu/2:
		return "size_half_mdu"
	case size == 16:
		return "size_16"
	case size == 64:
		return "size_64"
	default:
		return "size_other"
	}
}
