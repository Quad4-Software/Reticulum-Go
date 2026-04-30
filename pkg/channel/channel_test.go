// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package channel

import (
	"bytes"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/packet"
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

	msg := &testMessage{data: []byte("test")}
	err := c.Send(msg)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if len(link.sent) != 1 {
		t.Errorf("Expected 1 packet sent, got %d", len(link.sent))
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
