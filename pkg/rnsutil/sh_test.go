// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"testing"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
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
}
