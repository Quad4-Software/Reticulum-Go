// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package i2p

import (
	"strings"
	"testing"
)

func TestResolveDestinationB64Passthrough(t *testing.T) {
	b64 := strings.Repeat("A", 520)
	got, err := ResolveDestination(b64, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != b64 {
		t.Fatalf("expected passthrough")
	}
}

func TestResolveDestinationLookup(t *testing.T) {
	got, err := ResolveDestination("example.i2p", func(name string) (string, error) {
		if name != "example.i2p" {
			t.Fatalf("lookup name %q", name)
		}
		return "lookup-value", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "lookup-value" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveDestinationB32Passthrough(t *testing.T) {
	lookupCalled := false
	dest := "gdludub7eejnh3hhdtofakibaxmkivm5zjfl2zdxxd7btt6l4rkq.b32.i2p"
	got, err := ResolveDestination(dest, func(string) (string, error) {
		lookupCalled = true
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != dest {
		t.Fatalf("got %q want %q", got, dest)
	}
	if lookupCalled {
		t.Fatal("b32 destinations must not use NAMING LOOKUP")
	}
}

func TestResolveDestinationBareB32(t *testing.T) {
	raw := "gdludub7eejnh3hhdtofakibaxmkivm5zjfl2zdxxd7btt6l4rkq"
	got, err := ResolveDestination(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := raw + ".b32.i2p"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
