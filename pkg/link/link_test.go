package link

import (
	"bytes"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/destination"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
	"git.quad4.io/Networks/Reticulum-Go/pkg/packet"
)

type mockTransport struct {
	sentPackets []*packet.Packet
}

func (m *mockTransport) SendPacket(pkt *packet.Packet) error {
	m.sentPackets = append(m.sentPackets, pkt)
	return nil
}

func (m *mockTransport) RegisterLink(linkID []byte, link any) {
}

func (m *mockTransport) GetConfig() *common.ReticulumConfig {
	return &common.ReticulumConfig{}
}

func (m *mockTransport) GetInterfaces() map[string]common.NetworkInterface {
	return make(map[string]common.NetworkInterface)
}

func (m *mockTransport) RegisterDestination(hash []byte, dest any) {
}

type mockInterface struct {
	name string
}

func (m *mockInterface) GetName() string {
	return m.name
}

func (m *mockInterface) Start() error {
	return nil
}

func (m *mockInterface) Stop() error {
	return nil
}

func (m *mockInterface) Send(data []byte, address string) error {
	return nil
}

func (m *mockInterface) ProcessIncoming(data []byte) error {
	return nil
}

func (m *mockInterface) SetPacketCallback(cb func([]byte, common.NetworkInterface)) {
}

func (m *mockInterface) GetType() string {
	return "mock"
}

func (m *mockInterface) GetMTU() int {
	return 500
}

func (m *mockInterface) Detach() {
}

func (m *mockInterface) Enable() {
}

func (m *mockInterface) Disable() {
}

func (m *mockInterface) IsEnabled() bool {
	return true
}

func (m *mockInterface) IsOnline() bool {
	return true
}

func (m *mockInterface) IsDetached() bool {
	return false
}

func (m *mockInterface) GetPacketCallback() func([]byte, common.NetworkInterface) {
	return nil
}

func (m *mockInterface) GetConn() any {
	return nil
}

func (m *mockInterface) ProcessOutgoing(data []byte) ([]byte, error) {
	return data, nil
}

func (m *mockInterface) SendPathRequest(destHash []byte) error {
	return nil
}

func (m *mockInterface) SendLinkPacket(data []byte) error {
	return nil
}

func (m *mockInterface) GetBandwidthAvailable() float64 {
	return 1.0
}

func TestLinkRequestResponse(t *testing.T) {
	serverIdent, err := identity.New()
	if err != nil {
		t.Fatalf("Failed to create server identity: %v", err)
	}

	clientIdent, err := identity.New()
	if err != nil {
		t.Fatalf("Failed to create client identity: %v", err)
	}

	mockTrans := &mockTransport{
		sentPackets: make([]*packet.Packet, 0),
	}

	serverDest, err := destination.New(serverIdent, destination.IN, destination.SINGLE, "testapp", mockTrans, "server")
	if err != nil {
		t.Fatalf("Failed to create server destination: %v", err)
	}

	expectedResponse := []byte("response data")
	testPath := "/test/path"

	err = serverDest.RegisterRequestHandler(testPath, func(path string, data []byte, requestID []byte, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) []byte {
		if path != testPath {
			t.Errorf("Expected path %s, got %s", testPath, path)
		}
		return expectedResponse
	}, destination.ALLOW_ALL, nil)
	if err != nil {
		t.Fatalf("Failed to register request handler: %v", err)
	}

	// Test the handler is registered correctly
	pathHash := identity.TruncatedHash([]byte(testPath))
	handler := serverDest.GetRequestHandler(pathHash)
	if handler == nil {
		t.Fatal("Handler not found after registration")
	}

	// Call the handler
	testLinkID := make([]byte, 16)
	result := handler(pathHash, []byte("test data"), []byte("request-id"), testLinkID, clientIdent, time.Now())

	if result == nil {
		t.Fatal("Handler returned nil")
	}

	responseBytes, ok := result.([]byte)
	if !ok {
		t.Fatalf("Handler returned unexpected type: %T", result)
	}

	if !bytes.Equal(responseBytes, expectedResponse) {
		t.Errorf("Expected response %q, got %q", expectedResponse, responseBytes)
	}
}

func TestLinkRequestHandlerNotFound(t *testing.T) {
	serverIdent, _ := identity.New()
	mockTrans := &mockTransport{sentPackets: make([]*packet.Packet, 0)}

	serverDest, _ := destination.New(serverIdent, destination.IN, destination.SINGLE, "testapp", mockTrans, "server")

	nonExistentPath := "/does/not/exist"
	pathHash := identity.TruncatedHash([]byte(nonExistentPath))

	handler := serverDest.GetRequestHandler(pathHash)
	if handler != nil {
		t.Error("Expected no handler for non-existent path, but found one")
	}
}

func TestLinkResponseHandling(t *testing.T) {
	// This test verifies the basic structure for response handling
	// Full integration testing would require a proper transport setup

	requestID := []byte("test-request-id-")
	responseData := []byte("response payload")

	receipt := &RequestReceipt{
		requestID: requestID,
		status:    STATUS_PENDING,
	}

	// Verify initial state
	if receipt.status != STATUS_PENDING {
		t.Errorf("Expected initial status PENDING, got %d", receipt.status)
	}

	// Simulate setting response
	receipt.response = responseData
	receipt.status = STATUS_ACTIVE

	if !bytes.Equal(receipt.response, responseData) {
		t.Errorf("Expected response %q, got %q", responseData, receipt.response)
	}

	if receipt.status != STATUS_ACTIVE {
		t.Errorf("Expected status ACTIVE after response, got %d", receipt.status)
	}
}
