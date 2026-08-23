// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"testing"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/resource"
)

func TestSplitResourceMetadataCorruptMsgpackStillStripsPrefix(t *testing.T) {
	adv := &resource.ResourceAdvertisement{HasMetadata: true}
	body := []byte("FILEBODY")
	// Valid 3-byte length pointing at 4 garbage msgpack bytes, then body.
	payload := []byte{0x00, 0x00, 0x04, 0xff, 0xff, 0xff, 0xff}
	payload = append(payload, body...)

	got, meta := splitResourceMetadata(payload, adv)
	if meta != nil {
		t.Fatalf("expected nil meta on corrupt msgpack, got %v", meta)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("body must be stripped of meta region: got %q want %q", got, body)
	}
}

func TestSplitResourceMetadata_IntegerKeys(t *testing.T) {
	packed := []byte{0x81, 0x01, 0x00}
	body := []byte("bundle-bytes")
	wire := make([]byte, 3+len(packed)+len(body))
	wire[0] = byte(len(packed) >> 16)
	wire[1] = byte(len(packed) >> 8)
	wire[2] = byte(len(packed))
	copy(wire[3:], packed)
	copy(wire[3+len(packed):], body)
	adv := &resource.ResourceAdvertisement{HasMetadata: true}
	gotBody, gotMeta := splitResourceMetadata(wire, adv)
	if !bytes.Equal(gotBody, body) {
		t.Fatalf("body=%q", gotBody)
	}
	if gotMeta == nil {
		t.Fatal("expected metadata")
	}
	code, ok := gotMeta["1"]
	if !ok {
		t.Fatalf("meta=%v", gotMeta)
	}
	switch v := code.(type) {
	case int64:
		if v != 0 {
			t.Fatalf("code=%v", v)
		}
	case int:
		if v != 0 {
			t.Fatalf("code=%v", v)
		}
	case int8:
		if v != 0 {
			t.Fatalf("code=%v", v)
		}
	default:
		t.Fatalf("code type %T", code)
	}
}

func TestSplitResourceMetadataHonestRoundTrip(t *testing.T) {
	adv := &resource.ResourceAdvertisement{HasMetadata: true}
	meta := map[string]any{"name": []byte("a.bin")}
	packed, err := msgpack.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("payload-bytes")
	wire := make([]byte, 3+len(packed)+len(body))
	n := len(packed)
	wire[0] = byte(n >> 16)
	wire[1] = byte(n >> 8)
	wire[2] = byte(n)
	copy(wire[3:], packed)
	copy(wire[3+n:], body)

	gotBody, gotMeta := splitResourceMetadata(wire, adv)
	if !bytes.Equal(gotBody, body) {
		t.Fatalf("body=%q want %q", gotBody, body)
	}
	name, _ := gotMeta["name"].([]byte)
	if string(name) != "a.bin" {
		t.Fatalf("meta=%v", gotMeta)
	}
}

// FuzzSplitResourceMetadataExploratory locks strip-vs-passthrough rules.
// When HasMetadata and the length prefix is in-bounds, the returned body
// must equal payload[3+metaSize:] even if msgpack unpack fails.
func FuzzSplitResourceMetadataExploratory(f *testing.F) {
	f.Add([]byte{0x00, 0x00, 0x01, 0x80, 0x41})
	f.Add([]byte{0x00})
	f.Add([]byte{0xff, 0xff, 0xff, 0x01})
	if packed, err := msgpack.Marshal(map[string]any{"k": []byte("v")}); err == nil {
		wire := make([]byte, 3+len(packed)+3)
		wire[0] = byte(len(packed) >> 16)
		wire[1] = byte(len(packed) >> 8)
		wire[2] = byte(len(packed))
		copy(wire[3:], packed)
		copy(wire[3+len(packed):], []byte("xyz"))
		f.Add(wire)
	}

	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) > 1<<14 {
			t.Skip()
		}
		adv := &resource.ResourceAdvertisement{HasMetadata: true}
		got, meta := splitResourceMetadata(payload, adv)

		if len(payload) < 3 {
			if !bytes.Equal(got, payload) || meta != nil {
				t.Fatalf("short payload must passthrough unchanged")
			}
			return
		}
		metaSize := int(payload[0])<<16 | int(payload[1])<<8 | int(payload[2])
		if metaSize < 0 || 3+metaSize > len(payload) {
			if !bytes.Equal(got, payload) || meta != nil {
				t.Fatalf("invalid size must passthrough unchanged")
			}
			return
		}
		wantBody := payload[3+metaSize:]
		if !bytes.Equal(got, wantBody) {
			t.Fatalf("body mismatch: got %d want %d (metaSize=%d)", len(got), len(wantBody), metaSize)
		}
		if meta != nil {
			if _, err := unpackResourceMetadata(payload[3 : 3+metaSize]); err != nil {
				t.Fatalf("meta non-nil but unpack failed: %v", err)
			}
		}
	})
}
