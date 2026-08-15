// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"bytes"
	"testing"
)

func TestHashFromNameAndIdentityMatchesDestinationHash(t *testing.T) {
	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	want := DestinationHash(id, "rnstransport", "remote", "management")
	got := HashFromNameAndIdentity("rnstransport.remote.management", id.Hash())
	if !bytes.Equal(want, got) {
		t.Fatalf("got %x want %x", got, want)
	}
	got2 := HashFromIdentityHash(id.Hash(), "rnstransport", "remote", "management")
	if !bytes.Equal(want, got2) {
		t.Fatalf("identity hash path: got %x want %x", got2, want)
	}
}

func TestParseDestinationName(t *testing.T) {
	app, aspects, err := ParseDestinationName("app.aspect.extra")
	if err != nil {
		t.Fatalf("ParseDestinationName: %v", err)
	}
	if app != "app" || len(aspects) != 2 || aspects[0] != "aspect" || aspects[1] != "extra" {
		t.Fatalf("got app=%q aspects=%v", app, aspects)
	}
	app, aspects, err = ParseDestinationName("solo")
	if err != nil || app != "solo" || aspects != nil {
		t.Fatalf("solo: app=%q aspects=%v err=%v", app, aspects, err)
	}
	if _, _, err := ParseDestinationName(""); err == nil {
		t.Fatal("empty name should error")
	}
	if _, _, err := ParseDestinationName("   "); err == nil {
		t.Fatal("whitespace-only name should error")
	}
}

func TestHashFromIdentityHashEmptyIdentity(t *testing.T) {
	got := HashFromIdentityHash(nil, "app", "aspect")
	if len(got) != TruncatedHashLength/8 {
		t.Fatalf("len=%d", len(got))
	}
	got2 := DestinationHash(nil, "app", "aspect")
	if !bytes.Equal(got, got2) {
		t.Fatalf("nil identity mismatch")
	}
}
