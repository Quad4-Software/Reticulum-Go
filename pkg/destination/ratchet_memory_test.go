// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package destination

import (
	"testing"

	"quad4/reticulum-go/pkg/identity"
)

func TestEnableRatchetsInMemory(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := New(id, In, Single, "test", &mockTransport{}, "aspect")
	if err != nil {
		t.Fatal(err)
	}
	if !dest.EnableRatchetsInMemory() {
		t.Fatal("EnableRatchetsInMemory failed")
	}
	if err := dest.RotateRatchets(); err != nil {
		t.Fatalf("RotateRatchets in memory: %v", err)
	}
	dest.mutex.RLock()
	n := len(dest.ratchets)
	path := dest.ratchetPath
	dest.mutex.RUnlock()
	if n < 1 {
		t.Fatal("expected at least one ratchet after rotation")
	}
	if path != "" {
		t.Fatalf("ratchet path should be empty, got %q", path)
	}
}
