// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"bytes"
	"testing"
)

const fuzzMaxEnvelope = 64 << 10

func FuzzRRC_UnmarshalEnvelope(f *testing.F) {
	sender := bytes.Repeat([]byte{0x9c}, IdentityLength)
	env, err := NewEnvelope(TypeMsg, sender)
	if err != nil {
		f.Fatal(err)
	}
	env.Room = "#lobby"
	env.HasRoom = true
	env.Body = "hi"
	env.HasBody = true
	seed, err := env.Marshal()
	if err != nil {
		f.Fatal(err)
	}
	f.Add([]byte{})
	f.Add([]byte{0xff})
	f.Add(seed)
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > fuzzMaxEnvelope {
			data = data[:fuzzMaxEnvelope]
		}
		got, err := UnmarshalEnvelope(data)
		if err != nil {
			return
		}
		raw, err := got.Marshal()
		if err != nil {
			t.Fatalf("valid decode must remashal: %v", err)
		}
		again, err := UnmarshalEnvelope(raw)
		if err != nil {
			t.Fatalf("remashal decode: %v", err)
		}
		if again.Type != got.Type || again.Version != got.Version {
			t.Fatal("type/version drift")
		}
	})
}

func FuzzRRC_NormalizeSanitize(f *testing.F) {
	f.Add("#Lobby")
	f.Add("  alice\n")
	f.Add("")
	f.Add(string([]byte{0, 1, 2, 'x'}))
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 4096 {
			s = s[:4096]
		}
		r := NormalizeRoom(s)
		if r != NormalizeRoom(r) {
			t.Fatal("NormalizeRoom not idempotent")
		}
		n := SanitizeNick(s)
		if n != SanitizeNick(n) {
			t.Fatal("SanitizeNick not idempotent")
		}
	})
}

func TestMutation_EnvelopeStructural(t *testing.T) {
	sender := bytes.Repeat([]byte{0x44}, IdentityLength)
	env := mustEnvelope(t, TypeMsg, sender)
	env.Room = "#lobby"
	env.HasRoom = true
	env.Body = "payload"
	env.HasBody = true
	env.Nick = "mut"
	env.HasNick = true
	base, err := env.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	mutations := []struct {
		name string
		fn   func([]byte) []byte
	}{
		{"truncate", func(b []byte) []byte {
			if len(b) < 2 {
				return b
			}
			return b[:len(b)/2]
		}},
		{"flip-first", func(b []byte) []byte {
			out := append([]byte(nil), b...)
			if len(out) > 0 {
				out[0] ^= 0xff
			}
			return out
		}},
		{"append-noise", func(b []byte) []byte {
			return append(append([]byte(nil), b...), 0x00, 0xff, 0xaa)
		}},
		{"zero-mid", func(b []byte) []byte {
			out := append([]byte(nil), b...)
			if len(out) > 10 {
				out[len(out)/2] = 0
			}
			return out
		}},
	}

	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			mut := m.fn(base)
			got, err := UnmarshalEnvelope(mut)
			if err != nil {
				return
			}
			// If it still parses, remashaling must succeed (no panic / partial state).
			if _, err := got.Marshal(); err != nil {
				t.Fatalf("parsed mutation must remashal: %v", err)
			}
		})
	}
}

func TestMutation_WrongFixedLengths(t *testing.T) {
	m := map[uint64]any{
		KeyVersion:   ProtocolVersion,
		KeyType:      TypePing,
		KeyMsgID:     []byte{1, 2, 3},
		KeyTimestamp: uint64(1),
		KeySender:    bytes.Repeat([]byte{1}, IdentityLength),
	}
	raw, err := mustMarshalMap(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UnmarshalEnvelope(raw); err == nil {
		t.Fatal("short msgid should fail")
	}

	m[KeyMsgID] = bytes.Repeat([]byte{1}, MessageIDLength)
	m[KeySender] = []byte{1, 2}
	raw, err = mustMarshalMap(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UnmarshalEnvelope(raw); err == nil {
		t.Fatal("short sender should fail")
	}
}

func FuzzRRC_ValidateResourceEnvelope(f *testing.F) {
	f.Add([]byte{1, 2, 3}, uint8(0), uint8(1))
	f.Fuzz(func(t *testing.T, id []byte, kindTag uint8, size uint8) {
		if len(id) > 32 {
			id = id[:32]
		}
		kind := ResourceKindBlob
		switch kindTag % 4 {
		case 1:
			kind = ResourceKindNotice
		case 2:
			kind = ResourceKindMOTD
		case 3:
			kind = ""
		}
		body := map[uint64]any{
			ResourceKeyID:   append([]byte(nil), id...),
			ResourceKeyKind: kind,
			ResourceKeySize: uint64(size),
		}
		parsed, reason := ValidateResourceEnvelopeBody(body)
		if reason == "" {
			if parsed == nil || !parsed.HasID || !parsed.HasKind || !parsed.HasSize {
				t.Fatal("valid body missing fields")
			}
			if parsed.ToMap() == nil {
				t.Fatal("valid body map")
			}
		}
	})
}
