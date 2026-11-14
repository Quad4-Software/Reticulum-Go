// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
)

// orderLink records the sequence numbers it receives so tests can verify
// per-link wire order survives the sharded link-data lanes.
type orderLink struct {
	stubLink
	mu   sync.Mutex
	seen []uint64
}

func (l *orderLink) HandleInbound(pkt *packet.Packet) error {
	if len(pkt.Data) >= 8 {
		seq := binary.LittleEndian.Uint64(pkt.Data[:8])
		l.mu.Lock()
		l.seen = append(l.seen, seq)
		l.mu.Unlock()
	}
	return nil
}

func (l *orderLink) snapshot() []uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]uint64(nil), l.seen...)
}

// buildLinkDataHT1 hand-rolls a HeaderType1 link-data packet: flags, hops,
// 16-byte link id, context, payload. Unique payloads keep packet-hash dedup
// from collapsing the stream.
func buildLinkDataHT1(linkID []byte, seq uint64) []byte {
	flags := byte(0)
	flags |= (packet.HeaderType1 << 6) & packet.HeaderMaskHeaderType
	flags |= (packet.PropagationTransport << 4) & packet.HeaderMaskTransportType
	flags |= (packet.DestinationLink << 2) & packet.HeaderMaskDestinationType
	flags |= packet.PacketTypeData & packet.HeaderMaskPacketType

	payload := make([]byte, 8+8)
	binary.LittleEndian.PutUint64(payload[:8], seq)
	// Vary the tail so every packet hashes differently.
	binary.LittleEndian.PutUint64(payload[8:], seq*0x9e3779b97f4a7c15+uint64(len(linkID)))

	out := make([]byte, 0, 2+16+1+len(payload))
	out = append(out, flags, 0)
	out = append(out, linkID...)
	out = append(out, packet.ContextNone)
	out = append(out, payload...)
	return out
}

// TestLinkDataLanesPreservePerLinkOrder hammers the transport with
// interleaved link-data streams for several links and asserts each link
// observes its sequence numbers in send order. The sharded lanes must keep
// same-link packets FIFO while letting different links proceed in parallel.
func TestLinkDataLanesPreservePerLinkOrder(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	const links = 6
	const perLink = 200
	linkIDs := make([][]byte, links)
	recs := make([]*orderLink, links)
	for i := range links {
		id := make([]byte, 16)
		binary.LittleEndian.PutUint64(id, uint64(i+1))
		binary.LittleEndian.PutUint64(id[8:], uint64(i*977+13))
		linkIDs[i] = id
		recs[i] = &orderLink{}
		tr.RegisterLink(id, recs[i])
	}

	iface := newSimIface("lane-test")
	if err := tr.RegisterInterface(iface.GetName(), iface); err != nil {
		t.Fatalf("register iface: %v", err)
	}
	t.Cleanup(iface.stop)

	var wg sync.WaitGroup
	for i := range links {
		wg.Go(func() {
			for seq := uint64(0); seq < perLink; seq++ {
				tr.HandlePacket(buildLinkDataHT1(linkIDs[i], seq), iface)
			}
		})
	}
	wg.Wait()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		done := true
		for _, r := range recs {
			if len(r.snapshot()) < perLink {
				done = false
			}
		}
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	for i, r := range recs {
		got := r.snapshot()
		if len(got) != perLink {
			t.Fatalf("link %d received %d/%d packets", i, len(got), perLink)
		}
		for j, seq := range got {
			if seq != uint64(j) {
				t.Fatalf("link %d out of order at position %d: got seq %d", i, j, seq)
			}
		}
	}
}
