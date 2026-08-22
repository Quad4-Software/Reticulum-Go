// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/health"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
)

type announceAuthSpy struct {
	calls atomic.Int32
}

func (s *announceAuthSpy) AspectFilter() []string     { return nil }
func (s *announceAuthSpy) ReceivePathResponses() bool { return true }
func (s *announceAuthSpy) ReceivedAnnounce([]byte, any, []byte, uint8) error {
	s.calls.Add(1)
	return nil
}

func announceDestHashOffset(raw []byte) int {
	if len(raw) < 2 {
		return -1
	}
	headerType := (raw[0] & 0x40) >> 6
	if headerType == 0 {
		return 2
	}
	return 2 + 16
}

func announcePayloadStart(raw []byte) int {
	if len(raw) < 2 {
		return -1
	}
	headerType := (raw[0] & 0x40) >> 6
	if headerType == 0 {
		return 2 + 16 + 1
	}
	return 2 + 16 + 16 + 1
}

func announceSignatureOffset(raw []byte) int {
	start := announcePayloadStart(raw)
	if start < 0 || len(raw) < start+64+10+10+64 {
		return -1
	}
	return start + 64 + 10 + 10
}

func signedAnnounceRaw(t *testing.T, tr *Transport, id *identity.Identity) (raw, destHash []byte) {
	t.Helper()
	dest, err := destination.New(id, destination.In, destination.Single, "oracle-auth", tr, "node")
	if err != nil {
		t.Fatalf("destination.New: %v", err)
	}
	transportID := make([]byte, 16)
	pkt, err := packet.NewAnnouncePacket(dest.GetHash(), id, []byte("oracle"), transportID)
	if err != nil {
		t.Fatalf("NewAnnouncePacket: %v", err)
	}
	raw, err = pkt.Serialize()
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	return raw, dest.GetHash()
}

// Guarantee: bad announce signature does not Remember, register path, or notify handlers.
func TestOracleBadAnnounceSignatureLeavesStateClean(t *testing.T) {
	health.Default.Reset()
	tr := NewTransport(common.DefaultConfig())
	defer tr.Close()

	iface := &mockInterface{}
	iface.Name = "oracle-announce"
	iface.Enabled = true
	iface.Online = true
	if err := tr.RegisterInterface("oracle-announce", iface); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}

	spy := &announceAuthSpy{}
	tr.RegisterAnnounceHandler(spy)

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	annRaw, destHash := signedAnnounceRaw(t, tr, id)

	if _, err := identity.Recall(destHash); err == nil {
		t.Fatal("Recall succeeded before announce")
	}

	badSig := append([]byte(nil), annRaw...)
	sigStart := announceSignatureOffset(badSig)
	if sigStart < 0 {
		t.Fatal("could not locate announce signature")
	}
	badSig[sigStart] ^= 0x01

	tr.HandlePacket(badSig, iface)
	waitInboundDrain(t, tr, 200*time.Millisecond)

	if _, err := identity.Recall(destHash); err == nil {
		t.Fatal("Remember ran after bad announce signature")
	}
	if tr.HasPath(destHash) {
		t.Fatal("path registered after bad announce signature")
	}
	if spy.calls.Load() != 0 {
		t.Fatalf("announce handler calls=%d want 0", spy.calls.Load())
	}
	snap := health.Default.SnapshotIface("oracle-announce")
	if snap.AnnounceSigFail.Total == 0 {
		t.Fatal("expected announce_sig_fail counter increment")
	}
}

// Guarantee: destination hash mismatch after signature tamper still leaves state clean.
func TestOracleBadAnnounceDestHashLeavesStateClean(t *testing.T) {
	tr := NewTransport(common.DefaultConfig())
	defer tr.Close()

	iface := &mockInterface{}
	iface.Name = "oracle-dest"
	iface.Enabled = true
	iface.Online = true
	if err := tr.RegisterInterface("oracle-dest", iface); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}

	spy := &announceAuthSpy{}
	tr.RegisterAnnounceHandler(spy)

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	annRaw, destHash := signedAnnounceRaw(t, tr, id)

	tampered := append([]byte(nil), annRaw...)
	destStart := announceDestHashOffset(tampered)
	if destStart < 0 || len(tampered) <= destStart {
		t.Fatal("announce too short for destination hash")
	}
	tampered[destStart] ^= 0x01

	tr.HandlePacket(tampered, iface)
	waitInboundDrain(t, tr, 200*time.Millisecond)

	if _, err := identity.Recall(destHash); err == nil {
		t.Fatal("Remember ran after tampered announce header")
	}
	if tr.HasPath(destHash) {
		t.Fatal("path registered after tampered announce header")
	}
	if spy.calls.Load() != 0 {
		t.Fatalf("announce handler calls=%d want 0", spy.calls.Load())
	}
}

// Guarantee: dest hash already known under a different public key is rejected
// and does not replace the stored key, register a path, or notify handlers.
func TestOracleAnnouncePublicKeyMismatchLeavesStateClean(t *testing.T) {
	tr := NewTransport(common.DefaultConfig())
	defer tr.Close()

	iface := &mockInterface{}
	iface.Name = "oracle-mismatch"
	iface.Enabled = true
	iface.Online = true
	if err := tr.RegisterInterface("oracle-mismatch", iface); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}

	spy := &announceAuthSpy{}
	tr.RegisterAnnounceHandler(spy)

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	annRaw, destHash := signedAnnounceRaw(t, tr, id)

	poison, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	if !identity.Remember([]byte("poison"), destHash, poison.GetPublicKey(), nil) {
		t.Fatal("setup Remember should accept")
	}

	tr.HandlePacket(annRaw, iface)
	waitInboundDrain(t, tr, 200*time.Millisecond)

	recalled, err := identity.Recall(destHash)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if !bytes.Equal(recalled.GetPublicKey(), poison.GetPublicKey()) {
		t.Fatal("known destination public key was replaced")
	}
	if tr.HasPath(destHash) {
		t.Fatal("path registered after public key mismatch")
	}
	if spy.calls.Load() != 0 {
		t.Fatalf("announce handler calls=%d want 0", spy.calls.Load())
	}
}
