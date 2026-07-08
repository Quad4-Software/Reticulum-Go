// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package packet

import (
	"bytes"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/identity"
)

type mockLinkValidator struct {
	valid bool
}

func (m *mockLinkValidator) Validate(signature, message []byte) bool {
	return m.valid
}

func packedReceiptPacket(t *testing.T, hops byte) (*Packet, *identity.Identity) {
	t.Helper()
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	pkt := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		Context:         ContextNone,
		Hops:            hops,
		DestinationHash: id.Hash(),
		Data:            []byte("receipt-mutation"),
		CreateReceipt:   true,
	}
	if err := pkt.Pack(); err != nil {
		t.Fatal(err)
	}
	return pkt, id
}

func explicitProof(t *testing.T, id *identity.Identity, pkt *Packet) []byte {
	t.Helper()
	hash := pkt.GetHash()
	sig, err := id.Sign(hash)
	if err != nil {
		t.Fatal(err)
	}
	proof := make([]byte, 0, ExplicitLength)
	proof = append(proof, hash...)
	proof = append(proof, sig...)
	return proof
}

func TestReceiptValidateProofRejectsWrongHash(t *testing.T) {
	pkt, id := packedReceiptPacket(t, 0)
	receipt := NewPacketReceipt(pkt)
	receipt.SetDestinationIdentity(id)

	proof := explicitProof(t, id, pkt)
	proof[0] ^= 0xFF

	if receipt.ValidateProof(proof, &Packet{PacketType: PacketTypeProof, Data: proof}) {
		t.Fatal("expected invalid proof for hash mismatch")
	}
	if receipt.IsDelivered() {
		t.Fatal("status should not be delivered")
	}
}

func TestReceiptValidateProofRejectsBadSignature(t *testing.T) {
	pkt, id := packedReceiptPacket(t, 0)
	receipt := NewPacketReceipt(pkt)
	receipt.SetDestinationIdentity(id)

	proof := explicitProof(t, id, pkt)
	proof[len(proof)-1] ^= 0xFF

	if receipt.ValidateProof(proof, &Packet{PacketType: PacketTypeProof, Data: proof}) {
		t.Fatal("expected invalid proof for bad signature")
	}
}

func TestReceiptValidateProofRejectsWithoutIdentity(t *testing.T) {
	pkt, id := packedReceiptPacket(t, 0)
	receipt := NewPacketReceipt(pkt)

	proof := explicitProof(t, id, pkt)
	if receipt.ValidateProof(proof, &Packet{PacketType: PacketTypeProof, Data: proof}) {
		t.Fatal("expected rejection without destination identity")
	}
}

func TestReceiptValidateProofRejectsWrongLength(t *testing.T) {
	pkt, id := packedReceiptPacket(t, 0)
	receipt := NewPacketReceipt(pkt)
	receipt.SetDestinationIdentity(id)

	if receipt.ValidateProof([]byte{1, 2, 3}, &Packet{PacketType: PacketTypeProof}) {
		t.Fatal("expected rejection for short proof")
	}
}

func TestReceiptValidateImplicitProof(t *testing.T) {
	pkt, id := packedReceiptPacket(t, 0)
	receipt := NewPacketReceipt(pkt)
	receipt.SetDestinationIdentity(id)

	hash := pkt.GetHash()
	sig, err := id.Sign(hash)
	if err != nil {
		t.Fatal(err)
	}
	if len(sig) != ImplicitLength {
		t.Fatalf("implicit proof sig len=%d want %d", len(sig), ImplicitLength)
	}

	proofPacket := &Packet{PacketType: PacketTypeProof, Data: sig}
	if !receipt.ValidateProof(sig, proofPacket) {
		t.Fatal("valid implicit proof rejected")
	}
	if !receipt.IsDelivered() {
		t.Fatal("expected delivered after implicit proof")
	}
}

func TestReceiptValidateLinkProof(t *testing.T) {
	pkt, id := packedReceiptPacket(t, 0)
	receipt := NewPacketReceipt(pkt)
	link := &mockLinkValidator{valid: true}

	proof := explicitProof(t, id, pkt)
	proofPacket := &Packet{PacketType: PacketTypeProof, Data: proof, Link: link}
	if !receipt.ValidateLinkProof(proof, link, proofPacket) {
		t.Fatal("valid link proof rejected")
	}
	if !receipt.IsDelivered() {
		t.Fatal("expected delivered after link proof")
	}
}

func TestReceiptValidateLinkProofRejectsInvalidSignature(t *testing.T) {
	pkt, id := packedReceiptPacket(t, 0)
	receipt := NewPacketReceipt(pkt)
	link := &mockLinkValidator{valid: false}

	proof := explicitProof(t, id, pkt)
	if receipt.ValidateLinkProof(proof, link, &Packet{PacketType: PacketTypeProof, Data: proof}) {
		t.Fatal("expected link proof rejection")
	}
}

func TestReceiptValidateProofPacketRoutesLink(t *testing.T) {
	pkt, id := packedReceiptPacket(t, 0)
	receipt := NewPacketReceipt(pkt)
	link := &mockLinkValidator{valid: true}

	proof := explicitProof(t, id, pkt)
	proofPacket := &Packet{PacketType: PacketTypeProof, Data: proof, Link: link}
	if !receipt.ValidateProofPacket(proofPacket) {
		t.Fatal("ValidateProofPacket should accept link proof")
	}
}

func TestReceiptTimeoutMarksFailed(t *testing.T) {
	pkt, _ := packedReceiptPacket(t, 0)
	receipt := NewPacketReceipt(pkt)
	receipt.SetTimeout(50 * time.Millisecond)

	timeoutCalled := make(chan struct{}, 1)
	receipt.SetTimeoutCallback(func(*PacketReceipt) {
		timeoutCalled <- struct{}{}
	})

	waitForReceiptStatus(t, receipt, ReceiptFailed, 2*time.Second)

	if !receipt.IsFailed() {
		t.Fatalf("status=%d want failed", receipt.GetStatus())
	}
	if receipt.IsDelivered() {
		t.Fatal("should not be delivered")
	}

	select {
	case <-timeoutCalled:
	case <-time.After(time.Second):
		t.Fatal("timeout callback not invoked")
	}
}

func TestReceiptNegativeTimeoutCulled(t *testing.T) {
	pkt, _ := packedReceiptPacket(t, 0)
	receipt := NewPacketReceipt(pkt)
	receipt.SetTimeout(-1 * time.Second)

	waitForReceiptStatus(t, receipt, ReceiptCulled, 2*time.Second)
	if receipt.GetStatus() != ReceiptCulled {
		t.Fatalf("status=%d want culled", receipt.GetStatus())
	}
}

func TestReceiptCancelCullsInFlight(t *testing.T) {
	pkt, _ := packedReceiptPacket(t, 0)
	receipt := NewPacketReceipt(pkt)
	receipt.Cancel()

	if receipt.GetStatus() != ReceiptCulled {
		t.Fatalf("status=%d want culled", receipt.GetStatus())
	}
}

func TestReceiptGetRTTAfterDelivery(t *testing.T) {
	pkt, id := packedReceiptPacket(t, 0)
	receipt := NewPacketReceipt(pkt)
	receipt.SetDestinationIdentity(id)

	proof := explicitProof(t, id, pkt)
	if !receipt.ValidateProof(proof, &Packet{PacketType: PacketTypeProof, Data: proof}) {
		t.Fatal("proof validation failed")
	}
	if !receipt.IsDelivered() {
		t.Fatal("receipt not delivered")
	}
	if rtt := receipt.GetRTT(); rtt < 0 {
		t.Fatalf("expected non-negative RTT, got %v", rtt)
	}
}

func TestReceiptMatchesHash(t *testing.T) {
	pkt, id := packedReceiptPacket(t, 0)
	receipt := NewPacketReceipt(pkt)

	want := pkt.GetHash()
	if !receipt.MatchesHash(want) {
		t.Fatal("MatchesHash should accept packet hash")
	}
	bad := append([]byte(nil), want...)
	bad[0] ^= 0x01
	if receipt.MatchesHash(bad) {
		t.Fatal("MatchesHash should reject wrong hash")
	}
	_ = id
}

func TestReceiptDeliveredNotFailed(t *testing.T) {
	pkt, id := packedReceiptPacket(t, 2)
	receipt := NewPacketReceipt(pkt)
	receipt.SetDestinationIdentity(id)

	if receipt.IsDelivered() || receipt.IsFailed() {
		t.Fatal("new receipt should be in-flight only")
	}

	proof := explicitProof(t, id, pkt)
	if !receipt.ValidateProof(proof, &Packet{PacketType: PacketTypeProof, Data: proof}) {
		t.Fatal("proof failed")
	}
	if !receipt.IsDelivered() || receipt.IsFailed() {
		t.Fatal("delivered receipt should not be failed")
	}
}

func waitForReceiptStatus(t *testing.T, receipt *PacketReceipt, want byte, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if receipt.GetStatus() == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("status=%d want %d after %v", receipt.GetStatus(), want, timeout)
}

func TestReceiptLinkProofRejectsHashMismatch(t *testing.T) {
	pkt, id := packedReceiptPacket(t, 0)
	receipt := NewPacketReceipt(pkt)
	link := &mockLinkValidator{valid: true}

	proof := explicitProof(t, id, pkt)
	proof[1] ^= 0xFF
	if receipt.ValidateLinkProof(proof, link, &Packet{PacketType: PacketTypeProof, Data: proof}) {
		t.Fatal("expected rejection for link hash mismatch")
	}
}

func TestReceiptImplicitProofRejectsWithoutIdentity(t *testing.T) {
	pkt, id := packedReceiptPacket(t, 0)
	receipt := NewPacketReceipt(pkt)

	sig, err := id.Sign(pkt.GetHash())
	if err != nil {
		t.Fatal(err)
	}
	if receipt.ValidateProof(sig, &Packet{PacketType: PacketTypeProof, Data: sig}) {
		t.Fatal("implicit proof without identity should fail")
	}
}

func TestReceiptImplicitLinkLengthIgnored(t *testing.T) {
	pkt, _ := packedReceiptPacket(t, 0)
	receipt := NewPacketReceipt(pkt)
	link := &mockLinkValidator{valid: true}

	sig := bytes.Repeat([]byte{0xAB}, ImplicitLength)
	if receipt.ValidateLinkProof(sig, link, &Packet{PacketType: PacketTypeProof, Data: sig}) {
		t.Fatal("implicit link proof is not implemented")
	}
}

func TestCalculateTimeoutScalesWithHops(t *testing.T) {
	pkt := &Packet{Hops: 4}
	got := calculateTimeout(pkt)
	want := time.Duration(ReceiptTimeoutBaseSec)*time.Second +
		4*time.Duration(ReceiptTimeoutPerHopSec)*time.Second
	if got != want {
		t.Fatalf("timeout=%v want %v", got, want)
	}
	if got := calculateTimeout(&Packet{Hops: 0}); got != time.Duration(ReceiptTimeoutBaseSec)*time.Second {
		t.Fatalf("zero-hop timeout=%v", got)
	}
}

func TestReceiptWatchdogStopsAfterDelivery(t *testing.T) {
	pkt, id := packedReceiptPacket(t, 0)
	receipt := NewPacketReceipt(pkt)
	receipt.SetDestinationIdentity(id)

	proof := explicitProof(t, id, pkt)
	if !receipt.ValidateProof(proof, &Packet{PacketType: PacketTypeProof, Data: proof}) {
		t.Fatal("proof failed")
	}

	time.Sleep(1200 * time.Millisecond)
	if !receipt.IsDelivered() {
		t.Fatal("receipt should remain delivered")
	}
	if receipt.IsFailed() {
		t.Fatal("delivered receipt should not transition to failed")
	}
}
