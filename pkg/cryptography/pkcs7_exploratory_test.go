// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"bytes"
	"crypto/aes"
	"testing"
)

func addPKCS7Padding(plaintext []byte) []byte {
	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	out := make([]byte, len(plaintext)+padding)
	copy(out, plaintext)
	for i := len(plaintext); i < len(out); i++ {
		out[i] = byte(padding)
	}
	return out
}

func TestRemovePKCS7PaddingRoundTrip(t *testing.T) {
	for _, n := range []int{0, 1, 15, 16, 17, 31, 32, 64} {
		in := bytes.Repeat([]byte{0xA5}, n)
		padded := addPKCS7Padding(in)
		got, err := RemovePKCS7Padding(padded)
		if err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		if !bytes.Equal(got, in) {
			t.Fatalf("n=%d mismatch", n)
		}
	}
}

func TestRemovePKCS7PaddingRejectsUnaligned(t *testing.T) {
	_, err := RemovePKCS7Padding([]byte{0x01})
	if !IsPaddingError(err) {
		t.Fatalf("expected padding error for unaligned input, got %v", err)
	}
}

func TestRemovePKCS7PaddingRejectsBadBytes(t *testing.T) {
	padded := addPKCS7Padding([]byte("hello"))
	padded[len(padded)-2] ^= 0xff
	_, err := RemovePKCS7Padding(padded)
	if err != errPaddingBytes {
		t.Fatalf("expected errPaddingBytes, got %v", err)
	}
}

// FuzzRemovePKCS7PaddingExploratory asserts success only for valid PKCS#7 on
// AES block-aligned buffers, and that failures are padding sentinels.
func FuzzRemovePKCS7PaddingExploratory(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x01})
	f.Add(bytes.Repeat([]byte{0x10}, 16))
	f.Add(addPKCS7Padding([]byte("reticulum")))

	f.Fuzz(func(t *testing.T, pt []byte) {
		if len(pt) > 1<<12 {
			t.Skip()
		}
		got, err := RemovePKCS7Padding(pt)
		if err != nil {
			if !IsPaddingError(err) {
				t.Fatalf("non-padding error: %v", err)
			}
			return
		}
		if len(pt)%aes.BlockSize != 0 {
			t.Fatal("accepted non-block-aligned plaintext")
		}
		pad := int(pt[len(pt)-1])
		if pad < 1 || pad > aes.BlockSize || pad > len(pt) {
			t.Fatalf("accepted impossible pad=%d len=%d", pad, len(pt))
		}
		for i := len(pt) - pad; i < len(pt); i++ {
			if pt[i] != byte(pad) {
				t.Fatalf("accepted mismatched pad byte at %d", i)
			}
		}
		if len(got) != len(pt)-pad {
			t.Fatalf("returned len=%d want %d", len(got), len(pt)-pad)
		}
		if !bytes.Equal(got, pt[:len(pt)-pad]) {
			t.Fatal("returned body mismatch")
		}
	})
}
