//go:build js && wasm
// +build js,wasm

package wasm

import (
	"encoding/hex"
	"fmt"
	"syscall/js"
	"testing"

	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
)

func TestRegisterJSFunctions(t *testing.T) {
	RegisterJSFunctions()

	reticulum := js.Global().Get("reticulum")
	if reticulum.IsUndefined() {
		t.Fatal("reticulum object not registered in global scope")
	}

	functions := []string{
		"init", "getIdentity", "getDestination", "announce",
		"connect", "disconnect", "isConnected", "requestPath", "getStats",
		"setPacketCallback", "setAnnounceCallback", "sendData",
	}

	for _, fn := range functions {
		if reticulum.Get(fn).Type() != js.TypeFunction {
			t.Errorf("function %s not registered or not a function", fn)
		}
	}
}

func TestGetStats(t *testing.T) {
	// Reset stats
	stats.packetsSent = 10
	stats.packetsReceived = 5
	stats.bytesSent = 100
	stats.bytesReceived = 50

	result := GetStats(js.Undefined(), nil)
	val := result.(js.Value)

	if val.Get("packetsSent").Int() != 10 {
		t.Errorf("expected packetsSent 10, got %d", val.Get("packetsSent").Int())
	}
	if val.Get("packetsReceived").Int() != 5 {
		t.Errorf("expected packetsReceived 5, got %d", val.Get("packetsReceived").Int())
	}
}

func TestIsConnected(t *testing.T) {
	reticulumTransport = nil
	connected := IsConnected(js.Undefined(), nil).(js.Value).Bool()
	if connected {
		t.Error("expected connected to be false when transport is nil")
	}
}

func TestInitReticulum(t *testing.T) {
	// Mock JS global functions
	js.Global().Set("log", js.FuncOf(func(this js.Value, args []js.Value) interface{} { return nil }))

	// Test without arguments
	result := InitReticulum(js.Undefined(), []js.Value{})
	val := result.(js.Value)
	if val.Get("error").IsUndefined() || val.Get("error").String() != "WebSocket URL required" {
		t.Errorf("expected error 'WebSocket URL required', got %v", val.Get("error"))
	}

	// Test with valid URL and app name
	wsURL := "ws://localhost:8080"
	appName := "test_app"
	result = InitReticulum(js.Undefined(), []js.Value{js.ValueOf(wsURL), js.ValueOf(appName)})
	val = result.(js.Value)

	if !val.Get("success").Bool() {
		t.Errorf("InitReticulum failed: %v", val.Get("error"))
	}

	if reticulumIdentity == nil {
		t.Fatal("reticulumIdentity should not be nil after successful init")
	}

	// Test with provided identity
	id, _ := identity.NewIdentity()
	idHex := id.GetHexHash()
	// InitReticulum expects the FULL identity bytes in hex (64 bytes).
	idBytes, err := id.GetPrivateKey()
	if err != nil {
		t.Fatalf("GetPrivateKey: %v", err)
	}
	idHexFull := hex.EncodeToString(idBytes)

	result = InitReticulum(js.Undefined(), []js.Value{js.ValueOf(wsURL), js.ValueOf(appName), js.ValueOf(idHexFull)})
	val = result.(js.Value)

	if !val.Get("success").Bool() {
		t.Errorf("InitReticulum with identity failed: %v", val.Get("error"))
	}

	if reticulumIdentity.GetHexHash() != idHex {
		t.Errorf("expected identity hash %s, got %s", idHex, reticulumIdentity.GetHexHash())
	}
}

func TestIdentityAndDestination(t *testing.T) {
	// Ensure initialized
	js.Global().Set("log", js.FuncOf(func(this js.Value, args []js.Value) interface{} { return nil }))
	InitReticulum(js.Undefined(), []js.Value{js.ValueOf("ws://localhost")})

	idResult := GetIdentity(js.Undefined(), nil).(js.Value)
	if idResult.Get("hash").String() != reticulumIdentity.GetHexHash() {
		t.Error("GetIdentity returned wrong hash")
	}

	destResult := GetDestination(js.Undefined(), nil).(js.Value)
	expectedDest := fmt.Sprintf("%x", reticulumDest.GetHash())
	if destResult.Get("hash").String() != expectedDest {
		t.Errorf("GetDestination returned wrong hash, expected %s got %s", expectedDest, destResult.Get("hash").String())
	}
}

func TestSendDataJS(t *testing.T) {
	// Ensure initialized
	InitReticulum(js.Undefined(), []js.Value{js.ValueOf("ws://localhost")})

	// Create a mock peer
	peerId, _ := identity.NewIdentity()
	peerHash := peerId.Hash()
	peerHashHex := hex.EncodeToString(peerHash)

	// Manually add to known destinations so Recall works
	identity.Remember([]byte("mock_packet"), peerHash, peerId.GetPublicKey(), []byte("peer_app_data"))

	// Test SendDataJS with string
	data := "Hello Peer!"
	result := SendDataJS(js.Undefined(), []js.Value{js.ValueOf(peerHashHex), js.ValueOf(data)}).(js.Value)

	if !result.Get("error").IsUndefined() {
		errStr := result.Get("error").String()
		if errStr != "Packet sending failed: no path to destination" {
			t.Errorf("SendDataJS failed with unexpected error: %s", errStr)
		}
	} else if !result.Get("success").Bool() {
		t.Errorf("SendDataJS failed without error message")
	}
}
