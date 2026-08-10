// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"crypto/aes"
	"errors"
	"testing"

	"quad4/reticulum-go/pkg/cryptography"
)

func oracleHandshakeLink(t *testing.T, mode byte) *Link {
	t.Helper()
	l := &Link{linkID: bytes.Repeat([]byte{0x0C}, 16)}
	if err := l.generateEphemeralKeys(); err != nil {
		t.Fatal(err)
	}
	l.peerPub = l.pub
	l.mode = mode
	if err := l.performHandshake(); err != nil {
		t.Fatalf("performHandshake: %v", err)
	}
	return l
}

// Guarantee: established link decrypt rejects tampered session ciphertext with HMAC failure.
func TestOracleSessionDecryptRejectsTamperedMAC(t *testing.T) {
	l := oracleHandshakeLink(t, ModeAES256CBC)
	plain := []byte("BH_LINK_SESSION_HMAC")

	ct, err := l.encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if len(ct) < cryptography.SHA256Size {
		t.Fatalf("ciphertext too short: %d", len(ct))
	}

	tampered := append([]byte(nil), ct...)
	tampered[len(tampered)-1] ^= 0x01

	got, err := l.decrypt(tampered)
	if err == nil && bytes.Equal(got, plain) {
		t.Fatal("decrypt accepted MAC-tampered session ciphertext")
	}
	if !errors.Is(err, errHMACVerificationFailed) {
		t.Fatalf("decrypt err=%v want errHMACVerificationFailed", err)
	}
}

func TestOracleSessionDecryptRejectsTamperedIV(t *testing.T) {
	l := oracleHandshakeLink(t, ModeAES256CBC)
	plain := []byte("BH_LINK_SESSION_IV")

	ct, err := l.encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if len(ct) < aes.BlockSize+cryptography.SHA256Size {
		t.Fatalf("ciphertext too short: %d", len(ct))
	}

	tampered := append([]byte(nil), ct...)
	tampered[0] ^= 0x01

	got, err := l.decrypt(tampered)
	if err == nil && bytes.Equal(got, plain) {
		t.Fatal("decrypt accepted IV-tampered session ciphertext")
	}
	if !errors.Is(err, errHMACVerificationFailed) {
		t.Fatalf("decrypt err=%v want errHMACVerificationFailed", err)
	}
}

func TestOracleSessionDecryptRejectsShortFrame(t *testing.T) {
	l := oracleHandshakeLink(t, ModeAES256CBC)

	_, err := l.decrypt([]byte{0x01, 0x02, 0x03})
	if err == nil {
		t.Fatal("decrypt accepted short frame")
	}
	if !errors.Is(err, errHMACVerificationFailed) && err.Error() != "data too short" {
		t.Fatalf("decrypt err=%v want HMAC failure or data too short", err)
	}
}
