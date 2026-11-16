// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

func BenchmarkLXMF_Pack(b *testing.B) {
	src := mustNewIdentity(b)
	dst := mustNewIdentity(b)
	dh, sh := dst.Hash(), src.Hash()
	msg, err := NewMessage(dh, sh, []byte("subject"), []byte("Hello, bench."), nil)
	if err != nil {
		b.Fatal(err)
	}
	msg.Timestamp = 1_900_000_000.0
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := msg.Pack(src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLXMF_Unpack(b *testing.B) {
	src := mustNewIdentity(b)
	dst := mustNewIdentity(b)
	dh, sh := dst.Hash(), src.Hash()
	msg, err := NewMessage(dh, sh, []byte("subject"), []byte("Hello, bench."), nil)
	if err != nil {
		b.Fatal(err)
	}
	msg.Timestamp = 1_900_000_100.0
	packed, err := msg.Pack(src)
	if err != nil {
		b.Fatal(err)
	}
	identity.Remember(nil, sh, src.GetPublicKey(), nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Unpack(packed, RecallSource); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLXMF_StampWorkblock(b *testing.B) {
	material := make([]byte, 32)
	for i := range material {
		material[i] = byte(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := StampWorkblock(material, 6); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLXMF_StampValid(b *testing.B) {
	material := []byte("bench-stamp-material")
	wb, err := StampWorkblock(material, 5)
	if err != nil {
		b.Fatal(err)
	}
	stamp := make([]byte, StampSize)
	stamp[0] = 0x01
	stamp[1] = 0x02
	const cost = 10
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = StampValid(stamp, cost, wb)
	}
}
