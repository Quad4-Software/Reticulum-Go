// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build !js

package interfaces

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestParsePeerKeyPin(t *testing.T) {
	if pin, err := parsePeerKeyPin(""); err != nil || pin != nil {
		t.Fatalf("empty: %v %v", pin, err)
	}
	raw := bytes.Repeat([]byte{0x11}, sha256.Size)
	hexStr := hex.EncodeToString(raw)
	pin, err := parsePeerKeyPin(hexStr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pin, raw) {
		t.Fatalf("mismatch")
	}
	colon := "11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11"
	pin2, err := parsePeerKeyPin(colon)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pin2, raw) {
		t.Fatalf("colon form mismatch")
	}
	if _, err := parsePeerKeyPin("deadbeef"); err == nil {
		t.Fatal("expected length error")
	}
	if _, err := parsePeerKeyPin("zz"); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestGenerateAndPin(t *testing.T) {
	cert, err := generateEphemeralQUICCert()
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := leafCertificate(cert)
	if err != nil {
		t.Fatal(err)
	}
	pin := spkiSHA256(leaf)
	if len(pin) != sha256.Size {
		t.Fatalf("pin len %d", len(pin))
	}
	if err := verifyPeerKeyPin([][]byte{leaf.Raw}, pin); err != nil {
		t.Fatal(err)
	}
	bad := bytes.Repeat([]byte{0xff}, sha256.Size)
	if err := verifyPeerKeyPin([][]byte{leaf.Raw}, bad); err == nil {
		t.Fatal("expected pin mismatch")
	}
	hexPin := SPKIPinHex(leaf)
	parsed, err := parsePeerKeyPin(hexPin)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(parsed, pin) {
		t.Fatal("SPKIPinHex round-trip failed")
	}
}

func TestBuildTLSConfigs(t *testing.T) {
	cert, err := generateEphemeralQUICCert()
	if err != nil {
		t.Fatal(err)
	}
	pin := bytes.Repeat([]byte{0x42}, sha256.Size)
	client := buildQUICClientTLS("example.com", pin, cert)
	if client.ServerName != "example.com" {
		t.Fatalf("sni %q", client.ServerName)
	}
	if !client.InsecureSkipVerify {
		t.Fatal("expected InsecureSkipVerify")
	}
	if client.VerifyPeerCertificate == nil {
		t.Fatal("expected pin verifier")
	}
	server := buildQUICServerTLS(cert, pin)
	if server.VerifyPeerCertificate == nil {
		t.Fatal("expected server pin verifier")
	}
	nopin := buildQUICServerTLS(cert, nil)
	if nopin.VerifyPeerCertificate != nil {
		t.Fatal("unexpected verifier")
	}
}

func TestLoadOrGenerateQUICCert(t *testing.T) {
	c1, err := loadOrGenerateQUICCert("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(c1.Certificate) == 0 {
		t.Fatal("empty cert")
	}
	if _, err := loadOrGenerateQUICCert("only.pem", ""); err == nil {
		t.Fatal("expected both-or-neither error")
	}
}
