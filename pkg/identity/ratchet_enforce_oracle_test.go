// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"bytes"
	"testing"
)

// Regression: enforceRatchets=true must not fall back to the identity private
// key even when ratchets is nil/empty (Python Identity.decrypt ordering).
func TestOracleEnforceRatchetsBlocksIdentityFallback(t *testing.T) {
	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("BH_ID_RATCHET_ENFORCE")
	ct, err := id.Encrypt(plain, nil)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := id.Decrypt(ct, nil, true, nil)
	if err == nil && bytes.Equal(got, plain) {
		t.Fatal("Identity.Decrypt fell back to identity key under enforceRatchets")
	}
}
