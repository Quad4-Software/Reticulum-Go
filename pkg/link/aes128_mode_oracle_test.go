// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"errors"
	"testing"
)

func oraclePairedLinks(t *testing.T, modeA, modeB byte) (linkA, linkB *Link) {
	t.Helper()
	a := &Link{linkID: bytes.Repeat([]byte{0x0D}, 16)}
	b := &Link{linkID: bytes.Repeat([]byte{0x0D}, 16)}
	if err := a.generateEphemeralKeys(); err != nil {
		t.Fatal(err)
	}
	if err := b.generateEphemeralKeys(); err != nil {
		t.Fatal(err)
	}
	a.peerPub = b.pub
	b.peerPub = a.pub
	a.mode = modeA
	b.mode = modeB
	if err := a.performHandshake(); err != nil {
		t.Fatalf("handshake A: %v", err)
	}
	if err := b.performHandshake(); err != nil {
		t.Fatalf("handshake B: %v", err)
	}
	return a, b
}

// Guarantee: ModeAES128CBC truncates session keys and still roundtrips and rejects MAC tamper.
func TestOracleAES128ModeRoundTrip(t *testing.T) {
	a, b := oraclePairedLinks(t, ModeAES128CBC, ModeAES128CBC)
	plain := []byte("BH_LINK_AES128_ROUNDTRIP")

	ct, err := a.encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	got, err := b.decrypt(ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("roundtrip mismatch: got %q", got)
	}
}

func TestOracleAES128ModeRejectsTamperedMAC(t *testing.T) {
	l := oracleHandshakeLink(t, ModeAES128CBC)
	plain := []byte("BH_LINK_AES128_TAMPER")

	ct, err := l.encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	tampered := append([]byte(nil), ct...)
	tampered[len(tampered)-1] ^= 0x01

	got, err := l.decrypt(tampered)
	if err == nil && bytes.Equal(got, plain) {
		t.Fatal("AES128 decrypt accepted MAC-tampered ciphertext")
	}
	if !errors.Is(err, errHMACVerificationFailed) {
		t.Fatalf("decrypt err=%v want errHMACVerificationFailed", err)
	}
}

// Guarantee: mismatched link modes do not silently decrypt peer ciphertext.
func TestOracleMismatchedModeDoesNotDecrypt(t *testing.T) {
	aes256, aes128 := oraclePairedLinks(t, ModeAES256CBC, ModeAES128CBC)
	plain := []byte("BH_LINK_MODE_MISMATCH")

	ct, err := aes256.encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt AES256: %v", err)
	}
	got, err := aes128.decrypt(ct)
	if err == nil && bytes.Equal(got, plain) {
		t.Fatal("AES128 peer decrypted AES256 ciphertext")
	}
}
