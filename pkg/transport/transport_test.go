package transport

import (
	"bytes"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/destination"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
	"git.quad4.io/Networks/Reticulum-Go/pkg/packet"
)

type mockInterface struct {
	common.BaseInterface
	sent [][]byte
}

func (m *mockInterface) Send(data []byte, address string) error {
	m.sent = append(m.sent, data)
	return nil
}

func (m *mockInterface) GetName() string {
	return m.Name
}

func (m *mockInterface) IsEnabled() bool {
	return m.Enabled
}

func TestNewTransport(t *testing.T) {
	config := &common.ReticulumConfig{}
	tr := NewTransport(config)
	if tr == nil {
		t.Fatal("NewTransport returned nil")
	}
	defer tr.Close()
}

func TestRegisterInterface(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	iface := &mockInterface{}
	iface.Name = "test"
	err := tr.RegisterInterface("test", iface)
	if err != nil {
		t.Fatalf("RegisterInterface failed: %v", err)
	}

	retrieved, err := tr.GetInterface("test")
	if err != nil {
		t.Fatalf("GetInterface failed: %v", err)
	}
	if retrieved != iface {
		t.Error("Retrieved interface doesn't match")
	}
}

func TestPathManagement(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	destHash := []byte("test-destination-hash")
	nextHop := []byte("next-hop")
	iface := &mockInterface{}
	iface.Name = "iface1"
	_ = tr.RegisterInterface("iface1", iface)

	tr.UpdatePath(destHash, nextHop, "iface1", 2)

	if !tr.HasPath(destHash) {
		t.Error("Path not found after update")
	}

	if tr.HopsTo(destHash) != 2 {
		t.Errorf("Expected 2 hops, got %d", tr.HopsTo(destHash))
	}

	if !bytes.Equal(tr.NextHop(destHash), nextHop) {
		t.Error("Next hop mismatch")
	}

	if tr.NextHopInterface(destHash) != "iface1" {
		t.Errorf("Expected iface1, got %s", tr.NextHopInterface(destHash))
	}
}

func TestDestinationRegistration(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	destHash := []byte("dest")
	tr.RegisterDestination(destHash, "test-dest")

	tr.mutex.RLock()
	dest, ok := tr.destinations[string(destHash)]
	tr.mutex.RUnlock()

	if !ok || dest != "test-dest" {
		t.Error("Destination not registered correctly")
	}
}

func TestTransportStatus(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	destHash := []byte("dest")
	if tr.PathIsUnresponsive(destHash) {
		t.Error("Path should not be unresponsive initially")
	}

	tr.MarkPathUnresponsive(destHash)
	if !tr.PathIsUnresponsive(destHash) {
		t.Error("Path should be unresponsive")
	}

	tr.MarkPathResponsive(destHash)
	if tr.PathIsUnresponsive(destHash) {
		t.Error("Path should be responsive again")
	}
}

func TestAnnounceHopCount(t *testing.T) {
	config := common.DefaultConfig()
	tr := NewTransport(config)
	defer tr.Close()

	iface := &mockInterface{}
	iface.Name = "wasm0"
	iface.Enabled = true
	_ = tr.RegisterInterface("wasm0", iface)

	// Create an identity for the announce
	id, _ := identity.New()

	// Create a destination to get a valid hash for this identity
	// NewAnnouncePacket uses "reticulum-go.node" by default
	dest, _ := destination.New(id, destination.In, destination.Single, "reticulum-go.node", tr)
	destHash := dest.GetHash()

	// Create a raw announce packet manually to control hop count
	// Header(2) + DestHash(16) + Context(1) + Payload...
	// Header: 0x21 (Announce, Header Type 1, Broadcast, Destination Type Single)
	// Hop count: 0
	raw := make([]byte, 2+16+1+148) // header + dest + context + min_announce_payload
	raw[0] = 0x21
	raw[1] = 0 // Initial hop count
	copy(raw[2:18], destHash)
	raw[18] = 0 // context

	// Announce payload: pubKey(64) + nameHash(10) + randomHash(10) + signature(64)
	payload := raw[19:]
	copy(payload[0:64], id.GetPublicKey())
	// Name hash, random hash, signature - filling with dummy data but valid length
	// Normally we would sign it properly, but handleAnnouncePacket validates it.
	// Actually, handleAnnouncePacket WILL fail if signature is invalid.
	// Use NewAnnouncePacket to get a valid signed packet
	transportID := make([]byte, 16)
	annPkt, err := packet.NewAnnouncePacket(destHash, id, []byte("test"), transportID)
	if err != nil {
		t.Fatalf("NewAnnouncePacket failed: %v", err)
	}
	annRaw, err := annPkt.Serialize()
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	// Override hop count to 0 to simulate neighbor
	annRaw[1] = 0

	// Handle the packet
	tr.HandlePacket(annRaw, iface)

	// Wait a bit for the async processing
	time.Sleep(100 * time.Millisecond)

	// Check stored hops
	if !tr.HasPath(destHash) {
		t.Fatal("Path not registered from announce")
	}

	hops := tr.HopsTo(destHash)
	if hops != 1 {
		t.Errorf("Expected 1 hop for neighbor (received 0), got %d", hops)
	}
}
