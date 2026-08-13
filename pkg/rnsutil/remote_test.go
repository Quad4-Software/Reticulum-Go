// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"bytes"
	"testing"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/transport"
)

func TestRemoteManagementDestHashMatchesDestinationHash(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	want := destination.Hash(id, "rnstransport", "remote", "management")
	got := RemoteManagementDestHash(id.Hash())
	if !bytes.Equal(want, got) {
		t.Fatalf("got %x want %x", got, want)
	}
}

func TestPathTableFromResponse(t *testing.T) {
	h := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	via := []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	raw := []any{map[string]any{
		"hash":      h,
		"via":       via,
		"hops":      2,
		"expires":   1.0,
		"timestamp": 0.0,
		"interface": "UDP",
	}}
	table := PathTableFromResponse(raw)
	if len(table) != 1 || table[0].Hops != 2 || table[0].Interface != "UDP" {
		t.Fatalf("%+v", table)
	}
	if !bytes.Equal(table[0].Hash, h) {
		t.Fatalf("hash %x", table[0].Hash)
	}
}

func TestRateTableFromResponse(t *testing.T) {
	h := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	raw := []any{map[string]any{
		"hash":            h,
		"last":            10.0,
		"rate_violations": 1,
		"blocked_until":   0.0,
		"timestamps":      []any{1.0, 2.0},
	}}
	table := RateTableFromResponse(raw)
	if len(table) != 1 || table[0].RateViolations != 1 || len(table[0].Timestamps) != 2 {
		t.Fatalf("%+v", table)
	}
}

func TestPathTableFromTypedSlice(t *testing.T) {
	in := []transport.PathTableEntry{{Hash: []byte{1}, Hops: 3}}
	out := PathTableFromResponse(in)
	if len(out) != 1 || out[0].Hops != 3 {
		t.Fatalf("%+v", out)
	}
}
