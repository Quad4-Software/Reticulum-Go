// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
)

func (l *Link) updateKeepaliveLocked() {
	if l.rtt <= 0 {
		return
	}

	keepaliveMax := float64(Keepalive)

	calculatedKeepalive := l.rtt * (keepaliveMax / KeepaliveMaxRTT)
	if calculatedKeepalive > keepaliveMax {
		calculatedKeepalive = keepaliveMax
	}
	if calculatedKeepalive < KeepaliveMinSec {
		calculatedKeepalive = KeepaliveMinSec
	}

	l.keepalive = time.Duration(calculatedKeepalive * float64(time.Second))
	l.staleTime = time.Duration(float64(l.keepalive) * float64(2))
}
func (l *Link) sendKeepalive() error {
	keepaliveData := []byte{0xFF}
	keepalivePkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		TransportType:   0,
		Context:         packet.ContextKeepalive,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
		Data:            keepaliveData,
		CreateReceipt:   false,
	}
	// Python Packet.pack sends KEEPALIVE payloads unencrypted; encrypting
	// here would make the 0xFF byte unrecognizable to the peer and the
	// reply would never arrive, leaving the link to go stale.
	if err := keepalivePkt.Pack(); err != nil {
		return err
	}
	l.recordKeepaliveOutbound()
	return l.transport.SendPacket(keepalivePkt)
}

// keepaliveDue reports whether inbound or outbound activity is older than the
// keepalive interval (RNS 1.4.0 needKeepalive predicate).
func keepaliveDue(now, inboundActivity, lastOutbound time.Time, keepalive time.Duration) bool {
	return now.After(inboundActivity.Add(keepalive)) || now.After(lastOutbound.Add(keepalive))
}

// initiatorShouldSendKeepalive is the RNS 1.4.0 initiator probe gate.
// Probes fire when keepalive is due and lastKeepalive is older than the interval.
func initiatorShouldSendKeepalive(now, inboundActivity, lastOutbound, lastKeepalive time.Time, keepalive time.Duration, initiator bool) bool {
	if !initiator {
		return false
	}
	if !keepaliveDue(now, inboundActivity, lastOutbound, keepalive) {
		return false
	}
	return now.After(lastKeepalive.Add(keepalive))
}

// responderShouldReplyKeepalive is the RNS 1.4.0 responder reply throttle.
func responderShouldReplyKeepalive(now, lastOutbound time.Time, keepalive time.Duration, initiator bool) bool {
	if initiator {
		return false
	}
	return !now.Before(lastOutbound.Add(keepalive))
}
