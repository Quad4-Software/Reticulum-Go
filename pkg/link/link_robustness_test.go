// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"crypto/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

func TestLinkRobustness_AgeIsNonNegative(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	if initLink.GetAge() < 0 || respLink.GetAge() < 0 {
		t.Fatalf("negative link age: init=%v resp=%v", initLink.GetAge(), respLink.GetAge())
	}
	time.Sleep(50 * time.Millisecond)
	if initLink.GetAge() <= 0 {
		t.Fatalf("link age did not advance: %v", initLink.GetAge())
	}
}

func TestLinkRobustness_StatusActiveAfterEstablish(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	if initLink.GetStatus() != StatusActive {
		t.Fatalf("initiator status %d, want %d", initLink.GetStatus(), StatusActive)
	}
	if respLink.GetStatus() != StatusActive {
		t.Fatalf("responder status %d, want %d", respLink.GetStatus(), StatusActive)
	}
}

func TestLinkRobustness_LinkIDsMatch(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	if !bytes.Equal(initLink.linkID, respLink.linkID) {
		t.Fatalf("link IDs differ: init=%x resp=%x", initLink.linkID, respLink.linkID)
	}
}

func TestLinkRobustness_SymmetricKeysAfterEstablish(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	if !bytes.Equal(initLink.sessionKey, respLink.sessionKey) {
		t.Fatalf("session keys differ between peers")
	}
	if !bytes.Equal(initLink.hmacKey, respLink.hmacKey) {
		t.Fatalf("hmac keys differ between peers")
	}
}

func TestLinkRobustness_TeardownClosesBothEnds(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	closed := make(chan struct{}, 2)
	respLink.SetLinkClosedCallback(func(*Link) {
		select {
		case closed <- struct{}{}:
		default:
		}
	})
	initLink.SetLinkClosedCallback(func(*Link) {
		select {
		case closed <- struct{}{}:
		default:
		}
	})

	initLink.Teardown()

	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("teardown callback not invoked within 2s")
	}

	if initLink.GetStatus() != StatusClosed {
		t.Fatalf("initiator status after teardown=%d, want %d", initLink.GetStatus(), StatusClosed)
	}
}

func TestLinkRobustness_SendPacketOverManyMTUs(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	mtus := []int{200, 350, 500, 1064, 1196, 4096}

	for _, mtu := range mtus {
		t.Run(boundaryName(mtu, mtu), func(t *testing.T) {
			initLink, respLink, cleanup := establishInteropLink(t)
			defer cleanup()

			initLink.mutex.Lock()
			initLink.mtu = mtu
			initLink.updateMDU()
			initMdu := initLink.mdu
			initLink.mutex.Unlock()

			respLink.mutex.Lock()
			respLink.mtu = mtu
			respLink.updateMDU()
			respLink.mutex.Unlock()

			payload := bytes.Repeat([]byte{byte(mtu & 0xFF)}, initMdu)
			got := make(chan []byte, 1)
			respLink.SetPacketCallback(func(data []byte, _ *packet.Packet) {
				got <- append([]byte(nil), data...)
			})

			if err := initLink.SendPacket(payload); err != nil {
				t.Fatalf("SendPacket(%d-byte payload at mtu=%d): %v", initMdu, mtu, err)
			}

			select {
			case received := <-got:
				if !bytes.Equal(received, payload) {
					t.Fatalf("payload mismatch at mtu=%d: got %d bytes, want %d", mtu, len(received), len(payload))
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("timeout receiving payload at mtu=%d", mtu)
			}
		})
	}
}

func TestLinkRobustness_BidirectionalTraffic(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	const iterations = 25
	mdu := initLink.mdu

	var (
		respGotMu sync.Mutex
		respGot   [][]byte
		initGotMu sync.Mutex
		initGot   [][]byte
	)
	respDone := make(chan struct{}, iterations)
	initDone := make(chan struct{}, iterations)

	respLink.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		respGotMu.Lock()
		respGot = append(respGot, append([]byte(nil), data...))
		respGotMu.Unlock()
		respDone <- struct{}{}
	})
	initLink.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		initGotMu.Lock()
		initGot = append(initGot, append([]byte(nil), data...))
		initGotMu.Unlock()
		initDone <- struct{}{}
	})

	for i := range iterations {
		fwd := bytes.Repeat([]byte{byte(i)}, (mdu/3)+1)
		rev := bytes.Repeat([]byte{byte(255 - i)}, (mdu/3)+1)
		if err := initLink.SendPacket(fwd); err != nil {
			t.Fatalf("init SendPacket %d: %v", i, err)
		}
		if err := respLink.SendPacket(rev); err != nil {
			t.Fatalf("resp SendPacket %d: %v", i, err)
		}
	}

	deadline := time.After(5 * time.Second)
	respCount, initCount := 0, 0
	for respCount < iterations || initCount < iterations {
		select {
		case <-respDone:
			respCount++
		case <-initDone:
			initCount++
		case <-deadline:
			t.Fatalf("timeout: respCount=%d initCount=%d (want %d each)", respCount, initCount, iterations)
		}
	}
}

func TestLinkRobustness_ConcurrentSenders(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	const goroutines = 8
	const messagesPerGoroutine = 10

	var counter atomic.Int32
	done := make(chan struct{}, goroutines*messagesPerGoroutine)
	respLink.SetPacketCallback(func(_ []byte, _ *packet.Packet) {
		counter.Add(1)
		done <- struct{}{}
	})

	var wg sync.WaitGroup
	mdu := initLink.mdu
	for g := range goroutines {
		wg.Add(1)
		go func(gID int) {
			defer wg.Done()
			for m := range messagesPerGoroutine {
				payload := bytes.Repeat([]byte{byte(gID*10 + m)}, mdu/4)
				if err := initLink.SendPacket(payload); err != nil {
					t.Errorf("goroutine %d msg %d: %v", gID, m, err)
					return
				}
			}
		}(g)
	}

	wg.Wait()

	deadline := time.After(5 * time.Second)
	for counter.Load() < int32(goroutines*messagesPerGoroutine) {
		select {
		case <-done:
		case <-deadline:
			t.Fatalf("only %d/%d packets received", counter.Load(), goroutines*messagesPerGoroutine)
		}
	}
}

func TestLinkRobustness_SequentialLinkChurn(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	const cycles = 5

	for i := range cycles {
		t.Run(boundaryName(i, cycles), func(t *testing.T) {
			initLink, respLink, cleanup := establishInteropLink(t)
			defer cleanup()

			payload := []byte("churn cycle payload")
			got := make(chan []byte, 1)
			respLink.SetPacketCallback(func(data []byte, _ *packet.Packet) {
				got <- append([]byte(nil), data...)
			})

			if err := initLink.SendPacket(payload); err != nil {
				t.Fatalf("cycle %d send: %v", i, err)
			}
			select {
			case received := <-got:
				if !bytes.Equal(received, payload) {
					t.Fatalf("cycle %d: payload mismatch", i)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("cycle %d: timeout", i)
			}

			initLink.Teardown()
		})
	}
}

func TestLinkRobustness_IdentifyOverMTUClampedLink(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	idB, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}

	identified := make(chan *identity.Identity, 1)
	respLink.SetRemoteIdentifiedCallback(func(_ *Link, id *identity.Identity) {
		identified <- id
	})

	if err := initLink.Identify(idB); err != nil {
		t.Fatalf("Identify: %v", err)
	}

	select {
	case id := <-identified:
		if !bytes.Equal(id.GetPublicKey(), idB.GetPublicKey()) {
			t.Fatalf("identified pubkey mismatch")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("identification did not complete within 3s")
	}
}

func TestLinkRobustness_RequestResponseOverMTUClampedLink(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	cfgA := &common.ReticulumConfig{}
	trA := transport.NewTransport(cfgA)
	defer trA.Close()
	idA, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New A: %v", err)
	}

	cfgB := &common.ReticulumConfig{}
	trB := transport.NewTransport(cfgB)
	defer trB.Close()

	pa := NewPipeInterface("rrpipeA")
	pb := NewPipeInterface("rrpipeB")
	pa.peer, pb.peer = pb, pa
	pa.tr, pb.tr = trA, trB
	if err := trA.RegisterInterface(pa.Name, pa); err != nil {
		t.Fatalf("RegisterInterface A: %v", err)
	}
	if err := trB.RegisterInterface(pb.Name, pb); err != nil {
		t.Fatalf("RegisterInterface B: %v", err)
	}

	destA, err := destination.New(idA, destination.In, destination.Single, "rrapp", trA, "svc")
	if err != nil {
		t.Fatalf("destination.New: %v", err)
	}
	destA.AcceptsLinks(true)
	destA.RegisterRequestHandler("echo", func(_ string, data []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) []byte {
		return append([]byte("echo:"), data...)
	}, destination.AllowAll, nil)

	if err := destA.Announce(false, nil, nil); err != nil {
		t.Fatalf("Announce: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	estB := make(chan struct{}, 1)
	linkB := NewLink(destA, trB, pb, func(*Link) {
		select {
		case estB <- struct{}{}:
		default:
		}
	}, nil)
	if err := linkB.Establish(); err != nil {
		t.Fatalf("Establish: %v", err)
	}

	select {
	case <-estB:
	case <-time.After(3 * time.Second):
		t.Fatal("link establishment timeout")
	}

	if linkB.mtu > packet.MTU {
		t.Fatalf("linkB.mtu=%d after establish exceeds packet.MTU=%d", linkB.mtu, packet.MTU)
	}

	receipt, err := linkB.Request("echo", []byte("hello"), 3*time.Second)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	respCh := make(chan []byte, 1)
	receipt.SetResponseCallback(func(r *RequestReceipt) {
		respCh <- append([]byte(nil), r.GetResponse()...)
	})

	select {
	case got := <-respCh:
		want := []byte("echo:hello")
		if !bytes.Equal(got, want) {
			t.Fatalf("response mismatch: got %q, want %q", got, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request response timeout")
	}
}

func TestLinkRobustness_SendPacketRejectsFarOversize(t *testing.T) {
	initLink, _, cleanup := establishInteropLink(t)
	defer cleanup()

	mdu := initLink.mdu
	overage := []int{mdu * 4, packet.MTU * 2, packet.MTU * 8, 65536}
	for _, sz := range overage {
		payload := bytes.Repeat([]byte{0xAB}, sz)
		if err := initLink.SendPacket(payload); err == nil {
			t.Fatalf("SendPacket(%d) at mdu=%d should fail", sz, mdu)
		}
	}
}

func TestLinkRobustness_LargeRequestResponseResourceCompletes(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	mdu := initLink.mdu
	if mdu <= 0 {
		t.Fatal("mdu must be positive")
	}
	// sendResponse uses a resource when msgpack([requestID, body]) exceeds mdu.
	// A fixed multi-10s payload (e.g. 2048 repeats of a 23-byte string) forces
	// excessive HMU/resource rounds and times out under -race -count=N. Scale to mdu with a cap.

	const maxBody = 8 * 1024
	bodyLen := max(min(mdu*12+256, maxBody), mdu+64)
	largeResponse := bytes.Repeat([]byte("L"), bodyLen)
	respLink.destination.RegisterRequestHandler("echo_large", func(_ string, data []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) []byte {
		_ = data
		return largeResponse
	}, destination.AllowAll, nil)

	receipt, err := initLink.Request("echo_large", []byte("hello"), 30*time.Second)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	respCh := make(chan []byte, 1)
	receipt.SetResponseCallback(func(r *RequestReceipt) {
		respCh <- append([]byte(nil), r.GetResponse()...)
	})

	select {
	case got := <-respCh:
		if !bytes.Equal(got, largeResponse) {
			t.Fatalf("large response mismatch: got=%d want=%d", len(got), len(largeResponse))
		}
	case <-time.After(30 * time.Second):
		t.Fatal("large request response timeout")
	}
}

func TestLinkRobustness_LargeOutboundRequestAsResourceCompletes(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	mdu := initLink.mdu
	if mdu <= 0 {
		t.Fatal("mdu must be positive")
	}
	largeReq := bytes.Repeat([]byte("Q"), mdu+128)
	respLink.destination.RegisterRequestHandler("echo_req", func(_ string, data []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) []byte {
		return append([]byte("ok:"), data...)
	}, destination.AllowAll, nil)

	receipt, err := initLink.Request("echo_req", largeReq, 30*time.Second)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	respCh := make(chan []byte, 1)
	receipt.SetResponseCallback(func(r *RequestReceipt) {
		respCh <- append([]byte(nil), r.GetResponse()...)
	})

	want := append([]byte("ok:"), largeReq...)
	select {
	case got := <-respCh:
		if !bytes.Equal(got, want) {
			t.Fatalf("large outbound request mismatch: got=%d want=%d", len(got), len(want))
		}
	case <-time.After(30 * time.Second):
		t.Fatal("large outbound request timeout")
	}
}

func TestLinkRobustness_NoInboundResetsOnReceive(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	got := make(chan struct{}, 1)
	respLink.SetPacketCallback(func(_ []byte, _ *packet.Packet) {
		select {
		case got <- struct{}{}:
		default:
		}
	})

	if err := initLink.SendPacket([]byte("trigger inbound")); err != nil {
		t.Fatalf("SendPacket: %v", err)
	}
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("trigger packet not received")
	}

	freshAfterRecv := respLink.NoInboundFor()
	if freshAfterRecv > 0.5 {
		t.Fatalf("NoInboundFor too high right after recv: %v", freshAfterRecv)
	}
}

func TestLinkRobustness_RandomPayloadIntegrityAtClampedMDU(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	const iterations = 30
	mdu := initLink.mdu

	resultsMu := sync.Mutex{}
	results := make(map[string]bool)
	doneCh := make(chan struct{}, iterations)

	respLink.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		key := string(data)
		resultsMu.Lock()
		results[key] = true
		resultsMu.Unlock()
		doneCh <- struct{}{}
	})

	sent := make([][]byte, 0, iterations)
	for i := range iterations {
		size := 1 + (i*37)%mdu
		payload := make([]byte, size)
		if _, err := rand.Read(payload); err != nil {
			t.Fatalf("rand.Read: %v", err)
		}
		sent = append(sent, payload)
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
			t.Fatalf("only received %d/%d packets", count, iterations)
		}
	}

	resultsMu.Lock()
	defer resultsMu.Unlock()
	for i, p := range sent {
		if !results[string(p)] {
			t.Fatalf("payload %d not received intact (size=%d)", i, len(p))
		}
	}
}

func TestLinkRobustness_MDUMatchesPeerAfterIndependentClamping(t *testing.T) {
	mtus := []int{500, 700, 1064, 1196, 1500, 8192}
	for _, m := range mtus {
		t.Run(boundaryName(m, m), func(t *testing.T) {
			la := &Link{mtu: m}
			lb := &Link{mtu: m}
			la.updateMDU()
			lb.updateMDU()
			if la.mtu != lb.mtu {
				t.Fatalf("mtu drifted: la=%d lb=%d", la.mtu, lb.mtu)
			}
			if la.mdu != lb.mdu {
				t.Fatalf("mdu drifted: la=%d lb=%d", la.mdu, lb.mdu)
			}
			if la.mtu > packet.MTU {
				t.Fatalf("mtu unclamped: %d > %d", la.mtu, packet.MTU)
			}
		})
	}
}
