// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/cryptography"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/packet"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

const pathEstablishWait = 10 * time.Second

func TestMessenger_Compose(t *testing.T) {
	cfg := common.DefaultConfig()
	tr := transport.NewTransport(cfg)
	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	dest, err := NewDeliveryDestination(id, tr)
	if err != nil {
		t.Fatalf("destination: %v", err)
	}
	m := NewMessenger(tr, dest)

	remote := make([]byte, DestinationLength)
	for i := range remote {
		remote[i] = byte(i + 1)
	}

	msg, err := m.Compose(remote, "hi", "there", nil)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if !bytes.Equal(msg.SourceHash, m.DestinationHash()) {
		t.Errorf("source hash mismatch")
	}
	if msg.TitleString() != "hi" || msg.ContentString() != "there" {
		t.Errorf("title/content mismatch: %q / %q", msg.TitleString(), msg.ContentString())
	}
}

func TestMessenger_Send_RejectsUnknownDestination(t *testing.T) {
	cfg := common.DefaultConfig()
	tr := transport.NewTransport(cfg)
	id, _ := identity.NewIdentity()
	m, err := NewDeliveryMessenger(id, tr)
	if err != nil {
		t.Fatalf("messenger: %v", err)
	}

	remote := make([]byte, DestinationLength)
	if _, err := m.SendText(remote, "", "x"); err == nil {
		t.Fatal("expected error for unknown destination identity")
	}
}

func TestMessenger_SetSourceResolver(t *testing.T) {
	cfg := common.DefaultConfig()
	tr := transport.NewTransport(cfg)
	id, _ := identity.NewIdentity()
	m, err := NewDeliveryMessenger(id, tr)
	if err != nil {
		t.Fatalf("messenger: %v", err)
	}

	called := false
	m.SetSourceResolver(func(_ []byte) (*identity.Identity, error) {
		called = true
		return nil, errors.New("forced")
	})

	if m.resolver == nil {
		t.Fatal("resolver not set")
	}
	if _, err := m.resolver(nil); err == nil || !called {
		t.Fatal("custom resolver was not used")
	}

	m.SetSourceResolver(nil)
	if m.resolver == nil {
		t.Fatal("nil resolver should restore default")
	}
}

func TestMessenger_TwoWayLoopback(t *testing.T) {
	if testing.Short() {
		t.Skip("loopback messenger test skipped in -short mode")
	}

	mesh := newLXMFMesh(t, 42440)
	m1, m2 := mesh.m1, mesh.m2
	dest1, dest2 := mesh.h1, mesh.h2

	var wg sync.WaitGroup
	wg.Add(2)
	var got1, got2 *LXMessage
	m1.SetMessageHandler(func(msg *LXMessage, _ common.NetworkInterface) {
		got1 = msg
		wg.Done()
	})
	m2.SetMessageHandler(func(msg *LXMessage, _ common.NetworkInterface) {
		got2 = msg
		wg.Done()
	})

	t.Logf("paths established d1=%s d2=%s", hex.EncodeToString(dest1), hex.EncodeToString(dest2))

	if _, err := m1.SendText(dest2, "from1", "hello two"); err != nil {
		t.Fatalf("send 1->2: %v", err)
	}
	if _, err := m2.SendText(dest1, "from2", "hello one"); err != nil {
		t.Fatalf("send 2->1: %v", err)
	}

	doneCh := make(chan struct{})
	go func() { wg.Wait(); close(doneCh) }()
	select {
	case <-doneCh:
	case <-time.After(pathEstablishWait):
		t.Fatalf("timeout waiting for inbound messages")
	}

	if got1 == nil || got1.ContentString() != "hello one" {
		t.Errorf("messenger 1 received: %#v", got1)
	}
	if got2 == nil || got2.ContentString() != "hello two" {
		t.Errorf("messenger 2 received: %#v", got2)
	}
	if got1 != nil && !got1.SignatureValidated {
		t.Errorf("inbound 1 signature not validated: reason=%d", got1.UnverifiedReason)
	}
	if got2 != nil && !got2.SignatureValidated {
		t.Errorf("inbound 2 signature not validated: reason=%d", got2.UnverifiedReason)
	}
}

func TestMessenger_RatchetEncryptedInbound(t *testing.T) {
	cfg := common.DefaultConfig()
	tr := transport.NewTransport(cfg)
	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	dest, err := NewDeliveryDestination(id, tr)
	if err != nil {
		t.Fatalf("destination: %v", err)
	}
	m := NewMessenger(tr, dest)

	if _, err := id.RotateRatchet(); err != nil {
		t.Fatalf("RotateRatchet: %v", err)
	}
	ratchetPriv := id.GetCurrentRatchetKey()
	if ratchetPriv == nil {
		t.Fatal("expected identity ratchet key")
	}
	ratchetPub, err := cryptography.PublicKeyFromPrivate(ratchetPriv)
	if err != nil {
		t.Fatalf("ratchet public key: %v", err)
	}

	remote := make([]byte, DestinationLength)
	msg, err := m.Compose(remote, "ratchet", "encrypted inbound", nil)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if _, err := msg.Pack(id); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	inner, err := msg.EncryptedPayload()
	if err != nil {
		t.Fatalf("EncryptedPayload: %v", err)
	}

	senderView := identity.FromPublicKey(id.GetPublicKey())
	ciphertext, err := senderView.Encrypt(inner, ratchetPub)
	if err != nil {
		t.Fatalf("Encrypt with ratchet: %v", err)
	}

	done := make(chan *LXMessage, 1)
	m.SetMessageHandler(func(msg *LXMessage, _ common.NetworkInterface) {
		done <- msg
	})

	pkt := packet.NewPacket(
		packet.DestinationSingle,
		ciphertext,
		packet.PacketTypeData,
		packet.ContextNone,
		packet.PropagationBroadcast,
		packet.HeaderType1,
		nil,
		true,
		packet.FlagUnset,
	)
	m.Receive(pkt, nil)

	select {
	case got := <-done:
		if got.ContentString() != "encrypted inbound" {
			t.Errorf("content = %q", got.ContentString())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ratchet-encrypted inbound message")
	}
}
