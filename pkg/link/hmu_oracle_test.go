// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"testing"

	"quad4/reticulum-go/pkg/resource"
)

func TestChooseHashmapUpdateSegmentOracleInvariants(t *testing.T) {
	const sdu = 384
	payload := bytes.Repeat([]byte{0x52}, 40000)
	res, err := resource.New(payload, false)
	if err != nil {
		t.Fatalf("resource.New: %v", err)
	}
	identityEncrypt := func(plain []byte) ([]byte, error) {
		return bytes.Repeat([]byte{0x11}, len(plain)), nil
	}
	if err := res.PrepareOutboundForLink(identityEncrypt, sdu); err != nil {
		t.Fatalf("PrepareOutboundForLink: %v", err)
	}

	entries := resource.HashmapEntriesPerSegment(sdu)
	total := int(res.GetSegments())
	if total <= entries {
		t.Fatalf("need parts > entries, got parts=%d entries=%d", total, entries)
	}

	for receiverMin := 0; receiverMin < total; receiverMin += max(entries/2, 1) {
		anchorIdx := min(receiverMin+entries-1, total-1)
		if anchorIdx < 0 {
			continue
		}
		anchor := res.MapHashAt(anchorIdx)
		seg, nextMin, ok := chooseHashmapUpdateSegment(res, sdu, anchor, receiverMin)
		if !ok {
			continue
		}
		if seg <= 0 {
			t.Fatalf("segment=%d must be > 0", seg)
		}
		if nextMin <= 0 || nextMin > total {
			t.Fatalf("nextMin=%d out of range", nextMin)
		}
		target := nextMin - 1
		if seg != (target+1)/entries {
			t.Fatalf("segment=%d want (target+1)/entries with target=%d entries=%d", seg, target, entries)
		}
		mh := res.MapHashAt(target)
		if !bytes.Equal(mh, anchor) {
			t.Fatalf("chosen target map hash mismatch at %d", target)
		}
	}
}

func TestSelectRequestedPartIndexesStaysInGuardWindow(t *testing.T) {
	const sdu = 32
	payload := bytes.Repeat([]byte{0x4D}, 320)
	res, err := resource.New(payload, false)
	if err != nil {
		t.Fatalf("resource.New: %v", err)
	}
	identityEncrypt := func(plain []byte) ([]byte, error) {
		return bytes.Repeat([]byte{0x7C}, len(plain)), nil
	}
	if err := res.PrepareOutboundForLink(identityEncrypt, sdu); err != nil {
		t.Fatalf("PrepareOutboundForLink: %v", err)
	}

	total := int(res.GetSegments())
	receiverMin := max(min(4, total-1), 0)
	var req []byte
	for i := range total {
		req = append(req, res.MapHashAt(i)...)
	}
	indexes := selectRequestedPartIndexes(res, req, receiverMin)
	lo := max(receiverMin-resource.CollisionGuardSize, 0)
	hi := min(receiverMin+resource.CollisionGuardSize, total)
	for _, idx := range indexes {
		if idx < lo || idx >= hi {
			t.Fatalf("index %d outside guard window [%d,%d)", idx, lo, hi)
		}
	}
}

// FuzzSelectRequestedPartIndexesOracle builds a small outbound resource and
// checks selected indexes stay inside the collision-guard window.
func FuzzSelectRequestedPartIndexesOracle(f *testing.F) {
	f.Add(uint8(0), []byte{0x00, 0x01, 0x02, 0x03})
	f.Add(uint8(3), []byte{0xff, 0xfe, 0xfd, 0xfc, 0x01, 0x02, 0x03, 0x04})
	f.Add(uint8(255), []byte{})

	f.Fuzz(func(t *testing.T, receiverMinU uint8, reqHashes []byte) {
		if len(reqHashes) > 64 {
			t.Skip()
		}
		const sdu = 32
		payload := bytes.Repeat([]byte{0x21}, 200)
		res, err := resource.New(payload, false)
		if err != nil {
			t.Fatal(err)
		}
		enc := func(plain []byte) ([]byte, error) {
			return bytes.Repeat([]byte{0x5A}, len(plain)), nil
		}
		if err := res.PrepareOutboundForLink(enc, sdu); err != nil {
			t.Fatal(err)
		}
		total := int(res.GetSegments())
		receiverMin := int(receiverMinU)
		if total > 0 {
			receiverMin %= total + 2
		}
		trimmed := reqHashes[:len(reqHashes)-len(reqHashes)%resource.MapHashLen]
		indexes := selectRequestedPartIndexes(res, trimmed, receiverMin)
		if receiverMin < 0 {
			receiverMin = 0
		}
		searchStart := receiverMin
		if searchStart >= total {
			searchStart = 0
		}
		searchEnd := min(searchStart+resource.CollisionGuardSize, total)
		backStart := max(receiverMin-resource.CollisionGuardSize, 0)
		for _, idx := range indexes {
			if idx < 0 || idx >= total {
				t.Fatalf("index %d out of [0,%d)", idx, total)
			}
			inForward := idx >= searchStart && idx < searchEnd
			inLookback := receiverMin > 0 && idx >= backStart && idx < searchStart
			if !inForward && !inLookback {
				t.Fatalf("index %d outside windows forward=[%d,%d) lookback=[%d,%d) min=%d",
					idx, searchStart, searchEnd, backStart, searchStart, receiverMin)
			}
		}
	})
}
