// SPDX-License-Identifier: LicenseRef-Reticulum

package lxmf

import (
	"context"
	"crypto/sha256"
	"testing"
)

func BenchmarkMessageStoreAdd(b *testing.B) {
	dir := b.TempDir()
	ms, err := NewMessageStore(dir, 500, nil)
	if err != nil {
		b.Fatal(err)
	}
	dest := make([]byte, DestinationLength)
	lxm := append(append([]byte(nil), dest...), []byte("bench-payload")...)
	stamp := make([]byte, StampSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lxm[0] = byte(i)
		if _, err := ms.Add(lxm, stamp, 10); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidatePNStamps(b *testing.B) {
	dest := make([]byte, DestinationLength)
	lxm := append(append([]byte(nil), dest...), []byte("bench")...)
	tid := sha256.Sum256(lxm)
	stamp, _, err := GenerateStamp(context.Background(), tid[:], 10, WorkblockExpandRoundsPN)
	if err != nil {
		b.Fatal(err)
	}
	transient := append(append([]byte(nil), lxm...), stamp...)
	msgs := [][]byte{transient, transient, transient}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidatePNStamps(msgs, 10)
	}
}

func BenchmarkEncodePNAnnounceAppData(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := EncodePNAnnounceAppData(1700000000, true, 256, 10240, 16, 3, 18, "bench-node")
		if err != nil {
			b.Fatal(err)
		}
	}
}
