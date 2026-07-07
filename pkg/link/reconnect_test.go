// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import "testing"

func TestGlobalPaused(t *testing.T) {
	SetGlobalPaused(true)
	if !GlobalPaused() {
		t.Fatal("expected paused")
	}
	SetGlobalPaused(false)
	if GlobalPaused() {
		t.Fatal("expected unpaused")
	}
}

func TestDestinationHashNil(t *testing.T) {
	var l *Link
	if l.DestinationHash() != nil {
		t.Fatal("nil link should return nil hash")
	}
}
