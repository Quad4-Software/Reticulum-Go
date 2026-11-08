// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"context"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/stamps"
)

func TestNewIdentityWithValue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	id, err := NewIdentityWithValueContext(ctx, 6)
	if err != nil {
		t.Fatalf("NewIdentityWithValueContext: %v", err)
	}
	defer id.Close()
	if v := id.GetValue(); v < 6 {
		t.Fatalf("identity value %v below target 6", v)
	}
	if len(id.GetPublicKey()) != 64 {
		t.Fatal("bad public key")
	}
	// The X25519 pub key is the stamp over the sig pub material.
	sigPub := id.GetPublicKey()[32:]
	pub := id.GetPublicKey()[:32]
	if got := stamps.StampValue(sigPub, pub); got != id.GetValue() {
		t.Fatalf("GetValue %v != %v", id.GetValue(), got)
	}
}

func TestRememberStampedAndRecall(t *testing.T) {
	resetKnownDestinations(t)
	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer id.Close()
	destHash := id.Hash()
	lr := 12.5
	info := &StampInfo{Stamp: []byte("12345678901234"), Value: 9.25, LRCost: &lr}
	if !RememberStamped([]byte("pkt"), destHash, id.GetPublicKey(), []byte("app"), id, info) {
		t.Fatal("RememberStamped failed")
	}
	if v, ok := RecallStampValue(destHash); !ok || v != 9.25 {
		t.Fatalf("RecallStampValue = %v,%v", v, ok)
	}
	if c, ok := RecallLinkCost(destHash); !ok || c != 12.5 {
		t.Fatalf("RecallLinkCost = %v,%v", c, ok)
	}

	other, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, ok := RecallStampValue(other.Hash()); ok {
		t.Fatal("unknown destination must not report a stamp value")
	}
	if _, ok := RecallLinkCost(destHash); !ok {
		t.Fatal("lr cost lost")
	}
	// No stamp recorded: RecallLinkCost false but stamp value still recalls.
	if !Remember([]byte("pkt2"), other.Hash(), other.GetPublicKey(), nil) {
		t.Fatal("Remember failed")
	}
	if v, ok := RecallStampValue(other.Hash()); !ok || v != 0 {
		t.Fatalf("unstamped entry: value=%v ok=%v", v, ok)
	}
	if _, ok := RecallLinkCost(other.Hash()); ok {
		t.Fatal("unstamped entry must not report lr_cost")
	}
}
