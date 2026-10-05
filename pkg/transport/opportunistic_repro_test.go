// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
)

// TestOpportunisticDataPacketDelivery reproduces the plain data-packet drop:
// an encrypted DestinationSingle data packet sent over a real interface must
// reach the registered destination's packet callback.
func TestOpportunisticDataPacketDelivery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TCP loopback test in -short mode")
	}
	requireTCPPlumbing(t)

	port, err := pickFreeTCPPort()
	if err != nil {
		t.Fatalf("pick free port: %v", err)
	}

	server, err := interfaces.NewTCPServerInterface("opp-server", "127.0.0.1", port, false, false, false)
	if err != nil {
		t.Fatalf("server interface: %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("server start: %v", err)
	}
	defer server.Stop() // #nosec G104

	srvTr := NewTransport(&common.ReticulumConfig{})
	defer srvTr.Close()
	if err := srvTr.RegisterInterface(server.GetName(), server); err != nil {
		t.Fatalf("register server iface: %v", err)
	}

	// Destination with identity + packet callback on the server.
	srvID, _ := identity.New()
	srvDest, err := destination.New(srvID, destination.In, destination.Single, "testapp", srvTr, "test")
	if err != nil {
		t.Fatalf("server dest: %v", err)
	}
	got := make(chan []byte, 4)
	srvDest.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got <- data
	})

	client, err := interfaces.NewTCPClientInterface("opp-client", "127.0.0.1", port, false, false, true)
	if err != nil {
		t.Fatalf("client interface: %v", err)
	}
	defer client.Stop() // #nosec G104

	cliTr := NewTransport(&common.ReticulumConfig{})
	defer cliTr.Close()
	if err := cliTr.RegisterInterface(client.GetName(), client); err != nil {
		t.Fatalf("register client iface: %v", err)
	}

	if err := waitForServerConnection(server, 2*time.Second); err != nil {
		t.Fatalf("server never accepted connection: %v", err)
	}

	// Inject a path and identity for the server dest, like an announce would.
	destHash := srvDest.GetHash()
	cliTr.UpdatePath(destHash, srvID.Hash(), client.GetName(), 1)
	identity.Remember(nil, destHash, srvID.GetPublicKey(), nil)

	remote, err := identity.Recall(destHash)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	target, err := destination.FromHash(destHash, remote, destination.Single, cliTr)
	if err != nil {
		t.Fatalf("fromhash: %v", err)
	}
	plaintext := []byte("opportunistic payload")
	enc, err := target.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	pkt := packet.NewPacket(
		packet.DestinationSingle, enc, packet.PacketTypeData,
		packet.ContextNone, packet.PropagationBroadcast,
		packet.HeaderType1, nil, false, packet.FlagUnset,
	)
	pkt.DestinationHash = destHash
	if err := pkt.Pack(); err != nil {
		t.Fatalf("pack: %v", err)
	}
	if err := cliTr.SendPacket(pkt); err != nil {
		t.Fatalf("send: %v", err)
	}

	select {
	case data := <-got:
		if string(data) != string(plaintext) {
			t.Fatalf("payload mismatch: %q", data)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("data packet never reached destination callback")
	}
}
