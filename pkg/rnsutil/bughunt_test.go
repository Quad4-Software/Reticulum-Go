// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"bytes"
	"testing"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/identity"
)

// TestBughuntRSGForgedSignerHash ensures meta.signer cannot claim a foreign
// identity hash while the envelope is signed by a different pubkey.
func TestBughuntRSGForgedSignerHash(t *testing.T) {
	attacker, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	victim, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("bughunt-forged-signer")
	rsg, err := CreateRSG(attacker, msg, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	sig := rsg[:SignatureSize]
	var envMap map[string]any
	if err := msgpack.Unmarshal(rsg[SignatureSize:], &envMap); err != nil {
		t.Fatal(err)
	}
	meta := envMap["meta"].(map[string]any)
	meta["signer"] = append([]byte(nil), victim.Hash()...)
	patched, err := msgpack.Marshal(envMap)
	if err != nil {
		t.Fatal(err)
	}
	// Re-sign with attacker key so the signature still verifies under pubkey.
	newSig, err := attacker.Sign(patched)
	if err != nil {
		t.Fatal(err)
	}
	forged := append(append([]byte(nil), newSig...), patched...)
	_ = sig

	res, err := ValidateRSG(forged, msg, nil)
	if err == nil && res.Valid {
		t.Fatalf("forged meta.signer accepted (claimed %x, real %x)",
			victim.Hash(), attacker.Hash())
	}
	if bytes.Equal(victim.Hash(), attacker.Hash()) {
		t.Fatal("test identities collided")
	}
}
