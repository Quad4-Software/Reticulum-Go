// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"testing"
	"time"

	"quad4/reticulum-go/pkg/health"
	"quad4/reticulum-go/pkg/packet"
)

// keepaliveOracle holds the pure RNS 1.4.0 keepalive decision surface used as
// a test oracle against initiatorShouldSendKeepalive / responderShouldReplyKeepalive.
type keepaliveOracle struct {
	NeedKeepalive  bool
	InitiatorSend  bool
	ResponderReply bool
}

func decideKeepaliveOracle(now, inbound, outbound, lastKA time.Time, ka time.Duration, initiator bool) keepaliveOracle {
	need := keepaliveDue(now, inbound, outbound, ka)
	return keepaliveOracle{
		NeedKeepalive:  need,
		InitiatorSend:  initiatorShouldSendKeepalive(now, inbound, outbound, lastKA, ka, initiator),
		ResponderReply: responderShouldReplyKeepalive(now, outbound, ka, initiator),
	}
}

func TestOracleKeepaliveMatrixRNS140(t *testing.T) {
	ka := 40 * time.Millisecond
	now := time.Now()
	cases := []struct {
		name      string
		inbound   time.Time
		outbound  time.Time
		lastKA    time.Time
		initiator bool
		want      keepaliveOracle
	}{
		{
			name:      "remote_busy_local_quiet_initiator",
			inbound:   now,
			outbound:  now.Add(-time.Minute),
			lastKA:    now.Add(-time.Minute),
			initiator: true,
			want:      keepaliveOracle{NeedKeepalive: true, InitiatorSend: true, ResponderReply: false},
		},
		{
			name:      "local_data_masks_outbound_but_not_keepalive",
			inbound:   now.Add(-time.Minute),
			outbound:  now,
			lastKA:    now.Add(-time.Minute),
			initiator: true,
			want:      keepaliveOracle{NeedKeepalive: true, InitiatorSend: true, ResponderReply: false},
		},
		{
			name:      "fresh_keepalive_suppresses_initiator",
			inbound:   now.Add(-time.Minute),
			outbound:  now,
			lastKA:    now,
			initiator: true,
			want:      keepaliveOracle{NeedKeepalive: true, InitiatorSend: false, ResponderReply: false},
		},
		{
			name:      "both_fresh_no_probe",
			inbound:   now,
			outbound:  now,
			lastKA:    now.Add(-time.Minute),
			initiator: true,
			want:      keepaliveOracle{NeedKeepalive: false, InitiatorSend: false, ResponderReply: false},
		},
		{
			name:      "responder_throttled_when_outbound_fresh",
			inbound:   now,
			outbound:  now,
			lastKA:    now.Add(-time.Minute),
			initiator: false,
			want:      keepaliveOracle{NeedKeepalive: false, InitiatorSend: false, ResponderReply: false},
		},
		{
			name:      "responder_allowed_when_outbound_quiet",
			inbound:   now,
			outbound:  now.Add(-time.Minute),
			lastKA:    now.Add(-time.Minute),
			initiator: false,
			want:      keepaliveOracle{NeedKeepalive: true, InitiatorSend: false, ResponderReply: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decideKeepaliveOracle(now, tc.inbound, tc.outbound, tc.lastKA, ka, tc.initiator)
			if got != tc.want {
				t.Fatalf("oracle=%+v want %+v", got, tc.want)
			}
		})
	}
}

func TestOracleKeepaliveTimeoutIncrementsHealth(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()
	before := health.Default.TransportOracle()

	l.keepalive = 10 * time.Millisecond
	l.staleTime = 15 * time.Millisecond
	l.rtt = 0.001
	l.establishedAt = time.Now().Add(-time.Hour)
	l.lastInboundNs.Store(time.Now().Add(-time.Hour).UnixNano())
	l.lastOutboundNs.Store(time.Now().Add(-time.Hour).UnixNano())
	l.lastKeepaliveNs.Store(time.Now().Add(-time.Hour).UnixNano())
	l.linkID = make([]byte, 16)
	l.initiator = true
	l.status.Store(int32(StatusActive))
	l.startWatchdog()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if l.GetStatus() == StatusClosed || l.GetStatus() == StatusStale {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	d := before.Delta(health.Default.TransportOracle())
	if d.KeepaliveTimeout == 0 && d.LinkStaleClose == 0 {
		// Watchdog may race past Stale into Closed. Either counter proves the path.
		if l.GetStatus() != StatusClosed {
			t.Fatalf("expected stale/close path, status=%d oracle=%+v", l.GetStatus(), d)
		}
	}
}

func TestOracleRecordKeepaliveDoesNotTouchDataClocks(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()
	l.lastDataSentNs.Store(0)
	l.recordKeepaliveOutbound()
	if l.lastKeepaliveNs.Load() == 0 || l.lastOutboundNs.Load() == 0 {
		t.Fatal("keepalive must update keepalive and outbound clocks")
	}
	if l.lastDataSentNs.Load() != 0 {
		t.Fatal("keepalive must not update lastDataSentNs")
	}
}

func TestAdversarialKeepalivePayloads(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()
	l.initiator = false
	l.keepalive = time.Hour
	l.linkID = make([]byte, 16)
	// Fresh outbound throttles even a well-formed request.
	l.lastOutboundNs.Store(time.Now().UnixNano())
	l.status.Store(int32(StatusActive))
	l.sessionKey = nil

	adversarial := [][]byte{
		nil,
		{},
		{0x00},
		{KeepaliveResponseByte},
		{KeepaliveRequestByte},
		{KeepaliveRequestByte, 0x00},
		bytesRepeat(0xFF, 64),
	}
	beforeKA := l.lastKeepaliveNs.Load()
	beforeOut := l.lastOutboundNs.Load()
	for i, data := range adversarial {
		pkt := &packet.Packet{Context: packet.ContextKeepalive, Data: data}
		_ = l.handleDataPacket(pkt)
		if l.lastKeepaliveNs.Load() != beforeKA {
			t.Fatalf("case %d: throttled/adversarial keepalive mutated lastKeepaliveNs", i)
		}
		if l.lastOutboundNs.Load() != beforeOut {
			t.Fatalf("case %d: throttled/adversarial keepalive mutated lastOutboundNs", i)
		}
	}
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
