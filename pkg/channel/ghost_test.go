// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package channel

import (
	"errors"
	"testing"

	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

type failingLink struct {
	mockLink
	fail bool
}

func (f *failingLink) Send(data []byte) any {
	if f.fail {
		return nil
	}
	return f.mockLink.Send(data)
}

func TestSendOnFailingOutletDoesNotCorruptState(t *testing.T) {
	link := &failingLink{
		mockLink: mockLink{status: transport.StatusActive},
		fail:     true,
	}
	c := NewChannel(link)
	defer func() { _ = c.Close() }()

	msg := &testMessage{data: []byte("ghost")}
	err := c.Send(msg)
	if !errors.Is(err, ErrLinkNotReady) {
		t.Fatalf("Send: got %v, want ErrLinkNotReady", err)
	}
	if c.TxRingLen() != 0 {
		t.Fatalf("tx ring len: got %d, want 0 (no ghost envelope)", c.TxRingLen())
	}
	if c.NextSequence() != 0 {
		t.Fatalf("next sequence: got %d, want 0 (rewound)", c.NextSequence())
	}

	link.fail = false
	if err := c.Send(msg); err != nil {
		t.Fatalf("Send after recovery: %v", err)
	}
	if c.TxRingLen() != 1 {
		t.Fatalf("tx ring after success: got %d, want 1", c.TxRingLen())
	}
	if c.NextSequence() != 1 {
		t.Fatalf("next sequence after success: got %d, want 1", c.NextSequence())
	}
}

func TestSendAcceptsLinkStatusActive(t *testing.T) {
	link := &mockLink{status: 0x02}
	c := NewChannel(link)
	defer func() { _ = c.Close() }()
	if err := c.Send(&testMessage{data: []byte("ok")}); err != nil {
		t.Fatalf("Send with link ACTIVE status: %v", err)
	}
}

func TestHandleTimeoutIgnoresNilPacket(t *testing.T) {
	link := &mockLink{status: transport.StatusActive}
	c := NewChannel(link)
	defer func() { _ = c.Close() }()
	c.handleTimeout(nil)
	c.handleDelivered(nil)
}

func TestPacketTransmittedRequiresRaw(t *testing.T) {
	if packetTransmitted(nil) {
		t.Fatal("nil should not count as transmitted")
	}
	if packetTransmitted(&packet.Packet{}) {
		t.Fatal("empty Raw should not count as transmitted")
	}
	if !packetTransmitted(&packet.Packet{Raw: []byte{1}}) {
		t.Fatal("non-empty Raw should count as transmitted")
	}
}
