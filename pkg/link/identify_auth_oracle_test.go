// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"sync/atomic"
	"testing"

	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/identity"
)

// Guarantee: invalid LINKIDENTIFY signature never sets remoteIdentity or fires identified callback.
func TestOracleBadIdentifyLeavesPeerUnset(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	if initLink.GetStatus() != StatusActive {
		t.Fatalf("initiator status=%d want Active", initLink.GetStatus())
	}

	peerID, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	pubKey := peerID.GetPublicKey()
	signData := append(initLink.linkID, pubKey...)
	sig, err := peerID.Sign(signData)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	sig[0] ^= 0x01

	identData := append(append([]byte(nil), pubKey...), sig...)
	var callbackCalls atomic.Int32
	respLink.SetRemoteIdentifiedCallback(func(_ *Link, _ *identity.Identity) {
		callbackCalls.Add(1)
	})

	if err := respLink.HandleIdentification(identData); err == nil {
		t.Fatal("HandleIdentification accepted forged signature")
	}
	if respLink.remoteIdentity != nil {
		t.Fatal("remoteIdentity set after bad LINKIDENTIFY")
	}
	if callbackCalls.Load() != 0 {
		t.Fatalf("identified callback calls=%d want 0", callbackCalls.Load())
	}
}

// Guarantee: truncated LINKIDENTIFY never sets remoteIdentity.
func TestOracleShortIdentifyLeavesPeerUnset(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	_, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	var callbackCalls atomic.Int32
	respLink.SetRemoteIdentifiedCallback(func(_ *Link, _ *identity.Identity) {
		callbackCalls.Add(1)
	})

	short := make([]byte, identity.KeySize/8+cryptography.Ed25519SignatureSize-1)
	if err := respLink.HandleIdentification(short); err == nil {
		t.Fatal("HandleIdentification accepted short payload")
	}
	if respLink.remoteIdentity != nil {
		t.Fatal("remoteIdentity set after short LINKIDENTIFY")
	}
	if callbackCalls.Load() != 0 {
		t.Fatalf("identified callback calls=%d want 0", callbackCalls.Load())
	}
}
