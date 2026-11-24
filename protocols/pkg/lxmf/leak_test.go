// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"context"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/internal/leaktest"
)

func TestLeak_LXMF_PackUnpack(t *testing.T) {
	base := leaktest.Baseline()
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	dh, sh := dst.Hash(), src.Hash()
	for i := range 500 {
		msg, err := NewMessage(dh, sh, []byte("title"), []byte("content"), nil)
		if err != nil {
			t.Fatal(err)
		}
		msg.Timestamp = 1_700_000_000.0 + float64(i)
		if _, err := msg.Pack(src); err != nil {
			t.Fatal(err)
		}
	}
	leaktest.AssertStable(t, base, 12, 5*time.Second)
}

func TestLeak_LXMF_UnpackRoundTrip(t *testing.T) {
	base := leaktest.Baseline()
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	dh, sh := dst.Hash(), src.Hash()
	identity.Remember(nil, sh, src.GetPublicKey(), nil)
	for i := range 300 {
		msg, err := NewMessage(dh, sh, []byte("t"), []byte("c"), nil)
		if err != nil {
			t.Fatal(err)
		}
		msg.Timestamp = 1_700_000_100.0 + float64(i)
		packed, err := msg.Pack(src)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Unpack(packed, RecallSource); err != nil {
			t.Fatal(err)
		}
	}
	leaktest.AssertStable(t, base, 12, 5*time.Second)
}

func TestLeak_LXMF_GenerateStamp(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	base := leaktest.Baseline()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	mid := bytes.Repeat([]byte{0x66}, 32)
	_, _, err := GenerateStamp(ctx, mid, 4, WorkblockExpandRounds)
	if err != nil {
		t.Fatal(err)
	}
	leaktest.AssertStable(t, base, 16, 6*time.Second)
}
