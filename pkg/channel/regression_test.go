// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package channel

import (
	"encoding/binary"
	"testing"
)

// TestRegression_SendEmitsPythonEnvelopeHeader pins the wire format Python RNS
// Channel.Envelope.pack uses: big-endian MSGTYPE, sequence, length, then body.
// Sending only msg.Pack() breaks Python interop and Go HandleInbound.
func TestRegression_SendEmitsPythonEnvelopeHeader(t *testing.T) {
	link := &mockLink{status: 1}
	c := NewChannel(link)
	defer func() { _ = c.Close() }()

	body := []byte("wire-guard")
	if err := c.Send(&testMessage{data: body}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(link.sent) != 1 {
		t.Fatalf("sent=%d want 1", len(link.sent))
	}
	raw := link.sent[0]
	if len(raw) < ChannelHeaderSize {
		t.Fatalf("raw too short: %d", len(raw))
	}
	if ChannelHeaderSize != 6 {
		t.Fatalf("ChannelHeaderSize=%d want 6 (Python >HHH)", ChannelHeaderSize)
	}
	msgType := binary.BigEndian.Uint16(raw[0:2])
	seq := binary.BigEndian.Uint16(raw[2:4])
	length := binary.BigEndian.Uint16(raw[4:6])
	if msgType != 1 {
		t.Fatalf("MSGTYPE=%d want 1", msgType)
	}
	if seq != 0 {
		t.Fatalf("sequence=%d want 0 (reserved on first send)", seq)
	}
	if int(length) != len(body) {
		t.Fatalf("length=%d want %d", length, len(body))
	}
	if string(raw[ChannelHeaderSize:]) != string(body) {
		t.Fatalf("body=%q want %q", raw[ChannelHeaderSize:], body)
	}
	// Bare packed body must never be what goes on the wire.
	if len(raw) == len(body) {
		t.Fatal("Send transmitted bare Pack() body without envelope header")
	}
}

// TestRegression_HandleInboundUsesRegisteredFactory ensures typed dispatch is
// not silently reduced to GenericMessage (buffer readers type-assert).
func TestRegression_HandleInboundUsesRegisteredFactory(t *testing.T) {
	link := &mockLink{status: 1}
	c := NewChannel(link)
	defer func() { _ = c.Close() }()

	if err := c.RegisterMessageType(1, func() MessageBase { return &testMessage{} }); err != nil {
		t.Fatal(err)
	}

	var got *testMessage
	c.AddMessageHandler(func(m MessageBase) bool {
		tm, ok := m.(*testMessage)
		if !ok {
			t.Errorf("got %T want *testMessage", m)
			return true
		}
		got = tm
		return true
	})

	raw, err := packEnvelope(1, 3, []byte("typed"))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.HandleInbound(raw); err != nil {
		t.Fatal(err)
	}
	if got == nil || string(got.data) != "typed" {
		t.Fatalf("got=%v", got)
	}
}

// TestRegression_SystemMessageTypeReservation guards StreamDataMessage (0xFF00)
// registration: user RegisterMessageType must reject system range.
func TestRegression_SystemMessageTypeReservation(t *testing.T) {
	link := &mockLink{status: 1}
	c := NewChannel(link)
	defer func() { _ = c.Close() }()

	if SystemMessageTypeMin != 0xf000 {
		t.Fatalf("SystemMessageTypeMin=0x%04x want 0xf000", SystemMessageTypeMin)
	}
	err := c.RegisterMessageType(0xff00, func() MessageBase { return &GenericMessage{} })
	if err == nil {
		t.Fatal("RegisterMessageType(0xff00) should fail (system-reserved)")
	}
	if err := c.RegisterSystemMessageType(0xff00, func() MessageBase { return &GenericMessage{} }); err != nil {
		t.Fatalf("RegisterSystemMessageType(0xff00): %v", err)
	}
}
