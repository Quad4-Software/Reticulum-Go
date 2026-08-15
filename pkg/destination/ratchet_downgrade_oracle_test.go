// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package destination

import (
	"bytes"
	"path/filepath"
	"testing"

	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/identity"
)

// Regression: EnforceRatchets must reject identity-key ciphertext
// (Python Destination.decrypt + Identity.decrypt with enforce_ratchets=True).
func TestOracleEnforceRatchetsRejectsIdentityCiphertext(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := New(id, In|Out, Single, "oracle", &mockTransport{}, "ratchet")
	if err != nil {
		t.Fatal(err)
	}
	if !dest.EnableRatchets(filepath.Join(t.TempDir(), "ratchets")) {
		t.Fatal("EnableRatchets failed")
	}
	if err := dest.RotateRatchets(); err != nil {
		t.Fatalf("RotateRatchets: %v", err)
	}
	dest.EnforceRatchets()

	plain := []byte("BH_DEST_RATCHET_DOWNGRADE")
	ct, err := id.Encrypt(plain, nil)
	if err != nil {
		t.Fatalf("identity Encrypt: %v", err)
	}

	got, err := dest.Decrypt(ct)
	if err == nil && bytes.Equal(got, plain) {
		t.Fatal("EnforceRatchets accepted identity ciphertext (ratchet downgrade)")
	}
}

// Regression: Destination.Decrypt must try destination ratchet private keys
// (Python passes self.ratchets into Identity.decrypt).
func TestOracleDestinationDecryptUsesRatchets(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := New(id, In|Out, Single, "oracle", &mockTransport{}, "ratchet")
	if err != nil {
		t.Fatal(err)
	}
	if !dest.EnableRatchets(filepath.Join(t.TempDir(), "ratchets")) {
		t.Fatal("EnableRatchets failed")
	}
	if err := dest.RotateRatchets(); err != nil {
		t.Fatalf("RotateRatchets: %v", err)
	}
	ratchets := dest.GetRatchets()
	if len(ratchets) == 0 {
		t.Fatal("expected at least one ratchet private key")
	}
	ratchetPub, err := cryptography.PublicKeyFromPrivate(ratchets[0])
	if err != nil {
		t.Fatalf("PublicKeyFromPrivate: %v", err)
	}

	plain := []byte("BH_DEST_RATCHET_RECV")
	ct, err := id.Encrypt(plain, ratchetPub)
	if err != nil {
		t.Fatalf("Encrypt to ratchet: %v", err)
	}

	got, err := dest.Decrypt(ct)
	if err != nil {
		t.Fatalf("Destination.Decrypt ignored destination ratchets: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("plaintext mismatch: got %q want %q", got, plain)
	}
}
