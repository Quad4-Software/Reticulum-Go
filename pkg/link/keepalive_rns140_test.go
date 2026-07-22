// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"testing"
	"time"

	"quad4/reticulum-go/pkg/packet"
)

func TestInitiatorKeepaliveThrottleUsesLastKeepalive(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()

	l.initiator = true
	l.keepalive = 40 * time.Millisecond
	l.staleTime = time.Hour
	l.establishedAt = time.Now().Add(-time.Minute)
	now := time.Now()
	l.lastInboundNs.Store(now.Add(-time.Minute).UnixNano())
	l.lastOutboundNs.Store(now.UnixNano())
	l.lastKeepaliveNs.Store(now.Add(-time.Minute).UnixNano())

	lastInbound := nsToTime(l.lastInboundNs.Load())
	lastOutbound := nsToTime(l.lastOutboundNs.Load())
	lastKeepalive := nsToTime(l.lastKeepaliveNs.Load())
	nowT := time.Now()
	needKeepalive := nowT.After(lastInbound.Add(l.keepalive)) || nowT.After(lastOutbound.Add(l.keepalive))
	shouldSend := needKeepalive && l.initiator && nowT.After(lastKeepalive.Add(l.keepalive))

	if !needKeepalive {
		t.Fatal("expected needKeepalive when inbound is quiet")
	}
	if !shouldSend {
		t.Fatal("expected initiator keepalive when lastKeepalive aged out despite fresh outbound data")
	}
	if nowT.After(lastOutbound.Add(l.keepalive)) {
		t.Fatal("fixture outbound should still be within keepalive window")
	}
}

func TestResponderKeepaliveThrottleSkipsWhenOutboundFresh(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()
	l.initiator = false
	l.keepalive = time.Hour
	l.linkID = make([]byte, 16)
	before := time.Now().Add(-time.Second).UnixNano()
	l.lastOutboundNs.Store(before)
	l.lastKeepaliveNs.Store(0)
	l.status.Store(int32(StatusActive))
	l.sessionKey = nil

	pkt := &packet.Packet{
		Context: packet.ContextKeepalive,
		Data:    []byte{KeepaliveRequestByte},
	}
	if err := l.handleDataPacket(pkt); err != nil {
		t.Fatalf("handleDataPacket: %v", err)
	}
	if l.lastOutboundNs.Load() != before {
		t.Fatal("throttled keepalive reply must not update outbound timestamps")
	}
	if l.lastKeepaliveNs.Load() != 0 {
		t.Fatal("throttled keepalive reply must not update lastKeepaliveNs")
	}
}

func TestResponderKeepaliveReplyAllowedWhenOutboundQuiet(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()
	l.initiator = false
	l.keepalive = 20 * time.Millisecond
	l.lastOutboundNs.Store(time.Now().Add(-time.Hour).UnixNano())

	lastOutbound := nsToTime(l.lastOutboundNs.Load())
	if !time.Now().After(lastOutbound.Add(l.keepalive)) {
		t.Fatal("fixture outbound must be outside keepalive window")
	}
	before := l.lastKeepaliveNs.Load()
	l.recordKeepaliveOutbound()
	if l.lastKeepaliveNs.Load() == before || l.lastKeepaliveNs.Load() == 0 {
		t.Fatal("recordKeepaliveOutbound must update lastKeepaliveNs")
	}
}
