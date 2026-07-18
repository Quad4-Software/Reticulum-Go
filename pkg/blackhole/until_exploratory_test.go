// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package blackhole

import (
	"bytes"
	"testing"

	"quad4/msgpack/v5/pkg/msgpack"
)

func TestDecodeBlackholeMapRejectsNonNumericUntil(t *testing.T) {
	hash := string(bytes.Repeat([]byte{0x11}, HashLen))
	raw, err := encodeBlackholeMap(map[string]map[string]any{
		hash: {
			"source": bytes.Repeat([]byte{0x22}, HashLen),
			"until":  "not-a-timestamp",
			"reason": "bad",
		},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if _, err := DecodeBlackholeMap(raw); err == nil {
		t.Fatal("expected error for string until")
	}
}

func TestDecodeBlackholeMapNilUntilIsPermanent(t *testing.T) {
	hash := string(bytes.Repeat([]byte{0x33}, HashLen))
	raw, err := encodeBlackholeMap(map[string]map[string]any{
		hash: {
			"source": bytes.Repeat([]byte{0x44}, HashLen),
			"until":  nil,
			"reason": "perm",
		},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := DecodeBlackholeMap(raw)
	if err != nil {
		t.Fatalf("DecodeBlackholeMap: %v", err)
	}
	e, ok := got[hash]
	if !ok {
		t.Fatal("missing entry")
	}
	if e.Until != 0 {
		t.Fatalf("Until=%v want 0 for nil wire until", e.Until)
	}
}

func TestMergeRemoteDoesNotImmortalizeBadUntil(t *testing.T) {
	dir := t.TempDir()
	newLocal(t)
	tab := New(dir)
	src := bytes.Repeat([]byte{0x55}, HashLen)
	id := string(bytes.Repeat([]byte{0x66}, HashLen))

	buf := &bytes.Buffer{}
	enc := msgpack.NewEncoder(buf)
	_ = enc.EncodeMapLen(1)
	_ = enc.Encode([]byte(id))
	_ = enc.EncodeMapLen(2)
	_ = enc.Encode("source")
	_ = enc.Encode(src)
	_ = enc.Encode("until")
	_ = enc.Encode([]byte("tomorrow"))

	if _, err := DecodeBlackholeMap(buf.Bytes()); err == nil {
		t.Fatal("DecodeBlackholeMap must reject binary until")
	}
	if err := tab.MergeRemote(src, map[string]Entry{
		id: {Source: src, Until: 0, Reason: "should-not-matter"},
	}); err != nil {
		t.Fatalf("MergeRemote of explicit permanent: %v", err)
	}
	if !tab.Has([]byte(id)) {
		t.Fatal("explicit Until=0 remote merge should still apply")
	}
}
