// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package resource

import (
	"bytes"
	"testing"
)

func identityEncrypt(plain []byte) ([]byte, error) {
	return append([]byte(nil), plain...), nil
}

func TestPrepareOutboundSplitsLargePayload(t *testing.T) {
	body := bytes.Repeat([]byte("abcdefgh"), (MaxEfficientSize/8)+64)
	res, err := New(body, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := res.PrepareOutboundForLink(identityEncrypt, 200); err != nil {
		t.Fatal(err)
	}
	if !res.IsSplit() {
		t.Fatal("expected split resource")
	}
	if res.GetSegmentIndex() != 1 {
		t.Fatalf("segment index: got %d want 1", res.GetSegmentIndex())
	}
	if res.GetTotalSegments() < 2 {
		t.Fatalf("total segments: got %d want >= 2", res.GetTotalSegments())
	}
	adv := NewResourceAdvertisement(res)
	if adv == nil || !adv.Split {
		t.Fatal("advertisement missing split flag")
	}
	if adv.SegmentIndex != 1 || adv.TotalSegments != res.GetTotalSegments() {
		t.Fatalf("adv segments: i=%d l=%d", adv.SegmentIndex, adv.TotalSegments)
	}
	orig := res.GetOriginalHash()

	if err := res.PrepareNextOutboundSegment(identityEncrypt, 200); err != nil {
		t.Fatal(err)
	}
	if res.GetSegmentIndex() != 2 {
		t.Fatalf("segment index after next: got %d want 2", res.GetSegmentIndex())
	}
	if !bytes.Equal(res.GetOriginalHash(), orig) {
		t.Fatal("original hash changed across segments")
	}
}

func TestPrepareOutboundSmallNotSplit(t *testing.T) {
	res, err := New([]byte("small"), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := res.PrepareOutboundForLink(identityEncrypt, 200); err != nil {
		t.Fatal(err)
	}
	if res.IsSplit() {
		t.Fatal("small resource should not split")
	}
	if res.GetTotalSegments() != 1 || res.GetSegmentIndex() != 1 {
		t.Fatalf("segments: i=%d l=%d", res.GetSegmentIndex(), res.GetTotalSegments())
	}
}
