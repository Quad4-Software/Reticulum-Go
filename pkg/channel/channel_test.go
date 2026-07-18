// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package channel

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

type mockLink struct {
	status    byte
	rtt       float64
	sent      [][]byte
	timeouts  map[any]func(any)
	delivered map[any]func(any)
}

func (m *mockLink) GetStatus() byte   { return m.status }
func (m *mockLink) GetRTT() float64   { return m.rtt }
func (m *mockLink) RTT() float64      { return m.rtt }
func (m *mockLink) GetLinkID() []byte { return []byte("testlink") }
func (m *mockLink) Send(data []byte) any {
	m.sent = append(m.sent, data)
	p := &packet.Packet{Raw: data}
	return p
}
func (m *mockLink) Resend(p any) error { return nil }
func (m *mockLink) SetPacketTimeout(p any, cb func(any), t time.Duration) {
	if m.timeouts == nil {
		m.timeouts = make(map[any]func(any))
	}
	m.timeouts[p] = cb
}
func (m *mockLink) SetPacketDelivered(p any, cb func(any)) {
	if m.delivered == nil {
		m.delivered = make(map[any]func(any))
	}
	m.delivered[p] = cb
}
func (m *mockLink) HandleInbound(pkt *packet.Packet) error { return nil }
func (m *mockLink) ValidateLinkProof(pkt *packet.Packet, networkIface common.NetworkInterface) error {
	return nil
}
func (m *mockLink) LinkedNetworkInterface() common.NetworkInterface { return nil }

type testMessage struct {
	data []byte
}

func (m *testMessage) Pack() ([]byte, error)    { return m.data, nil }
func (m *testMessage) Unpack(data []byte) error { m.data = data; return nil }
func (m *testMessage) GetType() uint16          { return 1 }

func TestNewChannel(t *testing.T) {
	link := &mockLink{}
	c := NewChannel(link)
	if c == nil {
		t.Fatal("NewChannel returned nil")
	}
}

func TestChannelSend(t *testing.T) {
	link := &mockLink{status: 1} // StatusActive
	c := NewChannel(link)
	defer func() { _ = c.Close() }()

	msg := &testMessage{data: []byte("test")}
	err := c.Send(msg)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if len(link.sent) != 1 {
		t.Fatalf("Expected 1 packet sent, got %d", len(link.sent))
	}
	raw := link.sent[0]
	if len(raw) != ChannelHeaderSize+4 {
		t.Fatalf("envelope len=%d want %d", len(raw), ChannelHeaderSize+4)
	}
	if raw[0] != 0 || raw[1] != 1 {
		t.Fatalf("msgtype bytes=%x want 0001", raw[0:2])
	}
	if raw[2] != 0 || raw[3] != 0 {
		t.Fatalf("sequence bytes=%x want 0000", raw[2:4])
	}
	if raw[4] != 0 || raw[5] != 4 {
		t.Fatalf("length bytes=%x want 0004", raw[4:6])
	}
	if string(raw[ChannelHeaderSize:]) != "test" {
		t.Fatalf("body=%q", raw[ChannelHeaderSize:])
	}
}

func TestHandleInboundTypedFactory(t *testing.T) {
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
			t.Fatalf("expected *testMessage, got %T", m)
		}
		got = tm
		return true
	})

	raw, err := packEnvelope(1, 7, []byte("abcd"))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.HandleInbound(raw); err != nil {
		t.Fatal(err)
	}
	if got == nil || string(got.data) != "abcd" {
		t.Fatalf("got=%v", got)
	}
}

func TestHandleInbound(t *testing.T) {
	link := &mockLink{}
	c := NewChannel(link)

	received := false
	c.AddMessageHandler(func(m MessageBase) bool {
		received = true
		return true
	})

	// Packet format: [type 2][seq 2][len 2][data]
	data := []byte{0, 1, 0, 1, 0, 4, 't', 'e', 's', 't'}
	err := c.HandleInbound(data)
	if err != nil {
		t.Fatalf("HandleInbound failed: %v", err)
	}

	if !received {
		t.Error("Message handler was not called")
	}
}

func TestMessageHandlers(t *testing.T) {
	c := &Channel{
		messageHandlers: make([]messageHandlerEntry, 0),
	}
	h := func(m MessageBase) bool { return true }

	id := c.AddMessageHandler(h)
	if len(c.messageHandlers) != 1 {
		t.Errorf("Expected 1 handler, got %d", len(c.messageHandlers))
	}

	c.RemoveMessageHandler(id)
	if len(c.messageHandlers) != 0 {
		t.Errorf("Expected 0 handlers, got %d", len(c.messageHandlers))
	}
}

func TestGenericMessage(t *testing.T) {
	msg := &GenericMessage{Type: 1, Data: []byte("test")}
	if msg.GetType() != 1 {
		t.Error("Wrong type")
	}
	p, _ := msg.Pack()
	if !bytes.Equal(p, []byte("test")) {
		t.Error("Pack failed")
	}
	msg.Unpack([]byte("new"))
	if !bytes.Equal(msg.Data, []byte("new")) {
		t.Error("Unpack failed")
	}
}

type scaleMockLink struct {
	status byte
}

func (m *scaleMockLink) GetStatus() byte                                       { return m.status }
func (m *scaleMockLink) GetRTT() float64                                       { return 0.1 }
func (m *scaleMockLink) RTT() float64                                          { return 0.1 }
func (m *scaleMockLink) GetLinkID() []byte                                     { return []byte("mocklink") }
func (m *scaleMockLink) Send(data []byte) any                                  { return "packet" }
func (m *scaleMockLink) Resend(p any) error                                    { return nil }
func (m *scaleMockLink) SetPacketTimeout(p any, cb func(any), t time.Duration) {}
func (m *scaleMockLink) SetPacketDelivered(p any, cb func(any))                {}
func (m *scaleMockLink) HandleInbound(pkt *packet.Packet) error                { return nil }
func (m *scaleMockLink) ValidateLinkProof(pkt *packet.Packet, iface common.NetworkInterface) error {
	return nil
}
func (m *scaleMockLink) LinkedNetworkInterface() common.NetworkInterface { return nil }

func BenchmarkChannelScale(b *testing.B) {
	sizes := []int{10, 100, 1000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Handlers-%d", size), func(b *testing.B) {
			link := &scaleMockLink{status: transport.StatusActive}
			ch := NewChannel(link)

			for range size {
				ch.AddMessageHandler(func(m MessageBase) bool {
					return false // continue to next handler
				})
			}

			msg := &GenericMessage{Type: 1, Data: []byte("benchmark")}
			data := make([]byte, ChannelHeaderSize+len(msg.Data))
			// Manual pack for speed
			data[0], data[1] = 0, 1
			data[4], data[5] = 0, byte(len(msg.Data))
			copy(data[ChannelHeaderSize:], msg.Data)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = ch.HandleInbound(data)
			}
		})
	}
}

func BenchmarkChannelSendScale(b *testing.B) {
	link := &scaleMockLink{status: transport.StatusActive}
	ch := NewChannel(link)
	msg := &GenericMessage{Type: 1, Data: []byte("benchmark")}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = ch.Send(msg)
		if i%100 == 0 {
			ch.mutex.Lock()
			for _, env := range ch.txRing {
				releaseEnvelope(env)
			}
			ch.txRing = nil
			ch.mutex.Unlock()
		}
	}
	ch.mutex.Lock()
	for _, env := range ch.txRing {
		releaseEnvelope(env)
	}
	ch.txRing = nil
	ch.mutex.Unlock()
}

func TestSequenceWrapsAtModulus(t *testing.T) {
	link := &mockLink{status: transport.StatusActive}
	c := NewChannel(link)
	defer func() { _ = c.Close() }()
	c.nextSequence = SeqMax
	if err := c.Send(&testMessage{data: []byte("wrap")}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if c.nextSequence != 0 {
		t.Fatalf("nextSequence=%d want 0 after SeqMax", c.nextSequence)
	}
	if len(link.sent) != 1 {
		t.Fatalf("sent=%d want 1", len(link.sent))
	}
}
