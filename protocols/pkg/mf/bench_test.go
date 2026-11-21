package mf

import (
	"encoding/hex"
	"testing"
)

func BenchmarkMF_Pack(b *testing.B) {
	senderHash, err := hex.DecodeString(testHashHex)
	if err != nil {
		b.Fatal(err)
	}
	msg := &Message{
		SenderHash: senderHash,
		Text:       "Hello, Reticulum benchmark.",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := msg.Pack(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMF_Unpack(b *testing.B) {
	senderHash, err := hex.DecodeString(testHashHex)
	if err != nil {
		b.Fatal(err)
	}
	msg := &Message{
		SenderHash: senderHash,
		Text:       "Hello, Reticulum benchmark.",
	}
	packed, err := msg.Pack()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Unpack(packed); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMF_NewMessageValidate(b *testing.B) {
	senderHash, err := hex.DecodeString(testHashHex)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := NewMessage(senderHash, "short text"); err != nil {
			b.Fatal(err)
		}
	}
}
