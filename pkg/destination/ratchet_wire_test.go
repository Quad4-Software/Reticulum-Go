// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package destination

import (
	"bytes"
	"testing"

	"quad4/reticulum-go/pkg/announce"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
)

func TestAnnounceOmitsRatchetUnlessEnabled(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	ifaces := map[string]common.NetworkInterface{
		"if-a": newRecordingInterface("if-a"),
	}
	tr := &mockTransport{config: &common.ReticulumConfig{}, interfaces: ifaces}
	dest, err := New(id, In, Single, "ratchetapp", tr, "aspect")
	if err != nil {
		t.Fatal(err)
	}
	if err := dest.Announce(false, nil, nil); err != nil {
		t.Fatal(err)
	}
	pkt := ifaces["if-a"].(*recordingInterface).Sent()[0]
	if (pkt[0] & announce.HeaderContextFlagMask) != 0 {
		t.Fatalf("context flag set without EnableRatchets, flags=%08b", pkt[0])
	}
}

func TestAnnounceIncludesDestinationRatchet(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	ifaces := map[string]common.NetworkInterface{
		"if-a": newRecordingInterface("if-a"),
	}
	tr := &mockTransport{config: &common.ReticulumConfig{}, interfaces: ifaces}
	dest, err := New(id, In, Single, "ratchetapp", tr, "aspect")
	if err != nil {
		t.Fatal(err)
	}
	if !dest.EnableRatchetsInMemory() {
		t.Fatal("EnableRatchetsInMemory")
	}
	if err := dest.Announce(false, nil, nil); err != nil {
		t.Fatal(err)
	}
	pkt := ifaces["if-a"].(*recordingInterface).Sent()[0]
	if (pkt[0] & announce.HeaderContextFlagMask) == 0 {
		t.Fatal("expected context flag set when ratchets are enabled")
	}
	pub := dest.CurrentRatchetPublic()
	if len(pub) != 32 {
		t.Fatalf("current ratchet public len = %d", len(pub))
	}
	off := announce.HeaderType1Offset + announce.AnnounceRatchetOffset
	if len(pkt) < off+32 {
		t.Fatalf("announce too short for ratchet: %d", len(pkt))
	}
	if !bytes.Equal(pkt[off:off+32], pub) {
		t.Fatalf("wire ratchet = %x, want %x", pkt[off:off+32], pub)
	}
	got := identity.GetRatchet(dest.GetHash())
	if !bytes.Equal(got, pub) {
		t.Fatalf("remembered ratchet = %x, want %x", got, pub)
	}
}

func TestEncryptUsesAnnouncedRatchet(t *testing.T) {
	alice, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	ifaces := map[string]common.NetworkInterface{
		"if-a": newRecordingInterface("if-a"),
	}
	tr := &mockTransport{config: &common.ReticulumConfig{}, interfaces: ifaces}
	aliceDest, err := New(alice, In, Single, "ratchetapp", tr, "aspect")
	if err != nil {
		t.Fatal(err)
	}
	if !aliceDest.EnableRatchetsInMemory() {
		t.Fatal("EnableRatchetsInMemory")
	}
	if err := aliceDest.Announce(false, nil, nil); err != nil {
		t.Fatal(err)
	}

	bob, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	outDest, err := FromHash(aliceDest.GetHash(), alice, Single, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = bob

	plain := []byte("BH_RATCHET_WIRE")
	ct, err := outDest.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if rid := outDest.LatestRatchetID(); len(rid) == 0 {
		t.Fatal("expected latest ratchet id after encrypt")
	}
	got, err := aliceDest.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt with dest ratchets: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("got %q want %q", got, plain)
	}

	identityCt, err := alice.Encrypt(plain, nil)
	if err != nil {
		t.Fatal(err)
	}
	aliceDest.EnforceRatchets()
	if _, err := aliceDest.Decrypt(identityCt); err == nil {
		t.Fatal("enforce should reject identity-key ciphertext")
	}
}

func TestHandleAnnounceRemembersPeerRatchet(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	ifaces := map[string]common.NetworkInterface{
		"if-a": newRecordingInterface("if-a"),
	}
	tr := &mockTransport{config: &common.ReticulumConfig{}, interfaces: ifaces}
	dest, err := New(id, In, Single, "ratchetapp", tr, "aspect")
	if err != nil {
		t.Fatal(err)
	}
	if !dest.EnableRatchetsInMemory() {
		t.Fatal("EnableRatchetsInMemory")
	}
	if err := dest.Announce(false, nil, nil); err != nil {
		t.Fatal(err)
	}
	pkt := ifaces["if-a"].(*recordingInterface).Sent()[0]

	peer, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	ann, err := announce.New(peer, make([]byte, 16), "other", nil, false, &common.ReticulumConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if err := ann.HandleAnnounce(pkt); err != nil {
		t.Fatalf("HandleAnnounce: %v", err)
	}
	got := identity.GetRatchet(dest.GetHash())
	want := dest.CurrentRatchetPublic()
	if !bytes.Equal(got, want) {
		t.Fatalf("peer remembered %x, want %x", got, want)
	}
}
