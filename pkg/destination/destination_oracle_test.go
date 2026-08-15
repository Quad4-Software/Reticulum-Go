// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package destination

import (
	"bytes"
	"encoding/hex"
	"testing"

	"quad4/reticulum-go/pkg/identity"
)

func TestOracleDestinationHashesMatchPythonRNS(t *testing.T) {
	prv, err := hex.DecodeString("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f")
	if err != nil {
		t.Fatal(err)
	}
	id, err := identity.FromBytes(prv)
	if err != nil {
		t.Fatalf("FromBytes: %v", err)
	}
	got := Hash(id, "oracleapp", "node")
	want, _ := hex.DecodeString("a88a14abeb6f6872b757c14e147e23c2")
	if !bytes.Equal(got, want) {
		t.Fatalf("SINGLE dest hash=%x want %x", got, want)
	}
	pr := Hash(nil, "rnstransport", "path", "request")
	wantPR, _ := hex.DecodeString("6b9f66014d9853faab220fba47d02761")
	if !bytes.Equal(pr, wantPR) {
		t.Fatalf("PLAIN path request dest=%x want %x", pr, wantPR)
	}
}
