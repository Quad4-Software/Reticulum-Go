// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package channel

import (
	"encoding/binary"
	"testing"

	"quad4/reticulum-go/pkg/transport"
)

// FuzzHandleInboundEnvelopeExploratory checks wire length consistency. On success
// the declared body length must fit the buffer and GenericMessage payload
// length must equal that declaration.
func FuzzHandleInboundEnvelopeExploratory(f *testing.F) {
	if raw, err := packEnvelope(1, 0, []byte("abcd")); err == nil {
		f.Add(raw)
	}
	if raw, err := packEnvelope(0xffff, 1, nil); err == nil {
		f.Add(raw)
	}
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x02, 0x41})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			t.Skip()
		}
		link := &mockLink{status: transport.StatusActive}
		ch := NewChannel(link)
		var got MessageBase
		ch.AddMessageHandler(func(msg MessageBase) bool {
			got = msg
			return true
		})
		err := ch.HandleInbound(data)
		if err != nil {
			return
		}
		if len(data) < ChannelHeaderSize {
			t.Fatal("HandleInbound succeeded on short packet")
		}
		length := binary.BigEndian.Uint16(data[4:6])
		if len(data) < ChannelHeaderSize+int(length) {
			t.Fatal("HandleInbound succeeded when declared length exceeds buffer")
		}
		seq := binary.BigEndian.Uint16(data[2:4])
		if seq != 0 {
			if got != nil {
				t.Fatal("nonzero first sequence must not dispatch")
			}
			return
		}
		gm, ok := got.(*GenericMessage)
		if !ok {
			return
		}
		if uint16(len(gm.Data)) != length {
			t.Fatalf("delivered body len=%d want declared length=%d", len(gm.Data), length)
		}
		if gm.GetType() != binary.BigEndian.Uint16(data[0:2]) {
			t.Fatalf("MSGTYPE mismatch got=%d want=%d", gm.GetType(), binary.BigEndian.Uint16(data[0:2]))
		}
	})
}

// FuzzPackHandleInboundRoundTrip confirms packEnvelope bodies survive
// HandleInbound into GenericMessage unchanged.
func FuzzPackHandleInboundRoundTrip(f *testing.F) {
	f.Add(uint16(1), uint16(0), []byte("hello"))
	f.Add(uint16(0xabcd), uint16(9), []byte{})
	f.Add(uint16(7), uint16(3), []byte{0x00, 0xff})

	f.Fuzz(func(t *testing.T, msgType, seq uint16, body []byte) {
		if len(body) > 1<<12 {
			t.Skip()
		}
		raw, err := packEnvelope(msgType, seq, body)
		if err != nil {
			t.Fatalf("packEnvelope: %v", err)
		}
		link := &mockLink{status: transport.StatusActive}
		ch := NewChannel(link)
		var got *GenericMessage
		ch.AddMessageHandler(func(msg MessageBase) bool {
			if g, ok := msg.(*GenericMessage); ok {
				got = g
			}
			return true
		})
		if err := ch.HandleInbound(raw); err != nil {
			t.Fatalf("HandleInbound: %v", err)
		}
		if seq != 0 {
			if got != nil {
				t.Fatal("nonzero first sequence must not dispatch")
			}
			return
		}
		if got == nil {
			t.Fatal("handler did not receive GenericMessage")
		}
		if got.GetType() != msgType {
			t.Fatalf("type got=%d want=%d", got.GetType(), msgType)
		}
		if string(got.Data) != string(body) {
			t.Fatalf("body mismatch got=%q want=%q", got.Data, body)
		}
	})
}
