// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/packet"
)

// TestResponderProofUsesDestinationIdentity catches the NomadNet 50% bug:
// Python validates link DATA proofs against the destination identity signing
// key (peer_sig_pub), not an ephemeral link Ed25519 key.
func TestResponderProofUsesDestinationIdentity(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	owner := respLink.destination.GetIdentity()
	if owner == nil {
		t.Fatal("responder destination has no identity")
	}
	wantPub := owner.GetSigningKey()
	if !bytes.Equal(respLink.sigPub, wantPub) {
		t.Fatalf("responder sigPub is not owner identity signing key")
	}
	if !bytes.Equal(initLink.peerSigPub, wantPub) {
		t.Fatalf("initiator peerSigPub is not destination identity signing key")
	}

	hash := make([]byte, 32)
	if _, err := rand.Read(hash); err != nil {
		t.Fatalf("rand: %v", err)
	}
	sig, err := owner.Sign(hash)
	if err != nil {
		t.Fatalf("owner.Sign: %v", err)
	}
	if !initLink.Validate(sig, hash) {
		t.Fatal("initiator Validate rejected destination-identity proof (Python NomadNet path)")
	}

	// Ephemeral signing key must not validate as a delivery proof.
	ephemPub, ephemPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ephemeral key: %v", err)
	}
	_ = ephemPub
	bad := ed25519.Sign(ephemPriv, hash)
	if initLink.Validate(bad, hash) {
		t.Fatal("initiator Validate accepted ephemeral proof (would leave senders at 50%)")
	}
}

func TestProveAllMarksLinkReceiptDelivered(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)

	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	respLink.destination.SetProofStrategy(destination.ProveAll)

	delivered := make(chan struct{}, 1)
	var once sync.Once

	plain := []byte("lxmf-direct-delivery-proof")
	enc, err := initLink.encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	pkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		TransportType:   0,
		Context:         packet.ContextNone,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: DestTypeLink,
		DestinationHash: initLink.GetLinkID(),
		Data:            enc,
		CreateReceipt:   false,
		Link:            initLink,
	}
	if err := pkt.Pack(); err != nil {
		t.Fatalf("pack: %v", err)
	}

	receipt := packet.NewPacketReceipt(pkt)
	receipt.SetLink(initLink)
	receipt.SetDeliveryCallback(func(*packet.PacketReceipt) {
		once.Do(func() { close(delivered) })
	})
	initLink.transport.RegisterReceipt(receipt)
	defer receipt.Cancel()

	initLink.recordOutboundData()
	if err := initLink.transport.SendPacket(pkt); err != nil {
		t.Fatalf("SendPacket: %v", err)
	}

	select {
	case <-delivered:
		if !receipt.IsDelivered() {
			t.Fatal("callback fired but receipt not DELIVERED")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for DELIVERED proof status=%d hash=%x", receipt.GetStatus(), receipt.GetHash()[:8])
	}
}

func TestProvePacketSignatureVector(t *testing.T) {
	_, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	owner := respLink.destination.GetIdentity()
	sum := sha256.Sum256([]byte("vector"))
	hash := sum[:]
	sig, err := owner.Sign(hash)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if !ed25519.Verify(owner.GetSigningKey(), hash, sig) {
		t.Fatal("owner signature does not verify")
	}
	if !ed25519.Verify(respLink.sigPub, hash, sig) {
		t.Fatal("responder sigPub does not verify owner signature")
	}
}
