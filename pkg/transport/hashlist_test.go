// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"encoding/binary"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/packet"
)

func TestPacketHashlistAddSeenAllocBudget(t *testing.T) {
	hl := newPacketHashList(1_000_000)
	h := make([]byte, 32)
	h[0] = 1
	hl.add(h)
	allocs := testing.AllocsPerRun(1000, func() {
		_ = hl.seen(h)
		hl.add(h)
	})
	if allocs != 0 {
		t.Fatalf("hashlist seen/add allocs=%.1f want 0", allocs)
	}
}

func TestPacketFilterGetHashAllocBudget(t *testing.T) {
	dest := bytesRepeat(packet.TruncatedHashLength, 0x11)
	raw := make([]byte, 0, packet.MinPacketSize+len(dest)+2)
	raw = append(raw, 0x00, 1)
	raw = append(raw, dest...)
	raw = append(raw, packet.ContextNone, 'o', 'k')
	pkt := &packet.Packet{Raw: raw}
	if err := pkt.Unpack(); err != nil {
		t.Fatal(err)
	}
	tr := NewTransport(nil)
	t.Cleanup(func() { _ = tr.Close() })
	allocs := testing.AllocsPerRun(200, func() {
		_ = tr.packetFilter(pkt)
	})
	if allocs != 0 {
		t.Fatalf("packetFilter after Unpack allocs=%.1f want 0", allocs)
	}
}

func TestSeenAnnouncesHardCap(t *testing.T) {
	tr := NewTransport(nil)
	t.Cleanup(func() { _ = tr.Close() })
	tr.mutex.Lock()
	defer tr.mutex.Unlock()
	now := time.Now()
	var h [32]byte
	n := 100_000 + 200
	for i := range n {
		binary.LittleEndian.PutUint32(h[:], uint32(i+1))
		tr.rememberSeenAnnounceUnlocked(h, now)
	}
	if len(tr.seenAnnounces) > 100_000 {
		t.Fatalf("seenAnnounces=%d want <= 100000", len(tr.seenAnnounces))
	}
}

func bytesRepeat(n int, b byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
