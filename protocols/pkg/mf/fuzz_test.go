package mf

import (
	"bytes"
	"testing"
)

const fuzzMaxMFBytes = 128 << 10

func clipMF(b []byte, max int) []byte {
	if len(b) <= max {
		return b
	}
	return b[:max]
}

func FuzzMF_Unpack(f *testing.F) {
	f.Add([]byte{})
	f.Add(make([]byte, SenderHashLength))
	f.Add(append(make([]byte, SenderHashLength), []byte("hello")...))
	f.Fuzz(func(t *testing.T, data []byte) {
		data = clipMF(data, fuzzMaxMFBytes)
		_, _ = Unpack(data)
	})
}

func FuzzMF_NewMessage(f *testing.F) {
	h := make([]byte, SenderHashLength)
	f.Add(h, "")
	f.Add(h, "ok")
	f.Fuzz(func(t *testing.T, hash []byte, text string) {
		th := make([]byte, SenderHashLength)
		copy(th, hash)
		if len(text) > MaxMessageSize+64 {
			text = text[:MaxMessageSize+64]
		}
		msg, err := NewMessage(th, text)
		if err != nil {
			return
		}
		packed, err := msg.Pack()
		if err != nil {
			t.Fatal(err)
		}
		back, err := Unpack(packed)
		if err != nil {
			t.Fatal(err)
		}
		if !msg.Equal(back) {
			t.Fatal("mf message round-trip mismatch")
		}
	})
}

func FuzzMF_SenderHashFromHex(f *testing.F) {
	f.Add("")
	f.Add("nothex")
	f.Add("0123456789abcdef0123456789abcdef")
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 4096 {
			s = s[:4096]
		}
		h, err := SenderHashFromHex(s)
		if err != nil {
			return
		}
		if err := ValidateSenderHash(h); err != nil {
			t.Fatal(err)
		}
	})
}

func FuzzMF_PeerValidate(f *testing.F) {
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, hash []byte) {
		th := make([]byte, SenderHashLength)
		copy(th, hash)
		p := &Peer{Hash: th, AppData: "fuzz"}
		_ = p.Validate()
		_ = p.FormatHash()
		_ = p.String()
	})
}

func FuzzMF_InvariantPackLen(f *testing.F) {
	f.Add(bytes.Repeat([]byte{0x11}, SenderHashLength), "x")
	f.Fuzz(func(t *testing.T, rawHash []byte, text string) {
		h := make([]byte, SenderHashLength)
		copy(h, rawHash)
		if len(text) > MaxMessageSize {
			text = text[:MaxMessageSize]
		}
		msg, err := NewMessage(h, text)
		if err != nil {
			return
		}
		b, err := msg.Pack()
		if err != nil {
			t.Fatal(err)
		}
		if len(b) != msg.Len() {
			t.Fatalf("Len mismatch: %d vs %d", len(b), msg.Len())
		}
	})
}
