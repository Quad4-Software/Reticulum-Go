// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"errors"
	"github.com/Quad4-Software/reticulum-go-protocols/internal/testconfig"
	"strings"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

func TestAcceptance_1_AppNameIsLxmf(t *testing.T) {
	if AppName != "lxmf" {
		t.Fatalf("AppName=%q", AppName)
	}
}

func TestAcceptance_2_PackedLayoutHashesAndSignature(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	msg, err := NewMessage(dst.Hash(), src.Hash(), []byte("t"), []byte("c"), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw := mustPack(t, msg, src)
	if len(raw) < 2*DestinationLength+SignatureLength {
		t.Fatal("packed shorter than fixed overhead")
	}
	if !bytes.Equal(raw[:DestinationLength], dst.Hash()) {
		t.Fatal("destination hash prefix")
	}
	if !bytes.Equal(raw[DestinationLength:2*DestinationLength], src.Hash()) {
		t.Fatal("source hash prefix")
	}
}

func TestAcceptance_3_TamperRejectsSignature(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	identity.Remember(nil, src.Hash(), src.GetPublicKey(), nil)
	msg, err := NewMessage(dst.Hash(), src.Hash(), nil, []byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw := mustPack(t, msg, src)
	raw[len(raw)-1] ^= 0xff
	_, err = Unpack(raw, RecallSource)
	if err == nil {
		t.Fatal("tampered payload must fail")
	}
	if !errors.Is(err, ErrSignatureInvalid) && !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestAcceptance_4_PaperURIScheme(t *testing.T) {
	uri, err := PaperURI([]byte("accept"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(uri, "lxm://") {
		t.Fatalf("uri=%q", uri)
	}
}

func TestAcceptance_5_LiveDeliveryVerified(t *testing.T) {
	if testing.Short() {
		t.Skip("acceptance live skipped in -short")
	}
	mesh := newLXMFMesh(t, 42760)
	done := make(chan *LXMessage, 1)
	mesh.m2.SetMessageHandler(func(msg *LXMessage, _ common.NetworkInterface) {
		select {
		case done <- msg:
		default:
		}
	})
	if _, err := mesh.m1.SendText(mesh.h2, "acc", "verified"); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-done:
		if !msg.SignatureValidated {
			t.Fatal("inbound must validate signature")
		}
		if msg.ContentString() != "verified" {
			t.Fatalf("content=%q", msg.ContentString())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout")
	}
}

func TestAcceptance_UnknownDestinationRejected(t *testing.T) {
	cfg := testconfig.New()
	tr := transport.NewTransport(cfg)
	id := mustNewIdentity(t)
	m, err := NewDeliveryMessenger(id, tr)
	if err != nil {
		t.Fatal(err)
	}
	remote := bytes.Repeat([]byte{0x11}, DestinationLength)
	if _, err := m.SendText(remote, "", "x"); err == nil {
		t.Fatal("expected destination error")
	}
}
