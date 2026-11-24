// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"crypto/sha256"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/internal/testconfig"
	"path/filepath"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

func TestPeerSeedSyncQueuesStoreMessages(t *testing.T) {
	dir := t.TempDir()
	ms, err := NewMessageStore(filepath.Join(dir, "propagation"), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	dest := make([]byte, DestinationLength)
	copy(dest, []byte("desthash12345678"))
	lxm := append(append([]byte(nil), dest...), []byte("payload")...)
	stamp := make([]byte, StampSize)
	if _, err := ms.Add(lxm, stamp, 16); err != nil {
		t.Fatal(err)
	}

	tr := transport.NewTransport(testconfig.New())
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Close() })

	id := mustNewIdentity(t)
	cfg := DefaultConfig()
	cfg.Propagation.PropagationStampCostTarget = 16
	cfg.Propagation.PeeringCost = 16
	router, err := NewRouter(id, tr, RouterOptions{
		Config:      cfg,
		StoragePath: dir,
		MessagesDir: filepath.Join(dir, "messages"),
	})
	if err != nil {
		t.Fatal(err)
	}
	router.store = ms
	t.Cleanup(router.Stop)

	peerDest := make([]byte, DestinationLength)
	copy(peerDest, []byte("peerdesthash1234"))
	peer := newPeer(router, peerDest)
	router.peersMu.Lock()
	router.peers[peerKey(peerDest)] = peer
	router.peersMu.Unlock()

	if queued := router.queuePeerStoreSync(peer); queued != 1 {
		t.Fatalf("queued=%d want 1", queued)
	}
	if peer.UnhandledCount(router) != 1 {
		t.Fatalf("unhandled=%d want 1", peer.UnhandledCount(router))
	}
	if !peer.needsSync(router) {
		t.Fatal("peer should need sync")
	}
}

func TestPeerNeedsSyncFalseWhenHandled(t *testing.T) {
	dir := t.TempDir()
	ms, err := NewMessageStore(filepath.Join(dir, "propagation"), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	dest := make([]byte, DestinationLength)
	copy(dest, []byte("desthash12345678"))
	lxm := append(append([]byte(nil), dest...), []byte("payload")...)
	stamp := make([]byte, StampSize)
	if _, err := ms.Add(lxm, stamp, 16); err != nil {
		t.Fatal(err)
	}
	tid := sha256.Sum256(lxm)

	tr := transport.NewTransport(testconfig.New())
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Close() })

	id := mustNewIdentity(t)
	router, err := NewRouter(id, tr, RouterOptions{
		Config:      DefaultConfig(),
		StoragePath: dir,
		MessagesDir: filepath.Join(dir, "messages"),
	})
	if err != nil {
		t.Fatal(err)
	}
	router.store = ms
	t.Cleanup(router.Stop)

	peerDest := make([]byte, DestinationLength)
	copy(peerDest, []byte("peerdesthash1234"))
	peer := newPeer(router, peerDest)
	peer.LastSyncAttempt = float64(time.Now().Unix())
	ms.markHandled(tid[:], peerDest)

	if peer.needsSync(router) {
		t.Fatal("handled peer should not need sync")
	}
}

func TestPeerNeedsSyncTrueForUnhandledStoreEntry(t *testing.T) {
	dir := t.TempDir()
	ms, err := NewMessageStore(filepath.Join(dir, "propagation"), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	dest := make([]byte, DestinationLength)
	copy(dest, []byte("desthash12345678"))
	lxm := append(append([]byte(nil), dest...), []byte("payload")...)
	stamp := make([]byte, StampSize)
	if _, err := ms.Add(lxm, stamp, 16); err != nil {
		t.Fatal(err)
	}

	tr := transport.NewTransport(testconfig.New())
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Close() })

	id := mustNewIdentity(t)
	router, err := NewRouter(id, tr, RouterOptions{
		Config:      DefaultConfig(),
		StoragePath: dir,
		MessagesDir: filepath.Join(dir, "messages"),
	})
	if err != nil {
		t.Fatal(err)
	}
	router.store = ms
	t.Cleanup(router.Stop)

	peerDest := make([]byte, DestinationLength)
	copy(peerDest, []byte("peerdesthash1234"))
	peer := newPeer(router, peerDest)
	peer.LastSyncAttempt = float64(time.Now().Unix())

	if !peer.needsSync(router) {
		t.Fatal("unhandled store entry should require sync")
	}
}
