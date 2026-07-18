// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package resource

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestPrepareOutboundForLinkLayoutInvariants(t *testing.T) {
	const sdu = 256
	payload := bytes.Repeat([]byte{0xA1}, 500)
	res, err := New(payload, false)
	if err != nil {
		t.Fatal(err)
	}
	enc := func(plain []byte) ([]byte, error) {
		return append([]byte(nil), plain...), nil
	}
	if err := res.PrepareOutboundForLink(enc, sdu); err != nil {
		t.Fatal(err)
	}
	assertOutboundLayout(t, res, sdu)
}

func TestPrepareOutboundForLinkWithMetadataLayout(t *testing.T) {
	const sdu = 256
	payload := []byte("file-body-contents")
	res, err := New(payload, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := res.SetMetadata(map[string]any{"name": []byte("x.bin")}); err != nil {
		t.Fatal(err)
	}
	enc := func(plain []byte) ([]byte, error) {
		return append([]byte(nil), plain...), nil
	}
	if err := res.PrepareOutboundForLink(enc, sdu); err != nil {
		t.Fatal(err)
	}
	assertOutboundLayout(t, res, sdu)
	if !res.HasMetadata() {
		t.Fatal("expected metadata flag")
	}
}

func TestHashmapEntriesPerSegmentNeverNonPositive(t *testing.T) {
	for _, mdu := range []int{-1, 0, 1, 64, 133, 134, 135, 384, 1024} {
		n := HashmapEntriesPerSegment(mdu)
		if n <= 0 {
			t.Fatalf("HashmapEntriesPerSegment(%d)=%d want > 0", mdu, n)
		}
	}
}

func assertOutboundLayout(t *testing.T, res *Resource, sdu int) {
	t.Helper()
	parts := int(res.GetSegments())
	if parts <= 0 {
		t.Fatal("expected positive part count")
	}
	cipherLen := 0
	for i := 0; i < parts; i++ {
		slice := res.OutboundCiphertextSlice(i, sdu)
		if len(slice) == 0 {
			t.Fatalf("empty ciphertext slice at part %d", i)
		}
		cipherLen += len(slice)
		mh := res.MapHashAt(i)
		if len(mh) != MapHashLen {
			t.Fatalf("map hash len at %d", i)
		}
		sum := sha256.Sum256(append(append([]byte{}, slice...), res.GetRandomHash()...))
		if !bytes.Equal(mh, sum[:MapHashLen]) {
			t.Fatalf("map hash mismatch at part %d", i)
		}
	}
	wantParts := (cipherLen + sdu - 1) / sdu
	if parts != wantParts {
		t.Fatalf("parts=%d want ceil(cipher/sdu)=%d cipher=%d", parts, wantParts, cipherLen)
	}

	entries := HashmapEntriesPerSegment(sdu)
	if entries <= 0 {
		t.Fatalf("entries per segment must be positive, got %d", entries)
	}
	covered := 0
	for seg := 0; ; seg++ {
		chunk := res.HashmapSegment(sdu, seg)
		if len(chunk) == 0 {
			break
		}
		if len(chunk)%MapHashLen != 0 {
			t.Fatalf("segment %d length %d not multiple of MapHashLen", seg, len(chunk))
		}
		n := len(chunk) / MapHashLen
		if n > entries {
			t.Fatalf("segment %d has %d entries > per-segment %d", seg, n, entries)
		}
		for j := 0; j < n; j++ {
			off := j * MapHashLen
			idx := covered + j
			want := res.MapHashAt(idx)
			if !bytes.Equal(chunk[off:off+MapHashLen], want) {
				t.Fatalf("HashmapSegment(%d) entry %d mismatch part %d", seg, j, idx)
			}
		}
		covered += n
	}
	if covered != parts {
		t.Fatalf("HashmapSegment coverage %d != parts %d", covered, parts)
	}
}

// FuzzPrepareOutboundForLinkLayoutOracle prepares outbound resources for
// arbitrary sizes and SDUs and checks part counts, map hashes, and segment
// coverage stay consistent.
func FuzzPrepareOutboundForLinkLayoutOracle(f *testing.F) {
	f.Add(10, 256, false)
	f.Add(200, 384, true)
	f.Add(0, 200, false)
	f.Add(1500, 512, true)
	f.Add(50, 32, false)

	f.Fuzz(func(t *testing.T, size int, sdu int, withMeta bool) {
		if size < 0 || size > 1<<14 || sdu < 8 || sdu > 1024 {
			t.Skip()
		}
		payload := bytes.Repeat([]byte{0x3C}, size)
		res, err := New(payload, false)
		if err != nil {
			t.Fatal(err)
		}
		if withMeta {
			if err := res.SetMetadata(map[string]any{"name": []byte("f")}); err != nil {
				t.Fatal(err)
			}
		}
		enc := func(plain []byte) ([]byte, error) {
			out := make([]byte, len(plain))
			copy(out, plain)
			return out, nil
		}
		if err := res.PrepareOutboundForLink(enc, sdu); err != nil {
			t.Fatal(err)
		}
		assertOutboundLayout(t, res, sdu)
	})
}
