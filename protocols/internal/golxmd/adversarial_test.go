// SPDX-License-Identifier: LicenseRef-Reticulum
package golxmd

import (
	"strings"
	"testing"
	"testing/quick"
)

func TestAdversarial_DecodeDestHashGarbage(t *testing.T) {
	cases := []string{
		"",
		"zzzz",
		"not-a-hash",
		"gg" + strings.Repeat("a", 30),
		"a3.a5.zz.f4",
	}
	for _, c := range cases {
		if _, err := decodeDestHash(c); err == nil {
			t.Fatalf("decodeDestHash(%q) should fail", c)
		}
	}
}

func TestAdversarial_PrettyHexNeverPanics(t *testing.T) {
	f := func(s string) bool {
		if len(s) > 512 {
			s = s[:512]
		}
		_ = prettyHex(s)
		_ = isValidDestHashHex(s)
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 500}); err != nil {
		t.Fatal(err)
	}
}

func TestAdversarial_InvalidHashNotRegistered(t *testing.T) {
	o := OperatorSummary{
		Version:       "test",
		Home:          "/tmp",
		Delivery:      "deadbeef",
		Propagation:   "alsobad",
		PropagationOn: true,
	}
	if isValidDestHashHex(o.Delivery) {
		t.Fatal("short delivery should be invalid")
	}
	if isValidDestHashHex(o.Propagation) {
		t.Fatal("short propagation should be invalid")
	}
}

func TestAdversarial_NormalizeStripsNoise(t *testing.T) {
	mixed := "A3:A5.23-F4"
	got := normalizeDestHashHex(mixed)
	if strings.Contains(got, ":") || strings.Contains(got, ".") {
		t.Fatalf("normalize left separators: %q", got)
	}
	if got != "a3a523-f4" {
		t.Fatalf("normalize = %q", got)
	}
}
