// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"crypto/sha256"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/announce"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/health"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

// TestAnnounceLanesBypassBlockedWorkerPool proves announce jobs ride their
// dedicated lanes instead of the shared packet worker pool. With every
// packetQ worker parked, a valid announce must still verify and be
// remembered. Before the lanes existed it would shed on the full queue and
// never reach handleAnnouncePacket.
func TestAnnounceLanesBypassBlockedWorkerPool(t *testing.T) {
	health.Default.Reset()
	tr := NewTransport(&common.ReticulumConfig{
		EnableTransport:   true,
		MaxPacketHandlers: 4,
	})
	defer func() { _ = tr.Close() }()

	iface := &mockInterface{}
	iface.Name = "ann-lane"
	iface.Enabled = true
	iface.Online = true
	if err := tr.RegisterInterface("ann-lane", iface); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}

	hold := make(chan struct{})
	defer close(hold)
	if tr.occupyHandlerPoolForTest(hold) == 0 {
		t.Fatal("could not occupy handler pool")
	}

	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	destName := "ann.lane.test"
	nameHash := sha256.Sum256([]byte(destName))
	destInput := append(nameHash[:10], id.Hash()...)
	destFull := sha256.Sum256(destInput)
	destHash := destFull[:16]

	a, err := announce.New(id, destHash, destName, []byte(destName), false, &common.ReticulumConfig{})
	if err != nil {
		t.Fatalf("announce.New: %v", err)
	}
	annRaw, err := a.CreatePacket()
	if err != nil {
		t.Fatalf("announce.CreatePacket: %v", err)
	}

	tr.HandlePacket(annRaw, iface)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := identity.Recall(destHash); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("announce was not processed while the shared worker pool was blocked")
}
