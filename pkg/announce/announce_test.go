package announce

import (
	"bytes"
	"sync"
	"testing"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
)

type mockAnnounceHandler struct {
	received bool
}

func (m *mockAnnounceHandler) AspectFilter() []string {
	return nil
}

func (m *mockAnnounceHandler) ReceivedAnnounce(destinationHash []byte, announcedIdentity interface{}, appData []byte, hops uint8) error {
	m.received = true
	return nil
}

func (m *mockAnnounceHandler) ReceivePathResponses() bool {
	return true
}

type mockInterface struct {
	common.BaseInterface
	sent bool
}

func (m *mockInterface) Send(data []byte, address string) error {
	m.sent = true
	return nil
}

func (m *mockInterface) GetBandwidthAvailable() bool {
	return true
}

func (m *mockInterface) IsEnabled() bool {
	return true
}

func TestNewAnnounce(t *testing.T) {
	id, _ := identity.New()
	destHash := make([]byte, 16)
	config := &common.ReticulumConfig{}

	ann, err := New(id, destHash, "testapp", []byte("appdata"), false, config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if ann == nil {
		t.Fatal("New returned nil")
	}

	if !bytes.Equal(ann.destinationHash, destHash) {
		t.Error("Destination hash doesn't match")
	}
}

func TestCreateAndHandleAnnounce(t *testing.T) {
	id, _ := identity.New()
	destHash := make([]byte, 16)
	config := &common.ReticulumConfig{}

	ann, _ := New(id, destHash, "testapp", []byte("appdata"), false, config)
	packet := ann.CreatePacket()

	handler := &mockAnnounceHandler{}
	ann.RegisterHandler(handler)

	err := ann.HandleAnnounce(packet)
	if err != nil {
		t.Fatalf("HandleAnnounce failed: %v", err)
	}

	if !handler.received {
		t.Error("Handler did not receive announce")
	}
}

func TestPropagate(t *testing.T) {
	id, _ := identity.New()
	destHash := make([]byte, 16)
	config := &common.ReticulumConfig{}

	ann, _ := New(id, destHash, "testapp", []byte("appdata"), false, config)

	iface := &mockInterface{}
	iface.Name = "testiface"
	iface.Online = true
	iface.Enabled = true

	err := ann.Propagate([]common.NetworkInterface{iface})
	if err != nil {
		t.Fatalf("Propagate failed: %v", err)
	}

	if !iface.sent {
		t.Error("Packet was not sent on interface")
	}
}

func TestHandlerRegistration(t *testing.T) {
	ann := &Announce{
		mutex: &sync.RWMutex{},
	}
	handler := &mockAnnounceHandler{}

	ann.RegisterHandler(handler)
	if len(ann.handlers) != 1 {
		t.Errorf("Expected 1 handler, got %d", len(ann.handlers))
	}

	ann.DeregisterHandler(handler)
	if len(ann.handlers) != 0 {
		t.Errorf("Expected 0 handlers, got %d", len(ann.handlers))
	}
}
