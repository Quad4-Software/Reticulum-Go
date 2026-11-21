// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"encoding/hex"
	"fmt"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

func assertInteropFieldKeys(t *testing.T, fields map[string]any, want []byte) {
	t.Helper()
	if len(fields) < len(want) {
		t.Fatalf("fields=%#v want at least %d keys", fields, len(want))
	}
	for _, id := range want {
		key := fmt.Sprintf("0x%02x", id)
		if _, ok := fields[key]; !ok {
			t.Fatalf("missing field %s in %#v", key, fields)
		}
	}
}

func assertGoFieldKeys(t *testing.T, fields map[byte]any, want []byte) {
	t.Helper()
	if fields == nil {
		t.Fatal("fields nil")
	}
	for _, id := range want {
		if _, ok := fields[id]; !ok {
			t.Fatalf("missing field 0x%02x in %#v", id, fields)
		}
	}
}

func TestInterop_GoPackPythonUnpack_MessagingFields(t *testing.T) {
	requireInterop(t)
	src, _ := identityFromTest(t)
	dst, _ := identityFromTest(t)

	inner, err := NewMessage(outDeliveryHash(t, dst), outDeliveryHash(t, src), []byte("inner"), []byte("embedded"), nil)
	if err != nil {
		t.Fatal(err)
	}
	inner.Timestamp = 1700000098.0
	innerPacked, err := inner.Pack(src)
	if err != nil {
		t.Fatal(err)
	}

	fields := MessagingInteropFields(innerPacked)
	msg, err := NewMessage(outDeliveryHash(t, dst), outDeliveryHash(t, src), []byte("interop"), []byte("messaging fields"), fields)
	if err != nil {
		t.Fatal(err)
	}
	msg.Timestamp = 1700000100.0
	packed, err := msg.Pack(src)
	if err != nil {
		t.Fatal(err)
	}

	resp := interopCall(t, map[string]any{
		"cmd":        "unpack",
		"packed":     hex.EncodeToString(packed),
		"identities": interopIdentities(t, src),
	})
	if !resp.Message.SignatureValidated {
		t.Fatal("python did not validate messaging fields")
	}
	if resp.Message.Content != "messaging fields" {
		t.Fatalf("content=%q", resp.Message.Content)
	}
	assertInteropFieldKeys(t, resp.Message.Fields, MessagingInteropFieldKeys(true))
}

func TestInterop_PythonPackGoUnpack_MessagingFields(t *testing.T) {
	requireInterop(t)

	innerResp := interopCall(t, map[string]any{
		"cmd":       "pack",
		"title":     "inner",
		"content":   "embedded",
		"timestamp": 1700000098.0,
	})
	fields := MessagingInteropFieldsPython(innerResp.Packed)
	resp := interopCall(t, map[string]any{
		"cmd":       "pack",
		"title":     "interop",
		"content":   "from python messaging fields",
		"timestamp": 1700000100.0,
		"fields":    fields,
	})
	registerInteropSource(t, resp.SourceHash, resp.SourcePublicKey)

	packed, err := hex.DecodeString(resp.Packed)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Unpack(packed, RecallSource)
	if err != nil {
		t.Fatal(err)
	}
	if !got.SignatureValidated {
		t.Fatal("go did not validate python messaging fields")
	}
	if got.ContentString() != "from python messaging fields" {
		t.Fatalf("content=%q", got.ContentString())
	}
	assertGoFieldKeys(t, got.Fields, MessagingInteropFieldKeys(true))
}

func TestInterop_MessagingFields_IconAndAttachments(t *testing.T) {
	requireInterop(t)
	src, _ := identityFromTest(t)
	dst, _ := identityFromTest(t)

	fields := map[byte]any{
		FieldIconAppearance: map[byte]any{
			0x00: []byte("emoji"),
			0x01: []byte{0x89, 0x50, 0x4e, 0x47},
		},
		FieldFileAttachments: []any{
			map[byte]any{
				0x00: []byte("notes.txt"),
				0x01: []byte("file-body"),
			},
		},
	}
	msg, err := NewMessage(outDeliveryHash(t, dst), outDeliveryHash(t, src), []byte("attach"), []byte("see files"), fields)
	if err != nil {
		t.Fatal(err)
	}
	packed, err := msg.Pack(src)
	if err != nil {
		t.Fatal(err)
	}
	resp := interopCall(t, map[string]any{
		"cmd":        "unpack",
		"packed":     hex.EncodeToString(packed),
		"identities": interopIdentities(t, src),
	})
	if !resp.Message.SignatureValidated {
		t.Fatal("signature")
	}
	assertInteropFieldKeys(t, resp.Message.Fields, []byte{FieldIconAppearance, FieldFileAttachments})
}

func TestInterop_MessagingFields_EmbeddedRoundTrip(t *testing.T) {
	requireInterop(t)
	src, _ := identityFromTest(t)
	dst, _ := identityFromTest(t)

	inner, err := NewMessage(outDeliveryHash(t, dst), outDeliveryHash(t, src), []byte("inner"), []byte("payload"), nil)
	if err != nil {
		t.Fatal(err)
	}
	innerPacked, err := inner.Pack(src)
	if err != nil {
		t.Fatal(err)
	}

	fields := map[byte]any{
		FieldEmbeddedLXMs: [][]byte{innerPacked},
	}
	msg, err := NewMessage(outDeliveryHash(t, dst), outDeliveryHash(t, src), []byte("wrap"), []byte("outer"), fields)
	if err != nil {
		t.Fatal(err)
	}
	packed, err := msg.Pack(src)
	if err != nil {
		t.Fatal(err)
	}
	resp := interopCall(t, map[string]any{
		"cmd":        "unpack",
		"packed":     hex.EncodeToString(packed),
		"identities": interopIdentities(t, src),
	})
	if !resp.Message.SignatureValidated {
		t.Fatal("signature")
	}
	assertInteropFieldKeys(t, resp.Message.Fields, []byte{FieldEmbeddedLXMs})
}

func identityFromTest(t *testing.T) (*identity.Identity, *identity.Identity) {
	t.Helper()
	src, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	dst, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return src, dst
}
