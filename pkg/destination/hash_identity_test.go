// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package destination

import (
	"bytes"
	"testing"

	"quad4/reticulum-go/pkg/identity"
)

func TestHashFromNameAndIdentityMatchesHash(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	want := Hash(id, "rnstransport", "remote", "management")
	got := HashFromNameAndIdentity("rnstransport.remote.management", id.Hash())
	if !bytes.Equal(want, got) {
		t.Fatalf("got %x want %x", got, want)
	}
	got2 := HashFromIdentityHash(id.Hash(), "rnstransport", "remote", "management")
	if !bytes.Equal(want, got2) {
		t.Fatalf("identity hash path: got %x want %x", got2, want)
	}
}
