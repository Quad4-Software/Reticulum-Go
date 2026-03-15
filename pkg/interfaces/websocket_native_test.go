package interfaces

import (
	"net"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
)

func TestWebSocketGUID(t *testing.T) {
	if wsGUID != "258EAFA5-E914-47DA-95CA-C5AB0DC85B11" {
		t.Errorf("wsGUID mismatch: expected RFC 6455 GUID, got %s", wsGUID)
	}
}

func TestGenerateWebSocketKey(t *testing.T) {
	key1, err := generateWebSocketKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	key2, err := generateWebSocketKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	if key1 == key2 {
		t.Error("Generated keys should be unique")
	}

	if len(key1) != 24 {
		t.Errorf("Expected base64-encoded key length 24, got %d", len(key1))
	}
}

func TestComputeAcceptKey(t *testing.T) {
	testKey := "dGhlIHNhbXBsZSBub25jZQ=="
	expectedAccept := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="

	accept := computeAcceptKey(testKey)
	if accept != expectedAccept {
		t.Errorf("Accept key mismatch: expected %s, got %s", expectedAccept, accept)
	}
}

func TestNewWebSocketInterface(t *testing.T) {
	ws, err := NewWebSocketInterface("test", "wss://socket.quad4.io/ws", true)
	if err != nil {
		t.Fatalf("Failed to create WebSocket interface: %v", err)
	}

	if ws.GetName() != "test" {
		t.Errorf("Expected name 'test', got %s", ws.GetName())
	}

	if ws.GetType() != common.IF_TYPE_UDP {
		t.Errorf("Expected type IF_TYPE_UDP, got %v", ws.GetType())
	}

	if ws.GetMTU() != 1064 {
		t.Errorf("Expected MTU 1064, got %d", ws.GetMTU())
	}

	if ws.IsOnline() {
		t.Error("Interface should not be online before Start()")
	}
}

func checkNetwork(t *testing.T) {
	conn, err := net.DialTimeout("tcp", "socket.quad4.io:443", 1*time.Second)
	if err != nil {
		t.Skip("Skipping network test: socket.quad4.io unreachable")
	}
	conn.Close()
}

func TestWebSocketConnection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	checkNetwork(t)

	ws, err := NewWebSocketInterface("test", "wss://socket.quad4.io/ws", true)
	if err != nil {
		t.Fatalf("Failed to create WebSocket interface: %v", err)
	}

	ws.SetPacketCallback(func(data []byte, ni common.NetworkInterface) {
		t.Logf("Received packet: %d bytes", len(data))
	})

	err = ws.Start()
	if err != nil {
		t.Fatalf("Failed to start WebSocket: %v", err)
	}

	time.Sleep(2 * time.Second)

	if !ws.IsOnline() {
		t.Error("WebSocket should be online after Start()")
	}

	testData := []byte{0x01, 0x02, 0x03, 0x04}
	err = ws.Send(testData, "")
	if err != nil {
		t.Errorf("Failed to send data: %v", err)
	}

	time.Sleep(1 * time.Second)

	if err := ws.Stop(); err != nil {
		t.Errorf("Failed to stop WebSocket: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	if ws.IsOnline() {
		t.Error("WebSocket should be offline after Stop()")
	}
}

func TestWebSocketReconnection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}
	checkNetwork(t)

	ws, err := NewWebSocketInterface("test", "wss://socket.quad4.io/ws", true)
	if err != nil {
		t.Fatalf("Failed to create WebSocket interface: %v", err)
	}

	err = ws.Start()
	if err != nil {
		t.Fatalf("Failed to start WebSocket: %v", err)
	}

	time.Sleep(1 * time.Second)

	if !ws.IsOnline() {
		t.Error("WebSocket should be online")
	}

	conn := ws.GetConn()
	if conn == nil {
		t.Error("GetConn() should return a connection")
	}

	conn.Close()

	time.Sleep(3 * time.Second)

	if ws.IsOnline() {
		t.Log("WebSocket reconnected successfully")
	}

	if err := ws.Stop(); err != nil {
		t.Errorf("Failed to stop WebSocket: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
}

func TestWebSocketMessageQueue(t *testing.T) {
	checkNetwork(t)
	ws, err := NewWebSocketInterface("test", "wss://socket.quad4.io/ws", true)
	if err != nil {
		t.Fatalf("Failed to create WebSocket interface: %v", err)
	}

	ws.Enable()

	testData := []byte{0x01, 0x02, 0x03}
	err = ws.Send(testData, "")
	if err != nil {
		t.Errorf("Send should queue message when offline, got error: %v", err)
	}

	if testing.Short() {
		return
	}

	err = ws.Start()
	if err != nil {
		t.Fatalf("Failed to start WebSocket: %v", err)
	}

	// Wait for interface to be online (up to 10 seconds)
	for i := 0; i < 100; i++ {
		if ws.IsOnline() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !ws.IsOnline() {
		t.Error("WebSocket should be online")
	}

	time.Sleep(2 * time.Second)

	if err := ws.Stop(); err != nil {
		t.Errorf("Failed to stop WebSocket: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
}

func TestWebSocketFrameEncoding(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping frame encoding test in short mode")
	}
	checkNetwork(t)

	ws, err := NewWebSocketInterface("test", "wss://socket.quad4.io/ws", true)
	if err != nil {
		t.Fatalf("Failed to create WebSocket interface: %v", err)
	}

	err = ws.Start()
	if err != nil {
		t.Fatalf("Failed to start WebSocket: %v", err)
	}

	time.Sleep(1 * time.Second)

	testCases := []struct {
		name string
		data []byte
	}{
		{"small frame", []byte{0x01, 0x02, 0x03}},
		{"medium frame", make([]byte, 200)},
		{"large frame", make([]byte, 1000)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ws.Send(tc.data, "")
			if err != nil {
				t.Errorf("Failed to send %s: %v", tc.name, err)
			}
			time.Sleep(100 * time.Millisecond)
		})
	}

	if err := ws.Stop(); err != nil {
		t.Errorf("Failed to stop WebSocket: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
}

func TestWebSocketEnableDisable(t *testing.T) {
	ws, err := NewWebSocketInterface("test", "wss://socket.quad4.io/ws", false)
	if err != nil {
		t.Fatalf("Failed to create WebSocket interface: %v", err)
	}

	if ws.IsEnabled() {
		t.Error("Interface should not be enabled initially")
	}

	ws.Enable()
	if !ws.IsEnabled() {
		t.Error("Interface should be enabled after Enable()")
	}

	ws.Disable()
	if ws.IsEnabled() {
		t.Error("Interface should not be enabled after Disable()")
	}
}

func TestWebSocketDetach(t *testing.T) {
	ws, err := NewWebSocketInterface("test", "wss://socket.quad4.io/ws", true)
	if err != nil {
		t.Fatalf("Failed to create WebSocket interface: %v", err)
	}

	if ws.IsDetached() {
		t.Error("Interface should not be detached initially")
	}

	ws.Detach()
	if !ws.IsDetached() {
		t.Error("Interface should be detached after Detach()")
	}

	if ws.IsOnline() {
		t.Error("Interface should be offline after Detach()")
	}
}
