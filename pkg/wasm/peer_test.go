//go:build js && wasm
// +build js,wasm

package wasm

import (
	"bytes"
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
	if bytes.Equal(destHash, h.expectedHash) {
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
	// Suppress library debug output to avoid concurrent syscall.fsCall
	// deadlocks in the single-threaded WASM JS runtime.
	debug.SetDebugLevel(0)

	tmpDirA, _ := os.MkdirTemp("", "reticulum_peerA")
	defer os.RemoveAll(tmpDirA)
	tmpDirB, _ := os.MkdirTemp("", "reticulum_peerB")
	defer os.RemoveAll(tmpDirB)

	wsURL := "wss://socket.quad4.io/ws"

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
	t.Logf("Peer A hash: %x", hashA)

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

	start := time.Now()
	for time.Since(start) < 15*time.Second {
		if wsA.IsOnline() && wsB.IsOnline() {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !wsA.IsOnline() || !wsB.IsOnline() {
		onA, onB := wsA.IsOnline(), wsB.IsOnline()
		wsA.Stop()
		wsB.Stop()
		trA.Close()
		trB.Close()
		t.Skipf("Skipping: WebSocket server unavailable (PeerA Online=%v, PeerB Online=%v)", onA, onB)
	}

	if err := destA.Announce(false, nil, nil); err != nil {
		t.Fatalf("Failed to send announce: %v", err)
	}

	select {
	case <-receivedChan:
		t.Log("Peer B received announce from Peer A")
	case <-time.After(30 * time.Second):
		t.Skip("Skipping: announce not relayed by external WebSocket server within timeout")
	}

	if !trB.HasPath(hashA) {
		t.Error("Peer B should have a path to Peer A after receiving announce")
	} else {
		t.Logf("Peer B confirmed path to Peer A, hops=%d", trB.HopsTo(hashA))
	}
}
