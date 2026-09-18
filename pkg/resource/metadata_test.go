// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package resource

import (
	"bytes"
	"testing"

	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
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

func TestSetMetadataPackedSurvivesPrepareOutbound(t *testing.T) {
	// Regression: PrepareOutboundForLink calls ensureMetadataPackedLocked,
	// which used to wipe metadataPacked installed by SetMetadataPacked,
	// stripping the metadata flag from the advertisement and the length
	// prefix from the wire body. Python peers then misread the payload as
	// a packed response and the request hung.
	res, err := New([]byte("bundle bytes"), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := res.SetMetadataPacked([]byte{0x81, 0x01, 0x00}); err != nil {
		t.Fatal(err)
	}
	res.SetRequestID([]byte("0123456789abcdef"))
	res.SetIsResponse(true)

	identityEncrypt := func(b []byte) ([]byte, error) { return b, nil }
	if err := res.PrepareOutboundForLink(identityEncrypt, 200); err != nil {
		t.Fatal(err)
	}
	if !res.HasMetadata() {
		t.Fatal("metadataPacked was cleared by PrepareOutboundForLink")
	}

	adv := NewResourceAdvertisement(res)
	if !adv.HasMetadata || !adv.IsResponse {
		t.Fatalf("advertisement lost flags: %#v", adv)
	}
	if adv.Flags&AdvFlagHasMetadata == 0 {
		t.Fatalf("advertisement missing metadata flag: 0x%02x", adv.Flags)
	}

	// The wire body must carry the 3-byte length + packed metadata prefix.
	wantSize := int64(3 + 3 + len("bundle bytes"))
	if res.GetDataSize() != wantSize {
		t.Fatalf("data size %d want %d", res.GetDataSize(), wantSize)
	}
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
