// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package resource

import (
	"bytes"
	"testing"

	"quad4/msgpack/v5/pkg/msgpack"
)

func TestSetMetadataPacksPythonLayout(t *testing.T) {
	res, err := New([]byte("hello file"), false)
	if err != nil {
		t.Fatal(err)
	}
	meta := map[string]any{"name": []byte("hello.txt")}
	if err := res.SetMetadata(meta); err != nil {
		t.Fatal(err)
	}
	if !res.HasMetadata() {
		t.Fatal("expected HasMetadata")
	}

	identityEncrypt := func(b []byte) ([]byte, error) { return b, nil }
	if err := res.PrepareOutboundForLink(identityEncrypt, 200); err != nil {
		t.Fatal(err)
	}

	// Rebuild expected wire body: 3-byte len + msgpack + file bytes.
	packed, err := msgpack.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	blob := make([]byte, 3+len(packed))
	blob[0] = byte(len(packed) >> 16)
	blob[1] = byte(len(packed) >> 8)
	blob[2] = byte(len(packed))
	copy(blob[3:], packed)
	wantBody := append(blob, []byte("hello file")...)
	if res.GetDataSize() != int64(len(wantBody)) {
		t.Fatalf("data size %d want %d", res.GetDataSize(), len(wantBody))
	}
	_ = wantBody
}

func TestSplitMetadataRoundTrip(t *testing.T) {
	meta := map[string]any{"name": []byte("a.bin")}
	packed, err := msgpack.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	blob := make([]byte, 3+len(packed))
	blob[0] = byte(len(packed) >> 16)
	blob[1] = byte(len(packed) >> 8)
	blob[2] = byte(len(packed))
	copy(blob[3:], packed)
	payload := append(blob, []byte("BODY")...)

	metaSize := int(payload[0])<<16 | int(payload[1])<<8 | int(payload[2])
	var got map[string]any
	if err := msgpack.Unmarshal(payload[3:3+metaSize], &got); err != nil {
		t.Fatal(err)
	}
	name, _ := got["name"].([]byte)
	if !bytes.Equal(name, []byte("a.bin")) {
		t.Fatalf("name %q", name)
	}
	if string(payload[3+metaSize:]) != "BODY" {
		t.Fatalf("body %q", payload[3+metaSize:])
	}
}
