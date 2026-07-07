// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/packet"
)

// TestRegression_ParallelSimFastPathHookAccess guards against data races on
// simulation fast-path globals when parallel tests enable/disable overrides.
func TestRegression_ParallelSimFastPathHookAccess(t *testing.T) {
	const workers = 32
	var wg sync.WaitGroup
	deadline := time.Now().Add(2 * time.Second)
	for i := range workers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for time.Now().Before(deadline) {
				if id%2 == 0 {
					simHooksMu.Lock()
					simPathfinderRW = &simFastPathfinderRW
					simAnnounceRateKbps = &simFastAnnounceBypass
					simHooksMu.Unlock()
					_ = effectivePathfinderRW()
					_ = simFastPathActive()
					disableSimFastPath()
				} else {
					disableSimFastPath()
					_ = effectivePathfinderRW()
				}
			}
		}(i)
	}
	wg.Wait()
}

// TestRegression_UnregisterInterfaceClearsPathsAndLinks guards against stale
// path/link relay rows referencing a removed interface (blackhole routing,
// relay loops after hot reload).
func TestRegression_UnregisterInterfaceClearsPathsAndLinks(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	iface := mockIface("wan", true)
	if err := tr.RegisterInterface("wan", iface); err != nil {
		t.Fatal(err)
	}

	dest := bytes.Repeat([]byte{0xAA}, 16)
	nextHop := bytes.Repeat([]byte{0xBB}, 16)
	linkID := bytes.Repeat([]byte{0xCC}, 16)
	tr.UpdatePath(dest, nextHop, "wan", 1)
	tr.RegisterLink(linkID, &linkBoundIface{id: linkID, ni: iface})

	if !tr.HasPath(dest) {
		t.Fatal("fixture path missing")
	}

	tr.UnregisterInterface("wan")

	if tr.HasPath(dest) {
		t.Fatal("path table should drop entries for removed interface")
	}
	tr.mutex.RLock()
	_, linkExists := tr.links[hash16FromSlice(linkID)]
	tr.mutex.RUnlock()
	if linkExists {
		t.Fatal("link table should drop entries bound to removed interface")
	}
}

// TestRegression_ReplaceInterfaceScrubsOldPaths guards the same invariant when
// an interface is hot-swapped instead of unregistered outright.
func TestRegression_ReplaceInterfaceScrubsOldPaths(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	old := mockIface("wan", true)
	if err := tr.RegisterInterface("wan", old); err != nil {
		t.Fatal(err)
	}
	dest := bytes.Repeat([]byte{0xDD}, 16)
	tr.UpdatePath(dest, bytes.Repeat([]byte{0xEE}, 16), "wan", 1)

	replacement := mockIface("wan", true)
	if err := tr.ReplaceInterface("wan", replacement); err != nil {
		t.Fatal(err)
	}
	if tr.HasPath(dest) {
		t.Fatal("path table should be scrubbed when interface is replaced")
	}
}

// TestRegression_ConcurrentPathReadersDuringUnregister guards transport
// readers (HasPath, NextHop) against concurrent interface teardown.
func TestRegression_ConcurrentPathReadersDuringUnregister(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	iface := mockIface("eth", true)
	if err := tr.RegisterInterface("eth", iface); err != nil {
		t.Fatal(err)
	}
	dest := bytes.Repeat([]byte{0x11}, 16)
	tr.UpdatePath(dest, bytes.Repeat([]byte{0x22}, 16), "eth", 1)

	var wg sync.WaitGroup
	const readers = 12
	wg.Add(readers + 1)
	for range readers {
		go func() {
			defer wg.Done()
			for range 200 {
				_ = tr.HasPath(dest)
				_ = tr.NextHop(dest)
				_ = tr.NextHopInterface(dest)
			}
		}()
	}
	go func() {
		defer wg.Done()
		for range 40 {
			tr.UnregisterInterface("eth")
			_ = tr.RegisterInterface("eth", mockIface("eth", true))
			tr.UpdatePath(dest, bytes.Repeat([]byte{0x22}, 16), "eth", 1)
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent readers during unregister timed out")
	}
}

// TestRegression_HandleProofPacketDoesNotPanicOnUnknownLink guards the proof
// dispatch path when a proof arrives for a link ID that is not registered.
func TestRegression_HandleProofPacketDoesNotPanicOnUnknownLink(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	iface := mockIface("wan", true)
	if err := tr.RegisterInterface("wan", iface); err != nil {
		t.Fatal(err)
	}

	pkt := &packet.Packet{
		PacketType:      packet.PacketTypeProof,
		Context:         packet.ContextLRProof,
		DestinationType: 0x03,
		DestinationHash: bytes.Repeat([]byte{0x99}, 16),
		Data:            bytes.Repeat([]byte{0xAB}, 32),
	}
	if err := pkt.Pack(); err != nil {
		t.Fatal(err)
	}

	tr.handleProofPacket(pkt, iface)
}
