// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

func TestRgoshAppMismatchDetectsRnsh(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	rnsh := destination.Hash(id, RnshAppName)
	rgosh := destination.Hash(id, RgoshAppName)
	if msg := rgoshAppMismatch(rnsh, id, RgoshAppName); msg == "" {
		t.Fatal("expected rnsh mismatch hint")
	}
	if msg := rgoshAppMismatch(rgosh, id, RnshAppName); msg == "" {
		t.Fatal("expected rgosh mismatch hint")
	}
	if msg := rgoshAppMismatch(rgosh, id, RgoshAppName); msg != "" {
		t.Fatalf("unexpected: %s", msg)
	}
	if msg := rgoshAppMismatch(rnsh, id, RnshAppName); msg != "" {
		t.Fatalf("unexpected: %s", msg)
	}
	compat, app, ok := DetectShellMode(rnsh, id)
	if !ok || !compat || app != RnshAppName {
		t.Fatalf("detect rnsh: compat=%v app=%q ok=%v", compat, app, ok)
	}
	compat, app, ok = DetectShellMode(rgosh, id)
	if !ok || compat || app != RgoshAppName {
		t.Fatalf("detect rgosh: compat=%v app=%q ok=%v", compat, app, ok)
	}
}

func TestParseAllowedIdentityHashForms(t *testing.T) {
	const bare = "00112233445566778899aabbccddeeff"
	for _, text := range []string{bare, "<" + bare + ">"} {
		h, err := parseAllowedIdentityHash(text)
		if err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		if len(h) != 16 {
			t.Fatalf("%s decoded len=%d", text, len(h))
		}
	}
}

func TestParseAllowedIdentityHashRejectsTruncated(t *testing.T) {
	for _, text := range []string{"001122", "<001122...>"} {
		_, err := parseAllowedIdentityHash(text)
		if err == nil {
			t.Fatalf("%s accepted", text)
		}
		if err.Error() != "hash truncated, use full 32 hex chars" {
			t.Fatalf("%s error=%q", text, err)
		}
	}
}
