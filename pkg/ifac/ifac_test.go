// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package ifac

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// Golden IFAC vectors were produced by HKDF(length=64,
// derive_from=full_hash(full_hash("testnet")+full_hash("hunter2")),
// salt=IFAC_SALT, context=None) against reference vectors (1.1.5).
const (
	pythonNetname     = "testnet"
	pythonNetkey      = "hunter2"
	pythonGoldenKey   = "a1cd332c7b57176c2ccd49136c10373a7b18ff74ea2f48c05583f676d6a702e61b9f8f980f025582b3a7243b24055caaca3c89ab17644fcbbbc016e9edd77eb2"
	pythonGoldenIDHsh = "942acc417465374727049845d00454d1"
	pythonRaw         = "0001111111111111111111111111111111110041424142414241424142414241424142"
	pythonGoldenIFAC  = "de0ea0eadecd551075343450b40af500"
	pythonGoldenMask  = "e0dbde0ea0eadecd551075343450b40af5009e058813edb05c10b4d37dd52de5df1302adf3fe7c3c1b769d96a5656be60a49b4"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("invalid hex: %v", err)
	}
	return b
}

func TestNewMatchesPythonReference(t *testing.T) {
	id, err := New(16, pythonNetname, pythonNetkey)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if got, want := hex.EncodeToString(id.Key()), pythonGoldenKey; got != want {
		t.Fatalf("ifac key mismatch:\n got=%s\nwant=%s", got, want)
	}
	if got, want := hex.EncodeToString(id.IdentityHash()), pythonGoldenIDHsh; got != want {
		t.Fatalf("ifac identity hash mismatch:\n got=%s\nwant=%s", got, want)
	}
}

func TestSignMatchesPythonReference(t *testing.T) {
	id, err := New(16, pythonNetname, pythonNetkey)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	raw := mustHex(t, pythonRaw)
	sig, err := id.Sign(raw)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	if got, want := hex.EncodeToString(sig), pythonGoldenIFAC; got != want {
		t.Fatalf("ifac signature tail mismatch:\n got=%s\nwant=%s", got, want)
	}
}

func TestMaskMatchesPythonReference(t *testing.T) {
	id, err := New(16, pythonNetname, pythonNetkey)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	raw := mustHex(t, pythonRaw)
	masked, err := id.Mask(raw)
	if err != nil {
		t.Fatalf("Mask failed: %v", err)
	}
	if got, want := hex.EncodeToString(masked), pythonGoldenMask; got != want {
		t.Fatalf("masked packet mismatch:\n got=%s\nwant=%s", got, want)
	}
}

func TestUnmaskRoundTripFromPythonReference(t *testing.T) {
	id, err := New(16, pythonNetname, pythonNetkey)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	masked := mustHex(t, pythonGoldenMask)
	raw, ok, err := id.Unmask(masked)
	if err != nil {
		t.Fatalf("Unmask failed: %v", err)
	}
	if !ok {
		t.Fatalf("Unmask reported invalid IFAC for python golden masked packet")
	}
	if got, want := hex.EncodeToString(raw), pythonRaw; got != want {
		t.Fatalf("unmasked packet mismatch:\n got=%s\nwant=%s", got, want)
	}
}

func TestMaskUnmaskRoundTrip(t *testing.T) {
	id, err := New(16, "alpha", "beta")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	for _, size := range []int{1, 4, 8, 16, 32} {
		id.size = size
		raw := bytes.Repeat([]byte{0x42}, 64)
		raw[0] = 0x01
		raw[1] = 0x00
		masked, err := id.Mask(raw)
		if err != nil {
			t.Fatalf("Mask(size=%d) failed: %v", size, err)
		}
		if masked[0]&IFACFlag != IFACFlag {
			t.Fatalf("Mask(size=%d) did not set IFAC flag", size)
		}
		got, ok, err := id.Unmask(masked)
		if err != nil {
			t.Fatalf("Unmask(size=%d) failed: %v", size, err)
		}
		if !ok {
			t.Fatalf("Unmask(size=%d) rejected freshly masked packet", size)
		}
		if !bytes.Equal(got, raw) {
			t.Fatalf("Unmask(size=%d) mismatch: got=%x want=%x", size, got, raw)
		}
	}
}

func TestUnmaskRejectsCorruptIFAC(t *testing.T) {
	id, err := New(16, "alpha", "beta")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	raw := bytes.Repeat([]byte{0x42}, 64)
	raw[0] = 0x01
	masked, err := id.Mask(raw)
	if err != nil {
		t.Fatalf("Mask failed: %v", err)
	}
	masked[5] ^= 0x01
	_, ok, err := id.Unmask(masked)
	if err != nil {
		t.Fatalf("Unmask returned error on corruption: %v", err)
	}
	if ok {
		t.Fatalf("Unmask accepted a corrupt IFAC")
	}
}

func TestUnmaskNoFlagPassesThrough(t *testing.T) {
	id, err := New(16, "alpha", "beta")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	raw := []byte{0x01, 0x02, 0x03, 0x04}
	got, ok, err := id.Unmask(raw)
	if err != nil {
		t.Fatalf("Unmask failed: %v", err)
	}
	if !ok {
		t.Fatalf("Unmask without IFAC flag must pass through")
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("Unmask without IFAC flag must return the same bytes; got=%x", got)
	}
}

func TestNewRequiresAnyInput(t *testing.T) {
	if _, err := New(16, "", ""); err == nil {
		t.Fatalf("expected New to error when both netname and netkey are empty")
	}
}

func TestNewClampsSize(t *testing.T) {
	id, err := New(0, "alpha", "")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if id.Size() != DefaultSize {
		t.Fatalf("size=0 should clamp to DefaultSize=%d, got %d", DefaultSize, id.Size())
	}
}
