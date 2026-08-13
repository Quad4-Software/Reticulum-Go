// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package channel

import (
	"errors"
	"testing"

	"quad4/reticulum-go/pkg/transport"
)

func TestHandleInboundReordersAndDropsDuplicates(t *testing.T) {
	link := &mockLink{status: transport.StatusActive}
	c := NewChannel(link)
	defer func() { _ = c.Close() }()

	if err := c.RegisterMessageType(1, func() MessageBase { return &testMessage{} }); err != nil {
		t.Fatal(err)
	}

	var got []string
	c.AddMessageHandler(func(msg MessageBase) bool {
		if m, ok := msg.(*testMessage); ok {
			got = append(got, string(m.data))
		}
		return true
	})

	raw1, err := packEnvelope(1, 1, []byte("B"))
	if err != nil {
		t.Fatal(err)
	}
	raw0, err := packEnvelope(1, 0, []byte("A"))
	if err != nil {
		t.Fatal(err)
	}

	if err := c.HandleInbound(raw1); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("seq 1 before 0 dispatched %v", got)
	}
	if err := c.HandleInbound(raw0); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Fatalf("order=%v want [A B]", got)
	}

	if err := c.HandleInbound(raw0); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("duplicate seq 0 dispatched, got %v", got)
	}
}

func TestSendRefusesFullWindow(t *testing.T) {
	link := &mockLink{status: transport.StatusActive}
	c := NewChannel(link)
	defer func() { _ = c.Close() }()

	for i := 0; i < WindowInitial; i++ {
		if err := c.Send(&testMessage{data: []byte{byte(i)}}); err != nil {
			t.Fatalf("setup send %d: %v", i, err)
		}
	}
	if c.IsReadyToSend() {
		t.Fatal("window should be full")
	}
	err := c.Send(&testMessage{data: []byte("overflow")})
	if !errors.Is(err, ErrLinkNotReady) {
		t.Fatalf("Send: got %v want ErrLinkNotReady", err)
	}
	if c.TxRingLen() != WindowInitial {
		t.Fatalf("tx ring len=%d want %d", c.TxRingLen(), WindowInitial)
	}
}

func TestSendRefusesOversizeEnvelope(t *testing.T) {
	link := &mockLink{status: transport.StatusActive}
	c := NewChannel(link)
	defer func() { _ = c.Close() }()

	body := make([]byte, c.MDU()+64)
	err := c.Send(&testMessage{data: body})
	if !errors.Is(err, ErrTooBig) {
		t.Fatalf("Send: got %v want ErrTooBig", err)
	}
	if c.TxRingLen() != 0 {
		t.Fatalf("tx ring len=%d want 0", c.TxRingLen())
	}
	if c.NextSequence() != 0 {
		t.Fatalf("sequence consumed on oversized send: %d", c.NextSequence())
	}
}

func TestStaleRXSequenceWrapWindow(t *testing.T) {
	if staleRXSequence(5, 10) != true {
		t.Fatal("seq 5 must be stale when next is 10")
	}
	if staleRXSequence(10, 10) != false {
		t.Fatal("current next sequence is not stale")
	}
	if staleRXSequence(0, 65530) != false {
		t.Fatal("seq 0 after wrap must be accepted while next is 65530")
	}
	if staleRXSequence(100, 65530) != true {
		t.Fatal("seq 100 must be stale when next is 65530")
	}
}
