// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"testing"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
)

func BenchmarkDestinationHash(b *testing.B) {
	id, err := identity.NewIdentity()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = destination.Hash(id, "exampleapp", "aspect")
	}
}

func BenchmarkPrettyHex(b *testing.B) {
	h := make([]byte, 16)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = PrettyHex(h)
	}
}
