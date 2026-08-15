// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package securemem

import (
	"bytes"
	"testing"
)

func TestBufRoundTrip(t *testing.T) {
	b, err := New(32)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	src := bytes.Repeat([]byte{0xab}, 32)
	if err := b.CopyFrom(src); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b.Bytes(), src) {
		t.Fatal("bytes mismatch")
	}
	out := b.CopyOut()
	if !bytes.Equal(out, src) {
		t.Fatal("copy out mismatch")
	}
	WipeBytes(out)
	if !bytes.Equal(out, make([]byte, 32)) {
		t.Fatal("wipe failed")
	}
}

func TestBufWipeAndClose(t *testing.T) {
	b, err := New(16)
	if err != nil {
		t.Fatal(err)
	}
	_ = b.CopyFrom(bytes.Repeat([]byte{1}, 16))
	b.Wipe()
	if !bytes.Equal(b.Bytes(), make([]byte, 16)) {
		t.Fatal("wipe left non-zero")
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if b.Len() != 0 || b.Bytes() != nil {
		t.Fatal("closed buffer still live")
	}
}

func TestBufCopyFromLength(t *testing.T) {
	b, err := New(8)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err := b.CopyFrom([]byte{1, 2}); err == nil {
		t.Fatal("expected length mismatch")
	}
}
