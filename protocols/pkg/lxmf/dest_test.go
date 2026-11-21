// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

func TestDestHashMatchesDeliveryDestination(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	want, err := DeliveryHash(id)
	if err != nil {
		t.Fatal(err)
	}
	got := DestHash(id.Hash())
	if !bytes.Equal(got, want) {
		t.Fatalf("DestHash=%x DeliveryHash=%x", got, want)
	}
}

func TestDestHashRejectsShort(t *testing.T) {
	if DestHash([]byte{1, 2, 3}) != nil {
		t.Fatal("expected nil")
	}
}
