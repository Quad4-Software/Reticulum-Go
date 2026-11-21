// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf"
)

func TestBlackBox_PublicAPI_PackUnpack(t *testing.T) {
	src, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	dst, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	identity.Remember(nil, src.Hash(), src.GetPublicKey(), nil)

	msg, err := lxmf.NewMessage(dst.Hash(), src.Hash(), []byte("bb"), []byte("blackbox"), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := msg.Pack(src)
	if err != nil {
		t.Fatal(err)
	}
	got, err := lxmf.Unpack(raw, lxmf.RecallSource)
	if err != nil {
		t.Fatal(err)
	}
	if got.ContentString() != "blackbox" || got.TitleString() != "bb" {
		t.Fatalf("got title=%q content=%q", got.TitleString(), got.ContentString())
	}
}

func TestBlackBox_PublicAPI_PaperURI(t *testing.T) {
	uri, err := lxmf.PaperURI([]byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(uri, "lxm://") {
		t.Fatal(uri)
	}
	got, err := lxmf.DecodePaperURI(uri)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("payload")) {
		t.Fatal("round-trip")
	}
}

func TestBlackBox_PublicAPI_Errors(t *testing.T) {
	_, err := lxmf.NewMessage(make([]byte, 8), make([]byte, 16), nil, nil, nil)
	if !errors.Is(err, lxmf.ErrInvalidHashLength) {
		t.Fatalf("err=%v", err)
	}
	_, err = lxmf.Unpack(make([]byte, 4), lxmf.RecallSource)
	if !errors.Is(err, lxmf.ErrMessageTooShort) {
		t.Fatalf("err=%v", err)
	}
}

func TestBlackBox_PublicAPI_Helpers(t *testing.T) {
	if lxmf.AppName != "lxmf" || lxmf.URISchema != "lxm" {
		t.Fatal("constants")
	}
	if lxmf.HexHash(nil) != "<empty>" {
		t.Fatal("HexHash")
	}
	raw := lxmf.EncodeAnnounceAppData("name")
	name, err := lxmf.DisplayNameFromAppData(raw)
	if err != nil || name != "name" {
		t.Fatalf("name=%q err=%v", name, err)
	}
}

func TestBlackBox_PublicAPI_Container(t *testing.T) {
	src, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	dst, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	identity.Remember(nil, src.Hash(), src.GetPublicKey(), nil)
	msg, err := lxmf.NewMessage(dst.Hash(), src.Hash(), nil, []byte("c"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := msg.Pack(src); err != nil {
		t.Fatal(err)
	}
	cdata, err := lxmf.PackContainer(msg)
	if err != nil {
		t.Fatal(err)
	}
	_, got, err := lxmf.UnpackContainer(cdata, lxmf.RecallSource)
	if err != nil {
		t.Fatal(err)
	}
	if got.ContentString() != "c" {
		t.Fatal(got.ContentString())
	}
}
