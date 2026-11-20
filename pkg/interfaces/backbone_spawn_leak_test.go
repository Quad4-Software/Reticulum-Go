// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"net"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/backbone"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
)

// A spawned client is admitted into bi.spawned before its stream attaches.
// When attach fails (hub closed, fd error) no stream exists so onClose never
// runs: the dead client keeps its client_limit slot forever.
func TestBackboneAttachFailureFreesClientSlot(t *testing.T) {
	hub := testBackboneHub(t)
	cfg := &common.InterfaceConfig{Enabled: true, Port: 1, ClientLimit: 1}
	bi, err := NewBackboneInterface("leak", cfg, hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	bi.fastFlapGrace = 0
	bi.fastFlapThreshold = 0
	bi.blockFastFlapping = false

	// Closing the hub makes RegisterStream (and so attachStream) fail.
	backbone.Shutdown()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	server, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}

	bi.acceptConn(server)
	if got := bi.Clients(); got != 0 {
		t.Fatalf("failed attach left %d spawned clients, leaking a client_limit slot", got)
	}
}
