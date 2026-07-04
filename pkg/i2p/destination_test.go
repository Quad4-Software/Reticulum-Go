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
