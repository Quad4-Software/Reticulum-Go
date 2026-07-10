// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
)

// fuzzTransport builds a Transport with the announce-table maps only.
// It avoids NewTransport so fuzz workers do not spawn maintenance goroutines.
func fuzzTransport(id *identity.Identity) *Transport {
	return &Transport{
		interfaces:           make(map[string]common.NetworkInterface),
		paths:                make(map[[PathMapKeySize]byte]*common.Path),
		announceTable:        make(map[string]*PathAnnounceEntry),
		heldAnnounces:        make(map[string]*PathAnnounceEntry),
		announcePacketCache:  make(map[string]*packet.Packet),
		pendingLocalPathReqs: make(map[string]common.NetworkInterface),
		transportIdentity:    id,
		done:                 make(chan struct{}),
	}
}

var (
	fuzzIDOnce sync.Once
	fuzzID     *identity.Identity
)

func fuzzIdentity(t *testing.T) *identity.Identity {
	t.Helper()
	fuzzIDOnce.Do(func() {
		id, err := identity.New()
		if err != nil {
			panic(err)
		}
		fuzzID = id
	})
	return fuzzID
}

func FuzzCacheAnnouncePacket(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, []byte("announce-payload"))
	f.Add([]byte{}, []byte{})
	f.Add(bytes.Repeat([]byte{0xAB}, 16), bytes.Repeat([]byte{0xCD}, 200))

	f.Fuzz(func(t *testing.T, destHash []byte, data []byte) {
		tr := fuzzTransport(nil)
		pkt := &packet.Packet{
			DestinationHash: append([]byte(nil), destHash...),
			Data:            append([]byte(nil), data...),
		}
		tr.cacheAnnouncePacket(destHash, pkt)
		got := tr.getCachedAnnouncePacket(destHash)
		if len(destHash) != packet.TruncatedHashLength || len(data) == 0 {
			if got != nil {
				t.Fatalf("invalid inputs must not cache, got %#v", got)
			}
			return
		}
		if got == nil {
			t.Fatal("expected cached packet")
		}
		if !bytes.Equal(got.Data, data) {
			t.Fatalf("cached data mismatch")
		}
		if !bytes.Equal(got.DestinationHash, destHash) {
			t.Fatalf("cached dest mismatch")
		}
	})
}

func FuzzBuildPathResponseWire(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, []byte("payload"), byte(3), true)
	f.Add([]byte{}, []byte{}, byte(0), false)
	f.Add(bytes.Repeat([]byte{0x11}, 16), bytes.Repeat([]byte{0x22}, 128), byte(255), true)
	f.Add([]byte("0"), []byte("0"), byte('\a'), true)

	f.Fuzz(func(t *testing.T, destHash []byte, data []byte, hops byte, block bool) {
		tr := fuzzTransport(fuzzIdentity(t))
		entry := &PathAnnounceEntry{
			AnnounceHops:      hops,
			BlockRebroadcasts: block,
			Packet: &packet.Packet{
				DestinationHash: append([]byte(nil), destHash...),
				Data:            append([]byte(nil), data...),
			},
		}
		raw, err := tr.buildPathResponseWire(entry)
		if len(destHash) != packet.TruncatedHashLength || len(data) == 0 {
			if err == nil {
				t.Fatal("expected error for invalid dest length or empty data")
			}
			return
		}
		if err != nil {
			// HEADER_2 path responses must fit in packet.MTU.
			if strings.Contains(err.Error(), "exceeds MTU") {
				return
			}
			t.Fatalf("buildPathResponseWire: %v", err)
		}
		pkt := &packet.Packet{Raw: raw}
		if err := pkt.Unpack(); err != nil {
			t.Fatalf("unpack: %v", err)
		}
		if pkt.PacketType != packet.PacketTypeAnnounce {
			t.Fatalf("type = %d", pkt.PacketType)
		}
		if pkt.Hops != hops {
			t.Fatalf("hops = %d want %d", pkt.Hops, hops)
		}
		wantCtx := byte(packet.ContextNone)
		if block {
			wantCtx = packet.ContextPathResponse
		}
		if pkt.Context != wantCtx {
			t.Fatalf("context = %#x want %#x", pkt.Context, wantCtx)
		}
	})
}

func FuzzQueueAndProcessAnnounceTable(f *testing.F) {
	f.Add([]byte{9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 1, 2, 3, 4, 5, 6}, []byte("seed-announce"), true, int64(0))
	f.Add(bytes.Repeat([]byte{0x7E}, 16), bytes.Repeat([]byte{0x01}, 80), false, int64(-500))

	f.Fuzz(func(t *testing.T, destHash []byte, data []byte, local bool, graceSkewMs int64) {
		// Keep payloads packable into packet.MTU as HEADER_2 path responses.
		if len(destHash) != 16 || len(data) == 0 || len(data) > 400 {
			return
		}
		tr := fuzzTransport(fuzzIdentity(t))
		iface := newRelayIface("fuzz-iface")
		if local {
			iface.Type = common.IFTypeUnix
			iface.Mode = common.IFModeFull
		}
		tr.interfaces[iface.GetName()] = iface

		tr.cacheAnnouncePacket(destHash, &packet.Packet{
			DestinationHash: append([]byte(nil), destHash...),
			Data:            append([]byte(nil), data...),
		})
		path := &common.Path{
			NextHop:     bytes.Repeat([]byte{0x01}, 16),
			Interface:   iface,
			HopCount:    1,
			LastUpdated: time.Now(),
		}
		ok := tr.queuePathResponseAnnounce(destHash, path, iface, local)
		if !ok {
			t.Fatal("queue with cache must succeed")
		}
		if local {
			if n := countSends(iface); n != 1 {
				t.Fatalf("local emit want 1 send, got %d", n)
			}
			return
		}

		tr.mutex.Lock()
		if entry := tr.announceTable[string(destHash)]; entry != nil {
			entry.RetransmitTimeout = time.Now().Add(time.Duration(graceSkewMs) * time.Millisecond)
		}
		tr.mutex.Unlock()
		tr.processAnnounceTable()
	})
}

func FuzzPendingLocalPathRequestAnswer(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}, []byte("pending"), byte(5))
	f.Add([]byte{}, []byte{}, byte(0))

	f.Fuzz(func(t *testing.T, destHash []byte, data []byte, hops byte) {
		tr := fuzzTransport(fuzzIdentity(t))
		local := newLocalClientRelayIface("fuzz-local")
		tr.interfaces[local.GetName()] = local

		tr.notePendingLocalPathRequest(destHash, local)
		if len(data) > 0 && len(destHash) == packet.TruncatedHashLength {
			tr.cacheAnnouncePacket(destHash, &packet.Packet{
				DestinationHash: append([]byte(nil), destHash...),
				Data:            append([]byte(nil), data...),
			})
		}
		tr.answerPendingLocalPathRequest(destHash, hops)
		if len(destHash) != packet.TruncatedHashLength || len(data) == 0 {
			if n := countSends(local); n != 0 {
				t.Fatalf("invalid pending answer must not send, got %d", n)
			}
			return
		}
		if len(data) > 400 {
			// Oversized cached announces cannot be emitted as path responses.
			if n := countSends(local); n != 0 {
				t.Fatalf("MTU-exceeding pending answer must not send, got %d", n)
			}
			return
		}
		if n := countSends(local); n != 1 {
			t.Fatalf("valid pending answer want 1 send, got %d", n)
		}
	})
}
