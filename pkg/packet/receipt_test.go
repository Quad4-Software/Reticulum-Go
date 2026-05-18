// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package packet

import (
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
)

func TestPacketReceiptCreation(t *testing.T) {
	testIdent, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("Failed to create identity: %v", err)
	}

	destHash := testIdent.Hash()
	data := []byte("test packet data")

	pkt := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		TransportType:   0,
		Context:         ContextNone,
		ContextFlag:     FlagUnset,
		Hops:            0,
		DestinationType: 0x00,
		DestinationHash: destHash,
		Data:            data,
		CreateReceipt:   true,
	}

	if err := pkt.Pack(); err != nil {
		t.Fatalf("Failed to pack packet: %v", err)
	}

	receipt := NewPacketReceipt(pkt)
	if receipt == nil {
		t.Fatal("Receipt creation failed")
	}

	if receipt.GetStatus() != ReceiptSent {
		t.Errorf("Expected status SENT, got %d", receipt.GetStatus())
	}

	hash := receipt.GetHash()
	if len(hash) == 0 {
		t.Error("Receipt hash is empty")
	}
}

func TestPacketReceiptTimeout(t *testing.T) {
	testIdent, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("Failed to create identity: %v", err)
	}

	destHash := testIdent.Hash()
	data := []byte("test data")

	pkt := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		TransportType:   0,
		Context:         ContextNone,
		ContextFlag:     FlagUnset,
		Hops:            0,
		DestinationType: 0x00,
		DestinationHash: destHash,
		Data:            data,
		CreateReceipt:   true,
	}

	if err := pkt.Pack(); err != nil {
		t.Fatalf("Failed to pack packet: %v", err)
	}

	receipt := NewPacketReceipt(pkt)
	receipt.SetTimeout(100 * time.Millisecond)

	time.Sleep(150 * time.Millisecond)

	if !receipt.IsTimedOut() {
		t.Error("Receipt should be timed out")
	}
}

func TestPacketReceiptProofValidation(t *testing.T) {
	testIdent, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("Failed to create identity: %v", err)
	}

	destHash := testIdent.Hash()
	data := []byte("test data")

	pkt := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		TransportType:   0,
		Context:         ContextNone,
		ContextFlag:     FlagUnset,
		Hops:            0,
		DestinationType: 0x00,
		DestinationHash: destHash,
		Data:            data,
		CreateReceipt:   true,
	}

	if err := pkt.Pack(); err != nil {
		t.Fatalf("Failed to pack packet: %v", err)
	}

	receipt := NewPacketReceipt(pkt)
	receipt.SetDestinationIdentity(testIdent)

	packetHash := pkt.GetHash()
	t.Logf("Packet hash: %x", packetHash)

	signature, err := testIdent.Sign(packetHash)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	t.Logf("PacketHash length: %d", len(packetHash))
	t.Logf("Signature length: %d", len(signature))
	t.Logf("ExplicitLength constant: %d", ExplicitLength)

	if testIdent.Verify(packetHash, signature) {
		t.Log("Direct verification succeeded")
	} else {
		t.Error("Direct verification failed")
	}

	proof := make([]byte, 0, ExplicitLength)
	proof = append(proof, packetHash...)
	proof = append(proof, signature...)

	t.Logf("Proof length: %d", len(proof))

	proofPacket := &Packet{
		PacketType: PacketTypeProof,
		Data:       proof,
	}

	if !receipt.ValidateProof(proof, proofPacket) {
		t.Errorf("Valid proof was rejected. Proof len=%d, expected=%d", len(proof), ExplicitLength)
	}

	if receipt.GetStatus() != ReceiptDelivered {
		t.Errorf("Expected status DELIVERED, got %d", receipt.GetStatus())
	}

	if !receipt.IsDelivered() {
		t.Error("Receipt should be marked as delivered")
	}
}

func TestPacketReceiptCallbacks(t *testing.T) {
	testIdent, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("Failed to create identity: %v", err)
	}

	destHash := testIdent.Hash()
	data := []byte("test data")

	pkt := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		TransportType:   0,
		Context:         ContextNone,
		ContextFlag:     FlagUnset,
		Hops:            0,
		DestinationType: 0x00,
		DestinationHash: destHash,
		Data:            data,
		CreateReceipt:   true,
	}

	if err := pkt.Pack(); err != nil {
		t.Fatalf("Failed to pack packet: %v", err)
	}

	receipt := NewPacketReceipt(pkt)
	receipt.SetDestinationIdentity(testIdent)

	deliveryCalled := make(chan bool, 1)
	receipt.SetDeliveryCallback(func(r *PacketReceipt) {
		deliveryCalled <- true
	})

	packetHash := pkt.GetHash()
	signature, err := testIdent.Sign(packetHash)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	proof := make([]byte, 0, ExplicitLength)
	proof = append(proof, packetHash...)
	proof = append(proof, signature...)

	proofPacket := &Packet{
		PacketType: PacketTypeProof,
		Data:       proof,
	}

	receipt.ValidateProof(proof, proofPacket)

	select {
	case <-deliveryCalled:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Error("Delivery callback was not called")
	}
}

func TestPacketReceiptGetHashReturnsCopy(t *testing.T) {
	testIdent, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("Failed to create identity: %v", err)
	}

	pkt := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		TransportType:   0,
		Context:         ContextNone,
		ContextFlag:     FlagUnset,
		Hops:            0,
		DestinationType: 0x00,
		DestinationHash: testIdent.Hash(),
		Data:            []byte("hash-copy"),
		CreateReceipt:   true,
	}
	if err := pkt.Pack(); err != nil {
		t.Fatalf("Failed to pack packet: %v", err)
	}

	receipt := NewPacketReceipt(pkt)

	h1 := receipt.GetHash()
	if len(h1) == 0 {
		t.Fatal("empty receipt hash")
	}
	h1[0] ^= 0xFF

	h2 := receipt.GetHash()
	if h1[0] == h2[0] {
		t.Fatal("GetHash returned aliased internal slice")
	}
	if !receipt.MatchesHash(h2) {
		t.Fatal("MatchesHash should match fresh hash bytes")
	}
}
