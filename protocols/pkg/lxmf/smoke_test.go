// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

func TestSmoke_PackUnpackPlain(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	identity.Remember(nil, src.Hash(), src.GetPublicKey(), nil)
	msg, err := NewMessage(dst.Hash(), src.Hash(), []byte("t"), []byte("smoke"), nil)
	if err != nil {
		t.Fatal(err)
	}
	packed := mustPack(t, msg, src)
	got, err := Unpack(packed, RecallSource)
	if err != nil {
		t.Fatal(err)
	}
	if got.ContentString() != "smoke" {
		t.Fatalf("content=%q", got.ContentString())
	}
}

func TestSmoke_PaperURI(t *testing.T) {
	payload := []byte("paper-smoke")
	uri, err := PaperURI(payload)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodePaperURI(uri)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("paper round-trip")
	}
}

func TestSmoke_ContainerRoundTrip(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	identity.Remember(nil, src.Hash(), src.GetPublicKey(), nil)
	msg, err := NewMessage(dst.Hash(), src.Hash(), nil, []byte("c"), nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = mustPack(t, msg, src)
	msg.Method = MethodDirect
	cdata, err := PackContainer(msg)
	if err != nil {
		t.Fatal(err)
	}
	_, got, err := UnpackContainer(cdata, RecallSource)
	if err != nil {
		t.Fatal(err)
	}
	if got.ContentString() != "c" {
		t.Fatalf("content=%q", got.ContentString())
	}
}

func TestSmoke_AnnounceAppDataV5(t *testing.T) {
	raw, err := EncodeAnnounceAppDataV5("bob", 8)
	if err != nil {
		t.Fatal(err)
	}
	name, err := DisplayNameFromAppData(raw)
	if err != nil || name != "bob" {
		t.Fatalf("name=%q err=%v", name, err)
	}
	cost, ok, _ := StampCostFromAppData(raw)
	if !ok || cost != 8 {
		t.Fatalf("cost=%d ok=%v", cost, ok)
	}
}
