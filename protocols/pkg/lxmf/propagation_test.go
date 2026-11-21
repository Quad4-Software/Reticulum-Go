// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

func TestPropagationRegistry_PNAnnounce(t *testing.T) {
	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	propHash, err := PropagationDestinationHash(id)
	if err != nil {
		t.Fatalf("prop hash: %v", err)
	}

	blob := buildPNAnnounce(t, time.Now().Unix(), true, 256, 10240, 16, 3, 18, map[byte]any{
		PNMetaName: []byte("Test PN"),
	})

	reg := NewPropagationRegistry()
	if err := reg.ReceivedAnnounce(propHash, id, blob, 2); err != nil {
		t.Fatalf("ReceivedAnnounce: %v", err)
	}

	nodes := reg.List()
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Name != "Test PN" {
		t.Errorf("name = %q", nodes[0].Name)
	}
	if nodes[0].StampCost != 16 {
		t.Errorf("stamp cost = %d", nodes[0].StampCost)
	}
}

func TestPropagationRegistry_IgnoresDeliveryAnnounce(t *testing.T) {
	id, _ := identity.NewIdentity()
	tr := transport.NewTransport(common.DefaultConfig())
	dest, err := NewDeliveryDestination(id, tr)
	if err != nil {
		t.Fatalf("dest: %v", err)
	}

	appData, err := EncodeAnnounceAppDataV5("not a pn", 8)
	if err != nil {
		t.Fatalf("app data: %v", err)
	}

	reg := NewPropagationRegistry()
	if err := reg.ReceivedAnnounce(dest.GetHash(), id, appData, 1); err != nil {
		t.Fatalf("ReceivedAnnounce: %v", err)
	}
	if len(reg.List()) != 0 {
		t.Fatal("delivery announce should not register as propagation node")
	}
}

func TestPackPropagated_RoundTripFields(t *testing.T) {
	src, _ := identity.NewIdentity()
	dst, _ := identity.NewIdentity()
	tr := transport.NewTransport(common.DefaultConfig())

	srcDest, err := NewDeliveryDestination(src, tr)
	if err != nil {
		t.Fatalf("src dest: %v", err)
	}
	dstDest, err := NewDeliveryDestination(dst, tr)
	if err != nil {
		t.Fatalf("dst dest: %v", err)
	}
	identity.Remember(nil, dstDest.GetHash(), dst.GetPublicKey(), nil)

	recipient, err := destination.FromHash(dstDest.GetHash(), dst, destination.Single, tr)
	if err != nil {
		t.Fatalf("recipient: %v", err)
	}

	msg, err := NewMessage(dstDest.GetHash(), srcDest.GetHash(), []byte("t"), []byte("body"), nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	if _, err := msg.Pack(src); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if err := msg.PackPropagated(recipient, 13); err != nil {
		t.Fatalf("PackPropagated: %v", err)
	}
	if len(msg.PropagationPacked) == 0 {
		t.Fatal("empty propagation_packed")
	}
	if len(msg.TransientID) != 32 {
		t.Fatalf("transient id len = %d", len(msg.TransientID))
	}
	if len(msg.PropagationStamp) != StampSize {
		t.Fatalf("propagation stamp len = %d", len(msg.PropagationStamp))
	}
}

func TestSendPropagated_NoPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping propagation path wait in short mode")
	}
	tr := transport.NewTransport(common.DefaultConfig())
	id, _ := identity.NewIdentity()
	m, err := NewDeliveryMessenger(id, tr)
	if err != nil {
		t.Fatalf("messenger: %v", err)
	}

	remote := make([]byte, DestinationLength)
	propNode := make([]byte, DestinationLength)
	for i := range remote {
		remote[i] = byte(i + 1)
	}
	propNode[0] = 0xab
	identity.Remember(nil, remote, id.GetPublicKey(), nil)

	msg, err := m.Compose(remote, "x", "y", nil)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	err = m.SendPropagated(msg, propNode, 16)
	if err == nil {
		t.Fatal("expected error without path to propagation node")
	}
}
