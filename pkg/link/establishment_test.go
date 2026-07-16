// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"testing"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

func TestEphemeralKeyGeneration(t *testing.T) {
	link := &Link{}

	if err := link.generateEphemeralKeys(); err != nil {
		t.Fatalf("Failed to generate ephemeral keys: %v", err)
	}

	if bufLen(link.prv) != KeySize {
		t.Errorf("Expected private key length %d, got %d", KeySize, bufLen(link.prv))
	}

	if len(link.pub) != KeySize {
		t.Errorf("Expected public key length %d, got %d", KeySize, len(link.pub))
	}

	if bufLen(link.sigPriv) != 64 {
		t.Errorf("Expected signing private key length 64, got %d", bufLen(link.sigPriv))
	}

	if len(link.sigPub) != 32 {
		t.Errorf("Expected signing public key length 32, got %d", len(link.sigPub))
	}
}

func TestSignallingBytes(t *testing.T) {
	mtu := 500
	mode := byte(ModeAES256CBC)

	bytes := signallingBytes(mtu, mode)

	if len(bytes) != LinkMTUSize {
		t.Errorf("Expected signalling bytes length %d, got %d", LinkMTUSize, len(bytes))
	}

	extractedMTU := (int(bytes[0]&0x1F) << 16) | (int(bytes[1]) << 8) | int(bytes[2])
	if extractedMTU != mtu {
		t.Errorf("Expected MTU %d, got %d", mtu, extractedMTU)
	}

	extractedMode := (bytes[0] & ModeByteMask) >> 5
	if extractedMode != mode {
		t.Errorf("Expected mode %d, got %d", mode, extractedMode)
	}
}

func TestLinkIDGeneration(t *testing.T) {
	responderIdent, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("Failed to create responder identity: %v", err)
	}

	cfg := &common.ReticulumConfig{}
	transportInstance := transport.NewTransport(cfg)

	dest, err := destination.New(responderIdent, destination.In, destination.Single, "test", transportInstance, "link")
	if err != nil {
		t.Fatalf("Failed to create destination: %v", err)
	}

	link := &Link{
		destination: dest,
		transport:   transportInstance,
		initiator:   true,
	}

	if err := link.generateEphemeralKeys(); err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	link.mode = ModeDefault
	link.mtu = 500

	signalling := signallingBytes(link.mtu, link.mode)
	requestData := make([]byte, 0, ECPubSize+LinkMTUSize)
	requestData = append(requestData, link.pub...)
	requestData = append(requestData, link.sigPub...)
	requestData = append(requestData, signalling...)

	pkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeLinkReq,
		TransportType:   0,
		Context:         packet.ContextNone,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: dest.GetType(),
		DestinationHash: dest.GetHash(),
		Data:            requestData,
	}

	if err := pkt.Pack(); err != nil {
		t.Fatalf("Failed to pack packet: %v", err)
	}

	linkID := linkIDFromPacket(pkt)

	if len(linkID) != 16 {
		t.Errorf("Expected link ID length 16, got %d", len(linkID))
	}

	t.Logf("Generated link ID: %x", linkID)
}

func TestHandshake(t *testing.T) {
	link1 := &Link{}
	link2 := &Link{}

	if err := link1.generateEphemeralKeys(); err != nil {
		t.Fatalf("Failed to generate keys for link1: %v", err)
	}

	if err := link2.generateEphemeralKeys(); err != nil {
		t.Fatalf("Failed to generate keys for link2: %v", err)
	}

	link1.peerPub = link2.pub
	link2.peerPub = link1.pub

	link1.linkID = []byte("test-link-id-abc")
	link2.linkID = []byte("test-link-id-abc")

	link1.mode = ModeAES256CBC
	link2.mode = ModeAES256CBC

	if err := link1.performHandshake(); err != nil {
		t.Fatalf("Link1 handshake failed: %v", err)
	}

	if err := link2.performHandshake(); err != nil {
		t.Fatalf("Link2 handshake failed: %v", err)
	}

	if string(bufBytes(link1.sharedKey)) != string(bufBytes(link2.sharedKey)) {
		t.Error("Shared keys do not match")
	}

	if string(bufBytes(link1.derivedKey)) != string(bufBytes(link2.derivedKey)) {
		t.Error("Derived keys do not match")
	}

	if link1.status.Load() != int32(StatusHandshake) {
		t.Errorf("Expected link1 status HANDSHAKE, got %d", link1.status.Load())
	}

	if link2.status.Load() != int32(StatusHandshake) {
		t.Errorf("Expected link2 status HANDSHAKE, got %d", link2.status.Load())
	}
}

func TestLinkEstablishment(t *testing.T) {
	responderIdent, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("Failed to create responder identity: %v", err)
	}

	cfg := &common.ReticulumConfig{}
	transportInstance := transport.NewTransport(cfg)

	dest, err := destination.New(responderIdent, destination.In, destination.Single, "test", transportInstance, "link")
	if err != nil {
		t.Fatalf("Failed to create destination: %v", err)
	}

	initiatorLink := &Link{
		destination: dest,
		transport:   transportInstance,
		initiator:   true,
	}

	responderLink := &Link{
		transport: transportInstance,
		initiator: false,
	}

	if err := initiatorLink.generateEphemeralKeys(); err != nil {
		t.Fatalf("Failed to generate initiator keys: %v", err)
	}

	initiatorLink.mode = ModeDefault
	initiatorLink.mtu = 500

	signalling := signallingBytes(initiatorLink.mtu, initiatorLink.mode)
	requestData := make([]byte, 0, ECPubSize+LinkMTUSize)
	requestData = append(requestData, initiatorLink.pub...)
	requestData = append(requestData, initiatorLink.sigPub...)
	requestData = append(requestData, signalling...)

	linkRequestPkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeLinkReq,
		TransportType:   0,
		Context:         packet.ContextNone,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: dest.GetType(),
		DestinationHash: dest.GetHash(),
		Data:            requestData,
	}

	if err := linkRequestPkt.Pack(); err != nil {
		t.Fatalf("Failed to pack link request: %v", err)
	}

	initiatorLink.linkID = linkIDFromPacket(linkRequestPkt)
	initiatorLink.requestTime = time.Now()
	initiatorLink.status.Store(int32(StatusPending))

	t.Logf("Initiator link request created, link_id=%x", initiatorLink.linkID)

	responderLink.peerPub = linkRequestPkt.Data[0:KeySize]
	responderLink.peerSigPub = linkRequestPkt.Data[KeySize:ECPubSize]
	responderLink.linkID = linkIDFromPacket(linkRequestPkt)
	responderLink.initiator = false

	t.Logf("Responder link ID=%x (len=%d)", responderLink.linkID, len(responderLink.linkID))

	if len(responderLink.linkID) == 0 {
		t.Fatal("Responder link ID is empty!")
	}

	if len(linkRequestPkt.Data) >= ECPubSize+LinkMTUSize {
		mtuBytes := linkRequestPkt.Data[ECPubSize : ECPubSize+LinkMTUSize]
		responderLink.mtu = (int(mtuBytes[0]&0x1F) << 16) | (int(mtuBytes[1]) << 8) | int(mtuBytes[2])
		responderLink.mode = (mtuBytes[0] & ModeByteMask) >> 5
	}

	if err := responderLink.generateEphemeralKeys(); err != nil {
		t.Fatalf("Failed to generate responder keys: %v", err)
	}

	if err := responderLink.performHandshake(); err != nil {
		t.Fatalf("Responder handshake failed: %v", err)
	}

	responderLink.status.Store(int32(StatusActive))
	responderLink.establishedAt = time.Now()

	if string(responderLink.linkID) != string(initiatorLink.linkID) {
		t.Error("Link IDs do not match between initiator and responder")
	}

	t.Logf("Responder handshake successful, shared_key_len=%d", bufLen(responderLink.sharedKey))
}

func TestLinkProofValidation(t *testing.T) {
	responderIdent, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("Failed to create responder identity: %v", err)
	}

	cfg := &common.ReticulumConfig{}
	transportInstance := transport.NewTransport(cfg)

	dest, err := destination.New(responderIdent, destination.In, destination.Single, "test", transportInstance, "link")
	if err != nil {
		t.Fatalf("Failed to create destination: %v", err)
	}

	initiatorLink := &Link{
		destination: dest,
		transport:   transportInstance,
		initiator:   true,
	}

	responderLink := &Link{
		transport: transportInstance,
		initiator: false,
	}

	if err := initiatorLink.generateEphemeralKeys(); err != nil {
		t.Fatalf("Failed to generate initiator keys: %v", err)
	}

	initiatorLink.mode = ModeDefault
	initiatorLink.mtu = 500

	signalling := signallingBytes(initiatorLink.mtu, initiatorLink.mode)
	requestData := make([]byte, 0, ECPubSize+LinkMTUSize)
	requestData = append(requestData, initiatorLink.pub...)
	requestData = append(requestData, initiatorLink.sigPub...)
	requestData = append(requestData, signalling...)

	linkRequestPkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeLinkReq,
		TransportType:   0,
		Context:         packet.ContextNone,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: dest.GetType(),
		DestinationHash: dest.GetHash(),
		Data:            requestData,
	}

	if err := linkRequestPkt.Pack(); err != nil {
		t.Fatalf("Failed to pack link request: %v", err)
	}

	initiatorLink.linkID = linkIDFromPacket(linkRequestPkt)
	initiatorLink.requestTime = time.Now()
	initiatorLink.status.Store(int32(StatusPending))

	responderLink.peerPub = linkRequestPkt.Data[0:KeySize]
	responderLink.peerSigPub = linkRequestPkt.Data[KeySize:ECPubSize]
	responderLink.linkID = linkIDFromPacket(linkRequestPkt)
	responderLink.initiator = false

	if len(linkRequestPkt.Data) >= ECPubSize+LinkMTUSize {
		mtuBytes := linkRequestPkt.Data[ECPubSize : ECPubSize+LinkMTUSize]
		responderLink.mtu = (int(mtuBytes[0]&0x1F) << 16) | (int(mtuBytes[1]) << 8) | int(mtuBytes[2])
		responderLink.mode = (mtuBytes[0] & ModeByteMask) >> 5
	} else {
		responderLink.mtu = 500
		responderLink.mode = ModeDefault
	}

	if err := responderLink.generateEphemeralKeys(); err != nil {
		t.Fatalf("Failed to generate responder keys: %v", err)
	}

	if err := responderLink.performHandshake(); err != nil {
		t.Fatalf("Responder handshake failed: %v", err)
	}

	proofPkt, err := responderLink.GenerateLinkProof(responderIdent)
	if err != nil {
		t.Fatalf("Failed to generate link proof: %v", err)
	}

	if err := initiatorLink.ValidateLinkProof(proofPkt, nil); err != nil {
		t.Fatalf("Initiator failed to validate link proof: %v", err)
	}

	if initiatorLink.status.Load() != int32(StatusActive) {
		t.Errorf("Expected initiator status ACTIVE, got %d", initiatorLink.status.Load())
	}

	if string(bufBytes(initiatorLink.sharedKey)) != string(bufBytes(responderLink.sharedKey)) {
		t.Error("Shared keys do not match after full handshake")
	}

	if string(bufBytes(initiatorLink.derivedKey)) != string(bufBytes(responderLink.derivedKey)) {
		t.Error("Derived keys do not match after full handshake")
	}

	t.Logf("Full link establishment successful")
	t.Logf("Link ID: %x", initiatorLink.linkID)
	t.Logf("Shared key length: %d", bufLen(initiatorLink.sharedKey))
	t.Logf("Derived key length: %d", bufLen(initiatorLink.derivedKey))
	t.Logf("RTT: %.3f seconds", initiatorLink.rtt)
}

func TestParseRTTPayloadSecondsMsgpack(t *testing.T) {
	want := 0.734
	payload, err := msgpack.Marshal(want)
	if err != nil {
		t.Fatalf("failed to encode msgpack payload: %v", err)
	}

	got, err := parseRTTPayloadSeconds(payload)
	if err != nil {
		t.Fatalf("unexpected error parsing msgpack RTT payload: %v", err)
	}
	if got != want {
		t.Fatalf("expected RTT %.3f, got %.3f", want, got)
	}
}

func TestParseRTTPayloadSecondsRejectsNonMsgpack(t *testing.T) {
	payload := []byte{0x00, 0x01, 0x02, 0x03}
	if _, err := parseRTTPayloadSeconds(payload); err == nil {
		t.Fatal("expected parse error for non-msgpack RTT payload")
	}
}
