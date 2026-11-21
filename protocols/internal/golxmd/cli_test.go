// SPDX-License-Identifier: LicenseRef-Reticulum
package golxmd

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf"
)

func TestDecodeDestHash_HexFormats(t *testing.T) {
	raw := "a3a523f48208a950b026ccc0d8b702ac"
	want, err := hex.DecodeString(raw)
	if err != nil {
		t.Fatal(err)
	}
	cases := []string{
		raw,
		strings.ToUpper(raw),
		"a3.a5.23.f4.82.08.a9.50.b0.26.cc.c0.d8.b7.02.ac",
		"  " + raw + "  ",
	}
	for _, c := range cases {
		got, err := decodeDestHash(c)
		if err != nil {
			t.Fatalf("decodeDestHash(%q): %v", c, err)
		}
		if hex.EncodeToString(got) != hex.EncodeToString(want) {
			t.Fatalf("decodeDestHash(%q) = %x", c, got)
		}
	}
}

func TestPrettyHex_DottedForm(t *testing.T) {
	raw := "bd83d875f9b033ed9e89d7f3c685de6e"
	if prettyHex(raw) != raw {
		t.Fatalf("prettyHex plain = %q", prettyHex(raw))
	}
	dotted := "bd.83.d8.75.f9.b0.33.ed.9e.89.d7.f3.c6.85.de.6e"
	if prettyHex(dotted) != raw {
		t.Fatalf("prettyHex dotted = %q want %q", prettyHex(dotted), raw)
	}
	if !isValidDestHashHex(dotted) {
		t.Fatal("dotted hash should validate")
	}
}

func TestCountDeliveryMessages_SkipsDotfiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "msg.lxm"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".golxmd-write-test"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := countDeliveryMessages(dir); got != 1 {
		t.Fatalf("count=%d want 1", got)
	}
}

func TestStatusInterval_EnvOverride(t *testing.T) {
	t.Setenv("GOLXMD_STATUS_INTERVAL", "250ms")
	if got := statusInterval(true); got != 250*time.Millisecond {
		t.Fatalf("interval=%v", got)
	}
	t.Setenv("GOLXMD_STATUS_INTERVAL", "nope")
	if got := statusInterval(true); got != liveStatusInterval {
		t.Fatalf("fallback interval=%v", got)
	}
}

func TestNormalizeDestHashHex_Idempotent(t *testing.T) {
	f := func(s string) bool {
		if len(s) > 128 {
			s = s[:128]
		}
		a := normalizeDestHashHex(s)
		return a == normalizeDestHashHex(a)
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatal(err)
	}
}

func TestPrettyHex_ValidHashRoundTrip(t *testing.T) {
	f := func(b0, b1, b2, b3, b4, b5, b6, b7, b8, b9, b10, b11, b12, b13, b14, b15 byte) bool {
		raw := []byte{b0, b1, b2, b3, b4, b5, b6, b7, b8, b9, b10, b11, b12, b13, b14, b15}
		plain := hex.EncodeToString(raw)
		dotted := strings.Join(chunkPairs(plain), ".")
		if prettyHex(plain) != plain {
			return false
		}
		if prettyHex(dotted) != plain {
			return false
		}
		if !isValidDestHashHex(dotted) {
			return false
		}
		decoded, err := decodeDestHash(dotted)
		if err != nil {
			return false
		}
		return hex.EncodeToString(decoded) == plain
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeDestHash_RejectsWrongLength(t *testing.T) {
	f := func(s string) bool {
		s = normalizeDestHashHex(s)
		if len(s) == 2*lxmf.DestinationLength {
			return true
		}
		_, err := decodeDestHash(s)
		return err != nil
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
}

func chunkPairs(hexStr string) []string {
	out := make([]string, 0, len(hexStr)/2)
	for i := 0; i+1 < len(hexStr); i += 2 {
		out = append(out, hexStr[i:i+2])
	}
	return out
}
