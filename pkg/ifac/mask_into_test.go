// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package ifac

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestMaskIntoMatchesPythonReference(t *testing.T) {
	id, err := New(16, pythonNetname, pythonNetkey)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	raw := mustHex(t, pythonRaw)
	dst := make([]byte, 0, 128)
	masked, err := id.MaskInto(dst, raw)
	if err != nil {
		t.Fatalf("MaskInto failed: %v", err)
	}
	if got, want := hex.EncodeToString(masked), pythonGoldenMask; got != want {
		t.Fatalf("masked packet mismatch:\n got=%s\nwant=%s", got, want)
	}
}

func TestMaskIntoReusesBuffer(t *testing.T) {
	id, err := New(16, "alpha", "beta")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	raw := make([]byte, 64)
	raw[0] = 0x01
	raw[1] = 0x00
	buf := make([]byte, 128)
	first, err := id.MaskInto(buf[:0], raw)
	if err != nil {
		t.Fatalf("MaskInto: %v", err)
	}
	if cap(first) < cap(buf) {
		t.Fatalf("MaskInto did not reuse dst backing: cap=%d want >= %d", cap(first), cap(buf))
	}
	second, err := id.MaskInto(buf[:0], raw)
	if err != nil {
		t.Fatalf("MaskInto second: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("MaskInto round two produced different bytes")
	}
}

func TestMaskIntoMatchesMask(t *testing.T) {
	id, err := New(16, pythonNetname, pythonNetkey)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	raw := mustHex(t, pythonRaw)
	alloc, err := id.Mask(raw)
	if err != nil {
		t.Fatalf("Mask: %v", err)
	}
	reuse, err := id.MaskInto(make([]byte, 0, len(alloc)), raw)
	if err != nil {
		t.Fatalf("MaskInto: %v", err)
	}
	if !bytes.Equal(alloc, reuse) {
		t.Fatalf("Mask and MaskInto differ:\n alloc=%x\nreuse=%x", alloc, reuse)
	}
}
