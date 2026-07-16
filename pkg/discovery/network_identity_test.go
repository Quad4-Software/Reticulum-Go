// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"testing"

	"quad4/reticulum-go/pkg/identity"
)

func TestEncryptedDiscoveryRoundTrip(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	info := Info{Type: "UDPInterface", Name: "hub", TransportID: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}}
	app, err := BuildEncryptedAppData(info, 0, 3, id)
	if err != nil {
		t.Fatal(err)
	}
	if app[0]&FlagEncrypted == 0 {
		t.Fatal("expected FlagEncrypted")
	}
	if _, err := ValidateAndDecode(app, 0, 3); err == nil {
		t.Fatal("expected error without network identity")
	}
	got, err := ValidateAndDecodeWithIdentity(app, 0, 3, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Info.Type != info.Type || got.Info.Name != info.Name {
		t.Fatalf("decoded info mismatch: %+v", got.Info)
	}
}

func TestEncryptedDiscoveryWrongIdentity(t *testing.T) {
	a, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	b, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	info := Info{Type: "TCPClientInterface", Name: "peer", TransportID: make([]byte, 16)}
	app, err := BuildEncryptedAppData(info, 0, 3, a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateAndDecodeWithIdentity(app, 0, 3, b); err == nil {
		t.Fatal("expected decrypt failure with wrong identity")
	}
}
