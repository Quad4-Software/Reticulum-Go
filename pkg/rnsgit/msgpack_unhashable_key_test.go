// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"testing"

	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

// Request payloads are attacker-controlled on linked rnsgit sessions. A
// msgpack map with an array or map key is legal wire syntax but its key is
// unhashable in Go. Decoding must error, not panic with
// "hash of unhashable type".
func TestDecodeRequestUnhashableKey(t *testing.T) {
	data := []byte{0x81, 0x91, 0x01, 0x02} // {[1]:2}
	req, err := DecodeRequest(data)
	if err == nil {
		t.Fatalf("expected error for unhashable key, got req=%v", req)
	}
	t.Logf("clean error: %v", err)
}

func TestDecodeRequestNestedMapKey(t *testing.T) {
	data := []byte{0x81, 0x80, 0x01} // {{}:1}
	if _, err := DecodeRequest(data); err == nil {
		t.Fatal("expected error for nested map key")
	}
}

func TestDecodeRequestTypedMapUnhashableKey(t *testing.T) {
	data := []byte{0x81, 0x91, 0x01, 0x02} // {[1]:2}
	var m map[any]any
	if err := msgpack.Unmarshal(data, &m); err == nil {
		t.Fatalf("expected error for unhashable key in typed map, got %v", m)
	}
}

// Control: comparable keys still decode, including int and bin keys.
func TestDecodeRequestComparableKeys(t *testing.T) {
	data := []byte{0x82, 0x01, 0xa1, 0x61, 0xc4, 0x02, 0xaa, 0xbb, 0x05}
	req, err := DecodeRequest(data)
	if err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	if req == nil {
		t.Fatal("nil request")
	}
}
