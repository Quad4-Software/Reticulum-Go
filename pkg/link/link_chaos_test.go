// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"math/rand/v2"
	"runtime"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/resource"
	"quad4/reticulum-go/pkg/transport"
)

type chaosMesh struct {
	trA, trB     *transport.Transport
	pipeA, pipeB *PipeInterface
	destA        *destination.Destination
}

func newChaosMesh(t *testing.T) *chaosMesh {
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

	pipeA := NewPipeInterface("chaos-pipeA")
	pipeB := NewPipeInterface("chaos-pipeB")
	pipeA.peer = pipeB
	pipeB.peer = pipeA
	pipeA.tr = trA
	pipeB.tr = trB

	if err := trA.RegisterInterface(pipeA.Name, pipeA); err != nil {
		t.Fatalf("RegisterInterface A: %v", err)
	}
	if err := trB.RegisterInterface(pipeB.Name, pipeB); err != nil {
		t.Fatalf("RegisterInterface B: %v", err)
	}

	destA, err := destination.New(idA, destination.In, destination.Single, "chaosapp", trA, "service")
	if err != nil {
		t.Fatalf("destination.New: %v", err)
	}
	destA.AcceptsLinks(true)

	return &chaosMesh{trA: trA, trB: trB, pipeA: pipeA, pipeB: pipeB, destA: destA}
}

func (m *chaosMesh) close() {
	m.pipeA.clearChaos()
	m.pipeB.clearChaos()
	_ = m.trA.Close()
	_ = m.trB.Close()
}

// establish attempts a single link handshake and returns initiator and responder.
func (m *chaosMesh) establish(t *testing.T, timeout time.Duration) (initiator *Link, responder *Link, ok bool) {
	t.Helper()

	var (
		responderLink *Link
		respMu        sync.Mutex
	)
	estA := make(chan struct{}, 1)
	m.destA.SetLinkEstablishedCallback(func(l any) {
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

	if err := m.destA.Announce(false, nil, nil); err != nil {
		t.Fatalf("Announce: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	estB := make(chan struct{}, 1)
	initiatorLink := NewLink(m.destA, m.trB, m.pipeB, func(*Link) {
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
	case <-time.After(timeout):
		initiatorLink.Teardown()
		respMu.Lock()
		if responderLink != nil {
			responderLink.Teardown()
		}
		respMu.Unlock()
		return nil, nil, false
	}
	select {
	case <-estA:
	case <-time.After(timeout):
		initiatorLink.Teardown()
		respMu.Lock()
		if responderLink != nil {
			responderLink.Teardown()
		}
		respMu.Unlock()
		return nil, nil, false
	}

	respMu.Lock()
	r := responderLink
	respMu.Unlock()
	if r == nil {
		initiatorLink.Teardown()
		return nil, nil, false
	}
	return initiatorLink, r, true
}

func establishChaosLink(t *testing.T, beforeEstablish func(a, b *PipeInterface), attempts int, perAttempt time.Duration) (initiator *Link, responder *Link, mesh *chaosMesh) {
	t.Helper()
	if attempts < 1 {
		attempts = 1
	}
	if perAttempt <= 0 {
		perAttempt = 8 * time.Second
	}
	for range attempts {
		mesh = newChaosMesh(t)
		if beforeEstablish != nil {
			beforeEstablish(mesh.pipeA, mesh.pipeB)
		}
		init, resp, ok := mesh.establish(t, perAttempt)
		if ok {
			return init, resp, mesh
		}
		mesh.close()
	}
	t.Fatalf("link establishment failed after %d attempts under chaos", attempts)
	return nil, nil, nil
}

// TestLinkChaosEstablishUnderLoss establishes a link while both pipes drop
// ~15% of outbound frames. Handshake may need a few attempts when critical
// frames are dropped.
func TestLinkChaosEstablishUnderLoss(t *testing.T) {
	initLink, respLink, mesh := establishChaosLink(t, func(a, b *PipeInterface) {
		a.setLossyDrop(0.15, 0x11c0101)
		b.setLossyDrop(0.15, 0x11c0102)
	}, 8, 5*time.Second)
	defer mesh.close()

	if initLink.GetStatus() != StatusActive {
		t.Fatalf("initiator status %d, want Active", initLink.GetStatus())
	}
	if respLink.GetStatus() != StatusActive {
		t.Fatalf("responder status %d, want Active", respLink.GetStatus())
	}
}

// TestLinkChaosPacketUnderLoss round-trips a packet after establishing, then
// applying bidirectional loss on the live session.
func TestLinkChaosPacketUnderLoss(t *testing.T) {
	initLink, respLink, mesh := establishChaosLink(t, nil, 1, 15*time.Second)
	defer mesh.close()

	mesh.pipeA.setLossyDrop(0.20, 0x11c0203)
	mesh.pipeB.setLossyDrop(0.20, 0x11c0204)

	payload := bytes.Repeat([]byte{0xC4}, 64)
	got := make(chan []byte, 8)
	respLink.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		select {
		case got <- append([]byte(nil), data...):
		default:
		}
	})

	deadline := time.Now().Add(20 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := initLink.SendPacket(payload); err != nil {
			lastErr = err
			time.Sleep(20 * time.Millisecond)
			continue
		}
		select {
		case received := <-got:
			if !bytes.Equal(received, payload) {
				t.Fatalf("payload mismatch: got %d want %d", len(received), len(payload))
			}
			respLink.SetPacketCallback(nil)
			return
		case <-time.After(200 * time.Millisecond):
		}
	}
	respLink.SetPacketCallback(nil)
	t.Fatalf("packet round-trip under loss timed out (last send err: %v)", lastErr)
}

// TestLinkChaosPacketUnderReorder round-trips a packet while pipes reorder
// outbound frames in short batches.
func TestLinkChaosPacketUnderReorder(t *testing.T) {
	initLink, respLink, mesh := establishChaosLink(t, nil, 1, 15*time.Second)
	defer mesh.close()

	mesh.pipeA.setReorderDrop(4, 0x11c0211)
	mesh.pipeB.setReorderDrop(4, 0x11c0212)

	payload := bytes.Repeat([]byte{0xC5}, 48)
	got := make(chan []byte, 16)
	respLink.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		select {
		case got <- append([]byte(nil), data...):
		default:
		}
	})

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		_ = initLink.SendPacket(payload)
		_ = initLink.SendPacket(bytes.Repeat([]byte{0x11}, 16))
		select {
		case received := <-got:
			if bytes.Equal(received, payload) {
				respLink.SetPacketCallback(nil)
				mesh.pipeA.clearChaos()
				mesh.pipeB.clearChaos()
				return
			}
		case <-time.After(100 * time.Millisecond):
		}
	}
	respLink.SetPacketCallback(nil)
	mesh.pipeA.clearChaos()
	mesh.pipeB.clearChaos()
	t.Fatal("packet round-trip under reorder timed out")
}

// TestLinkChaosResourceUnderDrop transfers a multi-part resource while pipes
// drop a bounded number of frames. Completion relies on link resource retries
// and the incoming resource stall watchdog.
func TestLinkChaosResourceUnderDrop(t *testing.T) {
	initLink, respLink, mesh := establishChaosLink(t, nil, 1, 15*time.Second)
	defer mesh.close()

	// Cap total drops so retries can finish. Unbounded probabilistic loss can
	// starve the resource handshake indefinitely.
	installCappedLoss := func(p *PipeInterface, seed uint64, maxDrops uint64) {
		rng := rand.New(rand.NewPCG(seed, seed^0x5eed))
		var mu sync.Mutex
		var dropped uint64
		p.setDropOnce(func(_ []byte) bool {
			mu.Lock()
			defer mu.Unlock()
			if dropped >= maxDrops {
				return false
			}
			if rng.Float64() < 0.25 {
				dropped++
				return true
			}
			return false
		})
	}
	installCappedLoss(mesh.pipeA, 0x11c0301, 3)
	installCappedLoss(mesh.pipeB, 0x11c0302, 3)

	if err := respLink.SetResourceStrategy(AcceptAll); err != nil {
		t.Fatalf("SetResourceStrategy: %v", err)
	}

	got := make(chan []byte, 1)
	respLink.SetResourceConcludedCallback(func(p any) {
		if b, ok := p.([]byte); ok {
			got <- append([]byte(nil), b...)
		}
	})

	payload := bytes.Repeat([]byte{0xD7}, 2500)
	res, err := resource.New(payload, false)
	if err != nil {
		t.Fatalf("resource.New: %v", err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- initLink.SendResource(res) }()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("SendResource: %v", err)
		}
	case <-time.After(45 * time.Second):
		t.Fatal("SendResource timeout under loss")
	}

	select {
	case received := <-got:
		if !bytes.Equal(received, payload) {
			t.Fatalf("resource payload mismatch: got %d want %d", len(received), len(payload))
		}
	case <-time.After(45 * time.Second):
		t.Fatal("resource conclusion timeout under loss")
	}
}

// TestLinkChaosMidSessionFlap takes the pipes offline briefly after the
// session is active, restores them, proves a packet still round-trips, then
// tears down cleanly.
func TestLinkChaosMidSessionFlap(t *testing.T) {
	initLink, respLink, mesh := establishChaosLink(t, nil, 1, 15*time.Second)
	defer mesh.close()

	mesh.pipeA.setOnline(false)
	mesh.pipeB.setOnline(false)
	time.Sleep(80 * time.Millisecond)
	mesh.pipeA.setOnline(true)
	mesh.pipeB.setOnline(true)

	payload := bytes.Repeat([]byte{0x4D}, 32)
	got := make(chan []byte, 4)
	respLink.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		select {
		case got <- append([]byte(nil), data...):
		default:
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_ = initLink.SendPacket(payload)
		select {
		case received := <-got:
			if bytes.Equal(received, payload) {
				goto restored
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
	t.Fatal("packet round-trip failed after mid-session flap restore")
restored:
	respLink.SetPacketCallback(nil)

	closed := make(chan struct{}, 2)
	initLink.SetLinkClosedCallback(func(*Link) {
		select {
		case closed <- struct{}{}:
		default:
		}
	})
	respLink.SetLinkClosedCallback(func(*Link) {
		select {
		case closed <- struct{}{}:
		default:
		}
	})

	done := make(chan struct{})
	go func() {
		initLink.Teardown()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Teardown deadlocked after mid-session flap")
	}

	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("closed callback not invoked after teardown")
	}

	if initLink.GetStatus() != StatusClosed {
		t.Fatalf("initiator status after teardown=%d, want Closed", initLink.GetStatus())
	}
}

// TestLinkChaosGoroutineBudget runs establish, send under light loss, and
// teardown, then asserts the goroutine count returns near baseline.
func TestLinkChaosGoroutineBudget(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	runtime.GC()
	time.Sleep(30 * time.Millisecond)
	baseG := runtime.NumGoroutine()

	initLink, respLink, mesh := establishChaosLink(t, nil, 1, 15*time.Second)

	mesh.pipeA.setLossyDrop(0.10, 0x11c0501)
	mesh.pipeB.setLossyDrop(0.10, 0x11c0502)

	payload := []byte("chaos-budget")
	got := make(chan struct{}, 1)
	respLink.SetPacketCallback(func(_ []byte, _ *packet.Packet) {
		select {
		case got <- struct{}{}:
		default:
		}
	})
	for range 8 {
		_ = initLink.SendPacket(payload)
		select {
		case <-got:
			goto sent
		case <-time.After(100 * time.Millisecond):
		}
	}
sent:
	initLink.Teardown()
	mesh.close()

	runtime.GC()
	time.Sleep(400 * time.Millisecond)
	finalG := runtime.NumGoroutine()
	if finalG > baseG+12 {
		t.Fatalf("goroutine leak after link chaos: baseline=%d final=%d", baseG, finalG)
	}
}
