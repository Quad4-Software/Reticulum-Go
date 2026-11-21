package mf

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/internal/leaktest"
)

func TestLeak_MF_PackUnpack(t *testing.T) {
	senderHash, err := hex.DecodeString(testHashHex)
	if err != nil {
		t.Fatal(err)
	}
	base := leaktest.Baseline()
	for range 10_000 {
		msg := &Message{
			SenderHash: senderHash,
			Text:       "Hello, leak test.",
		}
		packed, err := msg.Pack()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Unpack(packed); err != nil {
			t.Fatal(err)
		}
	}
	leaktest.AssertStable(t, base, 12, 5*time.Second)
}
