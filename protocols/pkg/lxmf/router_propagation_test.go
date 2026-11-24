// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"github.com/Quad4-Software/reticulum-go-protocols/internal/testconfig"
	"path/filepath"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

func TestPeerSkipsOwnPropagationHash(t *testing.T) {
	tr := transport.NewTransport(testconfig.New())
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tr.Close() }()

	id := mustNewIdentity(t)
	home := t.TempDir()
	router, err := NewRouter(id, tr, RouterOptions{
		Config:      DefaultConfig(),
		StoragePath: filepath.Join(home, "storage"),
		MessagesDir: filepath.Join(home, "storage", "messages"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := router.EnablePropagation(); err != nil {
		t.Fatal(err)
	}
	ownHash := router.PropagationDestination().GetHash()
	router.peer(ownHash, 1, 100, 1000, 16, 4, 12, nil)

	router.peersMu.RLock()
	count := len(router.peers)
	router.peersMu.RUnlock()
	if count != 0 {
		t.Fatalf("peers=%d want 0 for own propagation hash", count)
	}
}

func TestHandlePropagationPacketIgnoresEmptyMessages(t *testing.T) {
	tr := transport.NewTransport(testconfig.New())
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tr.Close() }()

	id := mustNewIdentity(t)
	home := t.TempDir()
	router, err := NewRouter(id, tr, RouterOptions{
		Config:      DefaultConfig(),
		StoragePath: filepath.Join(home, "storage"),
		MessagesDir: filepath.Join(home, "storage", "messages"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := router.EnablePropagation(); err != nil {
		t.Fatal(err)
	}

	payload, err := msgpack.Marshal([]any{float64(0), []any{}})
	if err != nil {
		t.Fatal(err)
	}
	router.handlePropagationPacket(payload, nil, nil)
	if router.clientPropagationReceived != 0 {
		t.Fatalf("clientPropagationReceived=%d want 0 for empty upload", router.clientPropagationReceived)
	}
}

func TestThrottlePeerRecordsCooldown(t *testing.T) {
	tr := transport.NewTransport(testconfig.New())
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tr.Close() }()

	id := mustNewIdentity(t)
	home := t.TempDir()
	router, err := NewRouter(id, tr, RouterOptions{
		Config:      DefaultConfig(),
		StoragePath: filepath.Join(home, "storage"),
		MessagesDir: filepath.Join(home, "storage", "messages"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := router.EnablePropagation(); err != nil {
		t.Fatal(err)
	}

	hash := bytesRepeat(0xab, DestinationLength)
	router.throttlePeer(hash)
	if !router.throttled(peerKey(hash)) {
		t.Fatal("expected peer to be throttled")
	}
}
