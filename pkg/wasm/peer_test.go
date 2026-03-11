//go:build js && wasm
// +build js,wasm

package wasm

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
	"git.quad4.io/Networks/Reticulum-Go/pkg/destination"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
	"git.quad4.io/Networks/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/Networks/Reticulum-Go/pkg/transport"
)

type testAnnounceHandler struct {
	received     chan bool
	expectedHash []byte
}

func (h *testAnnounceHandler) AspectFilter() []string {
	return nil
}

func (h *testAnnounceHandler) ReceivedAnnounce(destHash []byte, id interface{}, appData []byte, hops uint8) error {
	debug.Log(debug.DEBUG_INFO, "testAnnounceHandler received announce", "dest", fmt.Sprintf("%x", destHash), "hops", hops)
	if bytes.Equal(destHash, h.expectedHash) {
		debug.Log(debug.DEBUG_INFO, "testAnnounceHandler: matches expected hash!")
		select {
		case h.received <- true:
		default:
		}
	}
	return nil
}

func (h *testAnnounceHandler) ReceivePathResponses() bool {
	return false
}

func TestTwoPeersAnnounce(t *testing.T) {
	// Enable high debug level for the test
	debug.SetDebugLevel(debug.DEBUG_ALL)

	// Use temporary directories for each peer to avoid identity/storage collision
	tmpDirA, _ := os.MkdirTemp("", "reticulum_peerA")
	defer os.RemoveAll(tmpDirA)
	tmpDirB, _ := os.MkdirTemp("", "reticulum_peerB")
	defer os.RemoveAll(tmpDirB)

	wsURL := "wss://socket.quad4.io/ws"

	// Peer A setup (The Announcer)
	debug.Log(debug.DEBUG_INFO, "Setting up Peer A...")
	os.Setenv("RETICULUM_STORAGE_PATH", tmpDirA)
	idA, _ := identity.NewIdentity()
	cfgA := common.DefaultConfig()
	trA := transport.NewTransport(cfgA)
	trA.SetIdentity(idA)
	defer trA.Close()

	wsA, err := interfaces.NewWebSocketInterface("peerA", wsURL, true)
	if err != nil {
		t.Fatalf("Failed to create WS interface A: %v", err)
	}
	wsA.SetPacketCallback(trA.HandlePacket)
	trA.RegisterInterface("peerA", wsA)
	if err := wsA.Start(); err != nil {
		t.Fatalf("Failed to start interface A: %v", err)
	}

	destA, err := destination.New(idA, destination.IN, destination.SINGLE, "test_app_A", trA)
	if err != nil {
		t.Fatalf("Failed to create destination A: %v", err)
	}
	hashA := destA.GetHash()
	debug.Log(debug.DEBUG_INFO, "Peer A hash", "hash", fmt.Sprintf("%x", hashA))

	// Peer B setup (The Listener)
	debug.Log(debug.DEBUG_INFO, "Setting up Peer B...")
	os.Setenv("RETICULUM_STORAGE_PATH", tmpDirB)
	idB, _ := identity.NewIdentity()
	cfgB := common.DefaultConfig()
	trB := transport.NewTransport(cfgB)
	trB.SetIdentity(idB)
	defer trB.Close()

	wsB, err := interfaces.NewWebSocketInterface("peerB", wsURL, true)
	if err != nil {
		t.Fatalf("Failed to create WS interface B: %v", err)
	}
	wsB.SetPacketCallback(trB.HandlePacket)
	trB.RegisterInterface("peerB", wsB)
	if err := wsB.Start(); err != nil {
		t.Fatalf("Failed to start interface B: %v", err)
	}

	receivedChan := make(chan bool, 1)
	handler := &testAnnounceHandler{
		received:     receivedChan,
		expectedHash: hashA,
	}
	trB.RegisterAnnounceHandler(handler)

	// Wait for both connections to be online
	debug.Log(debug.DEBUG_INFO, "Waiting for WebSocket connections to be online...")
	start := time.Now()
	for time.Since(start) < 15*time.Second {
		if wsA.IsOnline() && wsB.IsOnline() {
			debug.Log(debug.DEBUG_INFO, "Both peers are online!")
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !wsA.IsOnline() || !wsB.IsOnline() {
		t.Fatalf("Peers failed to connect: PeerA Online=%v, PeerB Online=%v", wsA.IsOnline(), wsB.IsOnline())
	}

	// Peer A sends announce
	debug.Log(debug.DEBUG_INFO, "Peer A sending announce...")
	if err := destA.Announce(false, nil, nil); err != nil {
		t.Fatalf("Failed to send announce: %v", err)
	}

	// Peer B waits for announce
	debug.Log(debug.DEBUG_INFO, "Peer B waiting for announce...")
	select {
	case <-receivedChan:
		debug.Log(debug.DEBUG_INFO, "Peer B successfully received announce from Peer A!")
	case <-time.After(30 * time.Second):
		t.Fatal("Timed out waiting for Peer B to receive Peer A's announce")
	}

	// Verify Peer B now has a path to Peer A
	if !trB.HasPath(hashA) {
		t.Error("Peer B should have a path to Peer A after receiving announce")
	} else {
		debug.Log(debug.DEBUG_INFO, "Peer B confirmed path to Peer A", "hops", trB.HopsTo(hashA))
	}
}
