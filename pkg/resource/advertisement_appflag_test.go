// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package resource

import (
	"bytes"
	"testing"
)

func TestResourceAdvertisementAppFlagRoundTrip(t *testing.T) {
	for _, set := range []bool{false, true} {
		res := &Resource{appFlag: set, hash: bytesSeq(32)}
		adv := NewResourceAdvertisement(res)
		if adv.AppFlag != set {
			t.Fatalf("AppFlag=%v want %v", adv.AppFlag, set)
		}
		if (adv.Flags&AdvFlagApp != 0) != set {
			t.Fatalf("flags %#02x app bit wrong", adv.Flags)
		}
		// Other flag bits unaffected.
		if adv.Flags&^(AdvFlagApp) != 0 {
			t.Fatalf("unexpected flag bits %#02x", adv.Flags)
		}
		packed, err := adv.Pack(0, 1500)
		if err != nil {
			t.Fatal(err)
		}
		got, err := UnpackResourceAdvertisement(packed)
		if err != nil {
			t.Fatal(err)
		}
		if got.AppFlag != set {
			t.Fatalf("unpacked AppFlag=%v want %v", got.AppFlag, set)
		}
	}
}

func TestResourceAdvertisementOversizeDefersHashmap(t *testing.T) {
	res := &Resource{appFlag: false, hash: bytesSeq(32), hashmap: bytes.Repeat([]byte{0xAB}, MapHashLen*64)}
	res.segments = 4
	adv := NewResourceAdvertisement(res)
	packed, err := adv.Pack(0, 50) // tiny MDU forces "m": b""
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnpackResourceAdvertisement(packed)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hashmap) != 0 {
		t.Fatalf("expected deferred empty hashmap, got %d bytes", len(got.Hashmap))
	}
}
