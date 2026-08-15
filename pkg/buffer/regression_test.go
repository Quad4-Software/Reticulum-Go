// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package buffer

import (
	"encoding/binary"
	"io"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/channel"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

type captureLink struct {
	status byte
	rtt    float64
	sent   [][]byte
}

func (m *captureLink) GetStatus() byte   { return m.status }
func (m *captureLink) GetRTT() float64   { return m.rtt }
func (m *captureLink) RTT() float64      { return m.rtt }
func (m *captureLink) GetLinkID() []byte { return []byte("testlink") }
func (m *captureLink) Send(data []byte) any {
	m.sent = append(m.sent, append([]byte(nil), data...))
	return &packet.Packet{Raw: data}
}
func (m *captureLink) Resend(p any) error                                    { return nil }
func (m *captureLink) SetPacketTimeout(p any, cb func(any), t time.Duration) {}
func (m *captureLink) SetPacketDelivered(p any, cb func(any)) {
	if cb != nil {
		cb(p)
	}
}
func (m *captureLink) HandleInbound(pkt *packet.Packet) error { return nil }
func (m *captureLink) ValidateLinkProof(pkt *packet.Packet, networkIface common.NetworkInterface) error {
	return nil
}
func (m *captureLink) LinkedNetworkInterface() common.NetworkInterface { return nil }

// TestRegression_StreamDataMessageTypeMatchesPython pins MSGTYPE 0xFF00.
// The previous 0x01 value was invisible to crossref packed-body vectors and
// broke Python Buffer / Channel interop.
func TestRegression_StreamDataMessageTypeMatchesPython(t *testing.T) {
	const pythonStreamDataMSGTYPE uint16 = 0xff00
	if StreamDataMessageType != pythonStreamDataMSGTYPE {
		t.Fatalf("StreamDataMessageType=0x%04x want 0x%04x (Python StreamDataMessage.MSGTYPE)",
			StreamDataMessageType, pythonStreamDataMSGTYPE)
	}
	if StreamDataMessageType == 0x01 {
		t.Fatal("StreamDataMessageType must not be 0x01 (pre-interop bug)")
	}
	msg := &StreamDataMessage{StreamID: 1, Data: []byte("x")}
	if msg.GetType() != pythonStreamDataMSGTYPE {
		t.Fatalf("GetType()=0x%04x want 0x%04x", msg.GetType(), pythonStreamDataMSGTYPE)
	}
	if StreamDataMessageType < channel.SystemMessageTypeMin {
		t.Fatalf("StreamDataMessageType=0x%04x must be system-reserved (>=0x%04x)",
			StreamDataMessageType, channel.SystemMessageTypeMin)
	}
}

// TestRegression_ChannelSendUsesStreamMSGTYPEInEnvelope ensures writers put
// 0xFF00 in the channel envelope MSGTYPE field, not only in GetType().
func TestRegression_ChannelSendUsesStreamMSGTYPEInEnvelope(t *testing.T) {
	link := &captureLink{status: transport.StatusActive}
	ch := channel.NewChannel(link)
	defer func() { _ = ch.Close() }()

	w := NewRawChannelWriter(1, ch)
	payload := []byte("stream-wire")
	n, err := w.Write(payload)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("n=%d want %d", n, len(payload))
	}
	if len(link.sent) != 1 {
		t.Fatalf("sent=%d want 1", len(link.sent))
	}
	raw := link.sent[0]
	if len(raw) < channel.ChannelHeaderSize {
		t.Fatalf("envelope too short: %d", len(raw))
	}
	msgType := binary.BigEndian.Uint16(raw[0:2])
	if msgType != StreamDataMessageType {
		t.Fatalf("envelope MSGTYPE=0x%04x want 0x%04x", msgType, StreamDataMessageType)
	}
}

// TestRegression_ReaderReceivesTypedStreamDataMessage guards the factory path:
// HandleInbound must deliver *StreamDataMessage or HandleMessage ignores it.
func TestRegression_ReaderReceivesTypedStreamDataMessage(t *testing.T) {
	link := &captureLink{status: transport.StatusActive}
	ch := channel.NewChannel(link)
	defer func() { _ = ch.Close() }()

	reader := NewRawChannelReader(1, ch)
	writer := NewRawChannelWriter(1, ch)

	payload := []byte("buffer-roundtrip")
	if _, err := writer.Write(payload); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if len(link.sent) < 1 {
		t.Fatal("writer sent nothing")
	}

	for _, raw := range link.sent {
		if err := ch.HandleInbound(raw); err != nil {
			t.Fatalf("HandleInbound: %v", err)
		}
	}

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("got %q want %q (typed StreamDataMessage dispatch likely broken)", got, payload)
	}
}
