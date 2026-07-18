// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

func FuzzLinkHandleData(f *testing.F) {
	cfg := &common.ReticulumConfig{}
	tr := transport.NewTransport(cfg)
	defer tr.Close()
	id, err := identity.New()
	if err != nil {
		f.Fatal(err)
	}
	dest, err := destination.New(id, destination.In, destination.Single, "testapp", tr, "service")
	if err != nil {
		f.Fatal(err)
	}

	l := NewLink(dest, tr, nil, nil, nil)
	l.status.Store(int32(StatusActive))
	l.linkID = make([]byte, 16)
	_ = setSecBuf(&l.sessionKey, make([]byte, 32))
	_ = setSecBuf(&l.hmacKey, make([]byte, 32))

	p := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
		Context:         packet.ContextNone,
		Data:            []byte("hello world"),
	}
	if err := p.Pack(); err == nil {
		f.Add(p.Raw)
	}
	f.Add([]byte{})
	f.Add([]byte{0xfa, 0xfa, 0xfa, 0xfa, 0xfa, 0xfa, 0xfa, 0xfa, 0xfa, 0xfa})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			t.Skip()
		}
		pkt := &packet.Packet{Raw: append([]byte(nil), data...)}
		if err := pkt.Unpack(); err != nil {
			return
		}
		pkt.DestinationType = DestTypeLink
		pkt.DestinationHash = l.linkID

		before := l.GetStatus()
		_ = l.handleDataPacket(pkt)
		after := l.GetStatus()
		assertLinkFuzzStatusLaws(t, l, before, after)
	})
}

// FuzzLinkHandleInbound drives HandleInbound across status and context noise.
func FuzzLinkHandleInbound(f *testing.F) {
	cfg := &common.ReticulumConfig{}
	tr := transport.NewTransport(cfg)
	defer tr.Close()
	id, err := identity.New()
	if err != nil {
		f.Fatal(err)
	}
	dest, err := destination.New(id, destination.In, destination.Single, "fuzzin", tr, "svc")
	if err != nil {
		f.Fatal(err)
	}

	l := NewLink(dest, tr, nil, nil, nil)
	l.linkID = make([]byte, 16)
	_ = setSecBuf(&l.sessionKey, make([]byte, 32))
	_ = setSecBuf(&l.hmacKey, make([]byte, 32))
	l.status.Store(int32(StatusActive))

	seed := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
		Context:         packet.ContextNone,
		Data:            []byte{0x01},
	}
	if err := seed.Pack(); err == nil {
		f.Add(seed.Raw, byte(StatusActive))
		f.Add(seed.Raw, byte(StatusClosed))
		f.Add(seed.Raw, byte(StatusStale))
	}
	f.Add([]byte{0x00, 0x00}, byte(StatusActive))

	f.Fuzz(func(t *testing.T, data []byte, statusByte byte) {
		if len(data) > 1<<14 {
			t.Skip()
		}
		st := statusByte % 5
		l.status.Store(int32(st))
		if st == StatusActive || st == StatusStale {
			if bufLen(l.sessionKey) == 0 {
				_ = setSecBuf(&l.sessionKey, make([]byte, 32))
			}
			if bufLen(l.hmacKey) == 0 {
				_ = setSecBuf(&l.hmacKey, make([]byte, 32))
			}
		}
		pkt := &packet.Packet{Raw: append([]byte(nil), data...)}
		if err := pkt.Unpack(); err != nil {
			pkt = &packet.Packet{
				PacketType:      packet.PacketTypeData,
				Context:         packet.ContextNone,
				DestinationType: DestTypeLink,
				DestinationHash: l.linkID,
				Data:            data,
			}
		} else {
			pkt.DestinationType = DestTypeLink
			pkt.DestinationHash = l.linkID
		}
		before := l.GetStatus()
		_ = l.HandleInbound(pkt)
		after := l.GetStatus()
		assertLinkFuzzStatusLaws(t, l, before, after)
	})
}

func assertLinkFuzzStatusLaws(t *testing.T, l *Link, before, after byte) {
	t.Helper()
	if before == StatusClosed && after != StatusClosed {
		t.Fatalf("Closed link changed status to %d", after)
	}
	if before == StatusActive && (after == StatusPending || after == StatusHandshake) {
		t.Fatalf("Active became establishing status %d", after)
	}
	if after == StatusActive && (bufLen(l.sessionKey) == 0 || bufLen(l.hmacKey) == 0) {
		t.Fatal("Active without session/hmac material")
	}
}
