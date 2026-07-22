// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"sync"
	"sync/atomic"
	"testing"

	"quad4/reticulum-go/pkg/packet"
)

// closedAbsorbingOracle is true when Closed never transitions to a live status.
type closedAbsorbingOracle struct {
	Revived bool
}

func observeClosedAbsorbing(l *Link, probes int) closedAbsorbingOracle {
	var revived atomic.Bool
	var wg sync.WaitGroup
	pkt := &packet.Packet{
		PacketType:      packet.PacketTypeData,
		Context:         packet.ContextNone,
		DestinationType: DestTypeLink,
		DestinationHash: append([]byte(nil), l.linkID...),
		Data:            []byte{0x00},
	}
	for range probes {
		wg.Go(func() {
			_ = l.HandleInbound(pkt)
			_ = l.promoteToActive()
			if l.GetStatus() != StatusClosed {
				revived.Store(true)
			}
		})
	}
	wg.Wait()
	return closedAbsorbingOracle{Revived: revived.Load()}
}

func TestOracleTOCTOUClosedNotRevivedByLateInbound(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()
	l.linkID = make([]byte, 16)
	_ = setSecBuf(&l.sessionKey, make([]byte, 32))
	_ = setSecBuf(&l.hmacKey, make([]byte, 32))
	l.status.Store(int32(StatusClosed))

	oracle := observeClosedAbsorbing(l, 64)
	if oracle.Revived {
		t.Fatal("Closed link must not be revived by late inbound or promoteToActive")
	}
	if l.GetStatus() != StatusClosed {
		t.Fatalf("status=%d want Closed", l.GetStatus())
	}
}

func TestAdversarialTOCTOUPromoteVsCloseRace(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()
	l.linkID = make([]byte, 16)
	_ = setSecBuf(&l.sessionKey, make([]byte, 32))
	_ = setSecBuf(&l.hmacKey, make([]byte, 32))
	l.status.Store(int32(StatusStale))

	var sawClosed atomic.Bool
	var revivedAfterClose atomic.Bool
	var wg sync.WaitGroup
	for range 48 {
		wg.Go(func() {
			_ = l.promoteToActive()
		})
		wg.Go(func() {
			if l.closeOnce(StatusFailed) {
				sawClosed.Store(true)
			}
			if sawClosed.Load() && l.GetStatus() != StatusClosed {
				revivedAfterClose.Store(true)
			}
		})
		wg.Go(func() {
			pkt := &packet.Packet{
				PacketType:      packet.PacketTypeData,
				Context:         packet.ContextNone,
				DestinationType: DestTypeLink,
				DestinationHash: l.linkID,
				Data:            []byte{0x01},
			}
			_ = l.HandleInbound(pkt)
		})
	}
	wg.Wait()

	_ = l.closeOnce(StatusFailed)
	_ = l.promoteToActive()
	pkt := &packet.Packet{
		PacketType:      packet.PacketTypeData,
		Context:         packet.ContextNone,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
		Data:            []byte{0x02},
	}
	_ = l.HandleInbound(pkt)
	if l.GetStatus() != StatusClosed {
		t.Fatalf("status=%d after forced close+promote, want Closed", l.GetStatus())
	}
	if revivedAfterClose.Load() {
		t.Fatal("status left Closed after closeOnce won")
	}
}

func TestAdversarialTOCTOUStaleInboundVsWatchdogClose(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()
	l.linkID = make([]byte, 16)
	_ = setSecBuf(&l.sessionKey, make([]byte, 32))
	_ = setSecBuf(&l.hmacKey, make([]byte, 32))
	l.status.Store(int32(StatusStale))

	var wg sync.WaitGroup
	for range 24 {
		wg.Go(func() {
			pkt := &packet.Packet{
				PacketType:      packet.PacketTypeData,
				Context:         packet.ContextNone,
				DestinationType: DestTypeLink,
				DestinationHash: l.linkID,
				Data:            []byte{0x03},
			}
			for range 40 {
				_ = l.HandleInbound(pkt)
			}
		})
		wg.Go(func() {
			l.finishWatchdogClose(StatusFailed, false)
		})
	}
	wg.Wait()

	_ = l.closeOnce(StatusFailed)
	_ = l.promoteToActive()
	if l.GetStatus() != StatusClosed {
		t.Fatalf("status=%d after close race, want Closed", l.GetStatus())
	}
}

func TestOracleTOCTOUCloseOnceIdempotent(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()
	l.status.Store(int32(StatusActive))
	var closes atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			if l.closeOnce(StatusFailed) {
				closes.Add(1)
			}
		})
	}
	wg.Wait()
	if closes.Load() != 1 {
		t.Fatalf("closeOnce winners=%d want 1", closes.Load())
	}
}
