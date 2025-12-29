package transport

import (
	"bytes"
	"testing"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
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
