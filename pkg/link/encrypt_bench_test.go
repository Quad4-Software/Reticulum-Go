// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"crypto/aes"
	"crypto/sha256"
	"testing"

	"quad4/reticulum-go/pkg/packet"
)

func handshakeLink(t testing.TB) *Link {
	t.Helper()
	l := &Link{linkID: bytes.Repeat([]byte{0x0C}, 16)}
	if err := l.generateEphemeralKeys(); err != nil {
		t.Fatal(err)
	}
	l.peerPub = l.pub
	l.mode = ModeAES256CBC
	if err := l.performHandshake(); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestEncryptLockedIntoRoundTrip(t *testing.T) {
	l := handshakeLink(t)
	plain := []byte("encrypt-into-dst")
	n := encryptedPayloadLen(len(plain))
	dst := make([]byte, n)
	l.mutex.Lock()
	ct, err := l.encryptLockedInto(plain, dst)
	l.mutex.Unlock()
	if err != nil {
		t.Fatalf("encryptLockedInto: %v", err)
	}
	if len(ct) != n || &ct[0] != &dst[0] {
		t.Fatalf("expected in-place encrypt len=%d got=%d alias=%v", n, len(ct), len(ct) > 0 && &ct[0] == &dst[0])
	}
	got, err := l.decrypt(ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("roundtrip mismatch got=%q", got)
	}
}

func TestSealEncryptedHT1MatchesEncryptThenPack(t *testing.T) {
	l := handshakeLink(t)
	plain := []byte("seal-ht1-parity")

	l.mutex.Lock()
	legacy, err := l.encryptLocked(plain)
	l.mutex.Unlock()
	if err != nil {
		t.Fatalf("encryptLocked: %v", err)
	}
	viaPack := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		Context:         packet.ContextNone,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
		Data:            legacy,
	}
	if err := viaPack.Pack(); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	sealed := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		Context:         packet.ContextNone,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
	}
	l.mutex.Lock()
	if err := l.sealEncryptedHT1Locked(sealed, plain); err != nil {
		l.mutex.Unlock()
		t.Fatalf("sealEncryptedHT1Locked: %v", err)
	}
	l.mutex.Unlock()

	if len(sealed.Raw) != len(viaPack.Raw) {
		t.Fatalf("len sealed=%d pack=%d", len(sealed.Raw), len(viaPack.Raw))
	}
	if sealed.Raw[0] != viaPack.Raw[0] || sealed.Raw[1] != viaPack.Raw[1] {
		t.Fatalf("header mismatch sealed=%x pack=%x", sealed.Raw[:2], viaPack.Raw[:2])
	}
	if !bytes.Equal(sealed.Raw[2:packet.HeaderType1Overhead], viaPack.Raw[2:packet.HeaderType1Overhead]) {
		t.Fatal("dest/context mismatch")
	}
	got, err := l.decrypt(sealed.Data)
	if err != nil {
		t.Fatalf("decrypt sealed: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("sealed plaintext mismatch")
	}
}

func TestEncryptLockedAllocBudget(t *testing.T) {
	l := handshakeLink(t)
	plain := bytes.Repeat([]byte{0x42}, 64)
	n := encryptedPayloadLen(len(plain))
	dst := make([]byte, n)

	allocs := testing.AllocsPerRun(50, func() {
		l.mutex.Lock()
		_, err := l.encryptLockedInto(plain, dst)
		l.mutex.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	})
	if allocs > 1 {
		t.Fatalf("encryptLockedInto allocs=%.1f want <= 1", allocs)
	}

	allocs = testing.AllocsPerRun(50, func() {
		l.mutex.Lock()
		_, err := l.encryptLocked(plain)
		l.mutex.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	})
	if allocs > 3 {
		t.Fatalf("encryptLocked allocs=%.1f want <= 3", allocs)
	}
}

func TestEncryptedPayloadLenMatchesPKCS7HMAC(t *testing.T) {
	for _, n := range []int{0, 1, 15, 16, 17, 64, 200} {
		padding := aes.BlockSize - n%aes.BlockSize
		want := aes.BlockSize + n + padding + sha256.Size
		if got := encryptedPayloadLen(n); got != want {
			t.Fatalf("n=%d got=%d want=%d", n, got, want)
		}
	}
}

func BenchmarkEncryptLocked(b *testing.B) {
	l := handshakeLink(b)
	plain := bytes.Repeat([]byte{0x11}, 128)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.mutex.Lock()
		_, err := l.encryptLocked(plain)
		l.mutex.Unlock()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncryptLockedInto(b *testing.B) {
	l := handshakeLink(b)
	plain := bytes.Repeat([]byte{0x11}, 128)
	dst := make([]byte, encryptedPayloadLen(len(plain)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.mutex.Lock()
		_, err := l.encryptLockedInto(plain, dst)
		l.mutex.Unlock()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSealEncryptedHT1(b *testing.B) {
	l := handshakeLink(b)
	plain := bytes.Repeat([]byte{0x11}, 128)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := &packet.Packet{
			HeaderType:      packet.HeaderType1,
			PacketType:      packet.PacketTypeData,
			Context:         packet.ContextNone,
			DestinationType: DestTypeLink,
			DestinationHash: l.linkID,
		}
		l.mutex.Lock()
		err := l.sealEncryptedHT1Locked(p, plain)
		l.mutex.Unlock()
		if err != nil {
			b.Fatal(err)
		}
	}
}
