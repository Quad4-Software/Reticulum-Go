// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package stamps

import (
	"context"
	"testing"
	"time"
)

func TestAnnounceExtRoundTrip(t *testing.T) {
	material := []byte("ext-material")
	stamp, _, err := GenerateStamp(context.Background(), material, 8, StampLength)
	if err != nil {
		t.Fatalf("GenerateStamp: %v", err)
	}
	appData := append([]byte("payload"), AnnounceExt(stamp, 12.5)...)
	if len(appData) != 7+ExtPayloadLen {
		t.Fatalf("ext len")
	}
	if appData[len(appData)-18] != ExtPayloadMarker {
		t.Fatal("marker misplaced")
	}
}

func TestParseAnnounceExt(t *testing.T) {
	destHash := []byte("0123456789abcdef")
	pub := make([]byte, 64)
	nameHash := []byte("0123456789")
	material := append(append(append([]byte{}, destHash...), pub...), nameHash...)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stamp, value, err := GenerateStamp(ctx, material, 8, StampLength)
	if err != nil {
		t.Fatalf("GenerateStamp: %v", err)
	}

	build := func(appData []byte) []byte {
		out := append([]byte(nil), appData...)
		return append(out, AnnounceExt(stamp, 12.5)...)
	}

	// Valid stamp strips the trailer and reports lr_cost.
	stripped, stamped, valid, gotStamp, gotValue, lr := ParseAnnounceExt(build([]byte("appdata")), destHash, pub, nameHash, 8)
	if !stamped || !valid {
		t.Fatal("valid ext not recognised")
	}
	if string(stripped) != "appdata" {
		t.Fatalf("stripped = %q", stripped)
	}
	if string(gotStamp) != string(stamp) || gotValue != value || lr != 12.5 {
		t.Fatalf("stamp=%x value=%v lr=%v", gotStamp, gotValue, lr)
	}

	// Ext-only app_data strips to nil.
	stripped, _, valid, _, _, _ = ParseAnnounceExt(build(nil), destHash, pub, nameHash, 8)
	if !valid || stripped != nil {
		t.Fatalf("ext-only strip: valid=%v stripped=%v", valid, stripped)
	}

	// Above the stamp's value: known format but invalid; app_data untouched.
	stripped, stamped, valid, gotStamp, gotValue, lr = ParseAnnounceExt(build([]byte("x")), destHash, pub, nameHash, value+4)
	if !stamped || valid || stripped != nil || lr != 0 {
		t.Fatalf("invalid stamp: stamped=%v valid=%v", stamped, valid)
	}
	if string(gotStamp) != string(stamp) || gotValue != value {
		t.Fatal("invalid stamp still records stamp and value")
	}

	// Extended flag set (anchor MSB) is not a known format.
	bad := build([]byte("x"))
	bad[len(bad)-1] |= 0x80
	if _, stamped, _, _, _, _ := ParseAnnounceExt(bad, destHash, pub, nameHash, 0); stamped {
		t.Fatal("extended-flag ext must not parse")
	}

	// Bad marker is not a known format.
	bad = build([]byte("x"))
	bad[len(bad)-18] = 0x00
	if _, stamped, _, _, _, _ := ParseAnnounceExt(bad, destHash, pub, nameHash, 0); stamped {
		t.Fatal("bad marker must not parse")
	}

	// lr_cost above the cap is not a known format.
	bad = build([]byte("x"))
	bad[len(bad)-1] = 101
	bad[len(bad)-2] = 0
	if _, stamped, _, _, _, _ := ParseAnnounceExt(bad, destHash, pub, nameHash, 0); stamped {
		t.Fatal("lr_cost > 100 must not parse")
	}

	// Short app_data never parses.
	if _, stamped, _, _, _, _ := ParseAnnounceExt([]byte("short"), destHash, pub, nameHash, 0); stamped {
		t.Fatal("short app_data must not parse")
	}
}
