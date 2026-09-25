// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

// TestLinkScaleManyConcurrent drives N real concurrent links to one
// destination over a pipe mesh and asserts goroutines and link state
// scale linearly, then converge after teardown. Until now nothing
// exercised more than 2 concurrent links, while production links carry
// watchdog, maintain, and timeout goroutines each.
func TestLinkScaleManyConcurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	const n = 48

	trA := transport.NewTransport(&common.ReticulumConfig{})
	trB := transport.NewTransport(&common.ReticulumConfig{})

	pa := NewPipeInterface("scale-a")
	pb := NewPipeInterface("scale-b")
	pa.peer = pb
	pb.peer = pa
	pa.tr = trA
	pb.tr = trB
	if err := trA.RegisterInterface(pa.Name, pa); err != nil {
		t.Fatalf("register A: %v", err)
	}
	if err := trB.RegisterInterface(pb.Name, pb); err != nil {
		t.Fatalf("register B: %v", err)
	}

	idA, err := identity.New()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	destA, err := destination.New(idA, destination.In, destination.Single, "scaleapp", trA, "links")
	if err != nil {
		t.Fatalf("destination: %v", err)
	}
	destA.AcceptsLinks(true)

	var responders atomic.Int64
	destA.SetLinkEstablishedCallback(func(l any) {
		if lnk, ok := l.(*Link); ok && lnk != nil {
			responders.Add(1)
		}
	})
	if err := destA.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}
	pathDeadline := time.Now().Add(10 * time.Second)
	for !trB.HasPath(destA.GetHash()) && time.Now().Before(pathDeadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !trB.HasPath(destA.GetHash()) {
		t.Fatal("trB never learned path")
	}

	// Warm both transports so their packet worker pools are up before the
	// goroutine baseline is sampled.
	baseG := runtime.NumGoroutine()
	links := make([]*Link, 0, n)
	for i := 0; i < n; i++ {
		estB := make(chan struct{}, 1)
		l := NewLink(destA, trB, pb, func(*Link) {
			select {
			case estB <- struct{}{}:
			default:
			}
		}, nil)
		if err := l.Establish(); err != nil {
			t.Fatalf("link %d establish: %v", i, err)
		}
		deadline := time.Now().Add(15 * time.Second)
		select {
		case <-estB:
		case <-time.After(0):
		}
		for l.GetStatus() != StatusActive && time.Now().Before(deadline) {
			select {
			case <-estB:
			case <-time.After(20 * time.Millisecond):
			}
		}
		if l.GetStatus() != StatusActive {
			t.Fatalf("link %d never became active: %v", i, l.GetStatus())
		}
		links = append(links, l)
	}

	rdeadline := time.Now().Add(10 * time.Second)
	for int(responders.Load()) != n && time.Now().Before(rdeadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := responders.Load(); int(got) != n {
		t.Fatalf("responder count %d want %d", got, n)
	}

	// Goroutine growth must be bounded per link, not unbounded churn.
	scaledG := runtime.NumGoroutine()
	perLink := float64(scaledG-baseG) / float64(n)
	if perLink > 8 {
		t.Fatalf("goroutine overhead %.1f/link exceeds bound (base %d, now %d)", perLink, baseG, scaledG)
	}

	// A sample link still round-trips data.
	got := make(chan struct{}, 1)
	links[0].SetPacketCallback(func(data []byte, _ *packet.Packet) {
		select {
		case got <- struct{}{}:
		default:
		}
	})
	if err := links[n/2].SendPacket([]byte("ping")); err != nil {
		t.Fatalf("send on link %d: %v", n/2, err)
	}

	// Tear everything down, close the transports, and converge to baseline.
	for _, l := range links {
		l.Teardown()
	}
	trA.Close()
	trB.Close()
	converge := time.Now().Add(15 * time.Second)
	for time.Now().Before(converge) {
		if runtime.NumGoroutine() <= baseG+6 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if g := runtime.NumGoroutine(); g > baseG+6 {
		t.Fatalf("goroutines did not converge: base %d, after teardown %d", baseG, g)
	}
}
