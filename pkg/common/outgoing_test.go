// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import (
	"errors"
	"testing"
)

func TestInterfaceAllowsOutgoingAndReject(t *testing.T) {
	b := NewBaseInterfacePtr("ro", IFTypeUDP, true)
	b.Online = true
	if !InterfaceAllowsOutgoing(b) {
		t.Fatal("default should allow outgoing")
	}
	b.SetOutgoingAllowed(false)
	if InterfaceAllowsOutgoing(b) {
		t.Fatal("expected blocked")
	}
	if err := RejectReceiveOnly(b); !errors.Is(err, ErrInterfaceReceiveOnly) {
		t.Fatalf("got %v", err)
	}
	if err := b.Send([]byte("x"), ""); !errors.Is(err, ErrInterfaceReceiveOnly) {
		t.Fatalf("Send got %v", err)
	}
	if InterfaceAllowsOutgoing(nil) {
		t.Fatal("nil should not allow")
	}
}
