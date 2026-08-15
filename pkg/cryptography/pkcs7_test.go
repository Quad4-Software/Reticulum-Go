// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"errors"
	"testing"
)

func TestIsPaddingError(t *testing.T) {
	if !IsPaddingError(errPaddingEmpty) || !IsPaddingError(errPaddingBytes) {
		t.Fatal("sentinel padding errors should match")
	}
	if IsPaddingError(errors.New("other")) || IsPaddingError(nil) {
		t.Fatal("non-padding errors must not match")
	}
	_, err := RemovePKCS7Padding(nil)
	if !IsPaddingError(err) {
		t.Fatalf("empty plaintext: %v", err)
	}
}
