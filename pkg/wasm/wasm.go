// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
//go:build js && wasm
// +build js,wasm

package wasm

import (
	"encoding/hex"
	"fmt"
	"syscall/js"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
	"git.quad4.io/Networks/Reticulum-Go/pkg/destination"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
	"git.quad4.io/Networks/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/Networks/Reticulum-Go/pkg/packet"
	"git.quad4.io/Networks/Reticulum-Go/pkg/transport"
)

var (
	reticulumTransport *transport.Transport
	reticulumDest      *destination.Destination
	reticulumIdentity  *identity.Identity
	stats              = struct {
		packetsSent       int
		packetsReceived   int
		bytesSent         int
		bytesReceived     int
		announcesSent     int
		announcesReceived int
	}{}
	packetCallback  js.Value
	announceHandler js.Value
)

// RegisterJSFunctions registers the Reticulum WASM API to the JavaScript global scope.
func RegisterJSFunctions() {
	js.Global().Set("reticulum", js.ValueOf(map[string]interface{}{
		"init":                js.FuncOf(InitReticulum),
		"getIdentity":         js.FuncOf(GetIdentity),
		"getDestination":      js.FuncOf(GetDestination),
		"connect":             js.FuncOf(ConnectWebSocket),
		"disconnect":          js.FuncOf(DisconnectWebSocket),
		"isConnected":         js.FuncOf(IsConnected),
		"requestPath":         js.FuncOf(RequestPath),
		"getStats":            js.FuncOf(GetStats),
		"setPacketCallback":   js.FuncOf(SetPacketCallback),
		"setAnnounceCallback": js.FuncOf(SetAnnounceCallback),
		"sendData":            js.FuncOf(SendDataJS),
		"sendMessage":         js.FuncOf(SendDataJS),
		"announce":            js.FuncOf(SendAnnounceJS),
	}))
}

func SetPacketCallback(this js.Value, args []js.Value) interface{} {
	if len(args) > 0 && args[0].Type() == js.TypeFunction {
		packetCallback = args[0]
		debug.Log(debug.DEBUG_INFO, "JS packet callback registered")
		return js.ValueOf(true)
	}
	debug.Log(debug.DEBUG_ERROR, "setPacketCallback called without a function argument", "argc", len(args))
	return js.ValueOf(false)
}

func SetAnnounceCallback(this js.Value, args []js.Value) interface{} {
	if len(args) > 0 && args[0].Type() == js.TypeFunction {
		announceHandler = args[0]
		debug.Log(debug.DEBUG_INFO, "JS announce callback registered")
		return js.ValueOf(true)
	}
	debug.Log(debug.DEBUG_ERROR, "setAnnounceCallback called without a function argument", "argc", len(args))
	return js.ValueOf(false)
}

func RequestPath(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return js.ValueOf(map[string]interface{}{
			"error": "Destination hash required",
		})
	}

	destHashHex := args[0].String()
	destHash, err := hex.DecodeString(destHashHex)
	if err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Invalid destination hash: %v", err),
		})
	}

	if reticulumTransport == nil {
		return js.ValueOf(map[string]interface{}{
			"error": "Reticulum not initialized",
		})
	}

	if err := reticulumTransport.RequestPath(destHash, "", nil, true); err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Failed to request path: %v", err),
		})
	}

	return js.ValueOf(map[string]interface{}{
		"success": true,
	})
}

func GetStats(this js.Value, args []js.Value) interface{} {
	if reticulumTransport != nil {
		ifaces := reticulumTransport.GetInterfaces()
		totalTxBytes := 0
		totalRxBytes := 0
		totalTxPackets := 0
		totalRxPackets := 0
		for _, iface := range ifaces {
			totalTxBytes += int(iface.GetTxBytes())
			totalRxBytes += int(iface.GetRxBytes())
			totalTxPackets += int(iface.GetTxPackets())
			totalRxPackets += int(iface.GetRxPackets())
		}
		stats.bytesSent = totalTxBytes
		stats.bytesReceived = totalRxBytes
		stats.packetsSent = totalTxPackets
		stats.packetsReceived = totalRxPackets
	}

	return js.ValueOf(map[string]interface{}{
		"packetsSent":       stats.packetsSent,
		"packetsReceived":   stats.packetsReceived,
		"bytesSent":         stats.bytesSent,
		"bytesReceived":     stats.bytesReceived,
		"announcesSent":     stats.announcesSent,
		"announcesReceived": stats.announcesReceived,
	})
}

func InitReticulum(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return js.ValueOf(map[string]interface{}{
			"error": "WebSocket URL required",
		})
	}

	if reticulumTransport != nil {
		reticulumTransport.Close()
		reticulumTransport = nil
	}

	wsURL := args[0].String()
	appName := "wasm_core"
	if len(args) >= 2 && args[1].Type() == js.TypeString {
		appName = args[1].String()
	}

	var id *identity.Identity
	var err error

	// Check for existing identity in args
	if len(args) >= 3 && !args[2].IsNull() && !args[2].IsUndefined() {
		idHex := args[2].String()
		idBytes, decodeErr := hex.DecodeString(idHex)
		if decodeErr == nil && len(idBytes) == 64 {
			id, err = identity.FromBytes(idBytes)
			if err != nil {
				debug.Log(debug.DEBUG_ERROR, "Failed to load provided identity, generating new one", "error", err)
				id, err = identity.NewIdentity()
			}
		} else {
			id, err = identity.NewIdentity()
		}
	} else {
		id, err = identity.NewIdentity()
	}

	if err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Failed to handle identity: %v", err),
		})
	}

	cfg := common.DefaultConfig()
	t := transport.NewTransport(cfg)
	// Ensure the global instance is set for internal transport calls (like Announce)
	transport.SetTransportInstance(t)

	// Set transport identity to the same as the node identity for now in WASM
	t.SetIdentity(id)
	if err := t.InitializePathRequestHandler(); err != nil {
		debug.Log(debug.DEBUG_ERROR, "Failed to initialize path request handler", "error", err)
	}

	dest, err := destination.New(
		id,
		destination.IN,
		destination.SINGLE,
		appName,
		t,
		"browser",
	)
	if err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Failed to create destination: %v", err),
		})
	}

	dest.SetPacketCallback(func(data []byte, ni common.NetworkInterface) {
		if packetCallback.IsUndefined() || packetCallback.IsNull() {
			debug.Log(debug.DEBUG_ERROR, "JS packet callback not registered; dropping packet", "bytes", len(data))
			return
		}
		if packetCallback.Type() != js.TypeFunction {
			debug.Log(debug.DEBUG_ERROR, "JS packet callback is not a function", "type", packetCallback.Type().String())
			return
		}
		defer func() {
			if r := recover(); r != nil {
				debug.Log(debug.DEBUG_CRITICAL, "JS packet callback panicked", "panic", fmt.Sprintf("%v", r))
			}
		}()
		uint8Array := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(uint8Array, data)
		packetCallback.Invoke(uint8Array)
	})

	dest.SetProofStrategy(destination.PROVE_ALL)

	t.RegisterAnnounceHandler(&genericAnnounceHandler{})

	wsInterface, err := interfaces.NewWebSocketInterface("wasm0", wsURL, true)
	if err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Failed to create WebSocket interface: %v", err),
		})
	}

	// Wire the interface to the transport
	wsInterface.SetPacketCallback(t.HandlePacket)

	if err := t.RegisterInterface("wasm0", wsInterface); err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Failed to register interface: %v", err),
		})
	}

	if err := t.Start(); err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Failed to start transport: %v", err),
		})
	}

	reticulumTransport = t
	reticulumDest = dest
	reticulumIdentity = id

	privHex := ""
	if pk, err := id.GetPrivateKey(); err == nil {
		privHex = hex.EncodeToString(pk)
	}
	return js.ValueOf(map[string]interface{}{
		"success":     true,
		"identity":    id.GetHexHash(),
		"privateKey":  privHex,
		"destination": fmt.Sprintf("%x", dest.GetHash()),
	})
}

// GetTransport returns the internal transport pointer.
func GetTransport() *transport.Transport {
	return reticulumTransport
}

// GetDestinationPointer returns the internal destination pointer.
func GetDestinationPointer() *destination.Destination {
	return reticulumDest
}

func GetIdentity(this js.Value, args []js.Value) interface{} {
	if reticulumIdentity == nil {
		return js.ValueOf(map[string]interface{}{
			"error": "Reticulum not initialized",
		})
	}

	return js.ValueOf(map[string]interface{}{
		"hash": reticulumIdentity.GetHexHash(),
	})
}

func GetDestination(this js.Value, args []js.Value) interface{} {
	if reticulumDest == nil {
		return js.ValueOf(map[string]interface{}{
			"error": "Reticulum not initialized",
		})
	}

	return js.ValueOf(map[string]interface{}{
		"hash": fmt.Sprintf("%x", reticulumDest.GetHash()),
	})
}

func IsConnected(this js.Value, args []js.Value) interface{} {
	if reticulumTransport == nil {
		return js.ValueOf(false)
	}

	ifaces := reticulumTransport.GetInterfaces()
	for _, iface := range ifaces {
		if iface.IsOnline() {
			return js.ValueOf(true)
		}
	}

	return js.ValueOf(false)
}

func ConnectWebSocket(this js.Value, args []js.Value) interface{} {
	if reticulumTransport == nil {
		return js.ValueOf(map[string]interface{}{
			"error": "Reticulum not initialized",
		})
	}

	ifaces := reticulumTransport.GetInterfaces()
	for name, iface := range ifaces {
		if iface.IsOnline() {
			return js.ValueOf(map[string]interface{}{
				"success":   true,
				"interface": name,
			})
		}
		if err := iface.Start(); err != nil {
			return js.ValueOf(map[string]interface{}{
				"error": fmt.Sprintf("Failed to connect: %v", err),
			})
		}
		return js.ValueOf(map[string]interface{}{
			"success":   true,
			"interface": name,
		})
	}

	return js.ValueOf(map[string]interface{}{
		"error": "WebSocket interface not found",
	})
}

func DisconnectWebSocket(this js.Value, args []js.Value) interface{} {
	if reticulumTransport == nil {
		return js.ValueOf(map[string]interface{}{
			"error": "Reticulum not initialized",
		})
	}

	ifaces := reticulumTransport.GetInterfaces()
	for _, iface := range ifaces {
		if err := iface.Stop(); err != nil {
			return js.ValueOf(map[string]interface{}{
				"error": fmt.Sprintf("Failed to stop interface: %v", err),
			})
		}
		return js.ValueOf(map[string]interface{}{
			"success": true,
		})
	}

	return js.ValueOf(map[string]interface{}{
		"error": "WebSocket interface not found",
	})
}

type genericAnnounceHandler struct{}

func (h *genericAnnounceHandler) AspectFilter() []string {
	return nil
}

func (h *genericAnnounceHandler) ReceivePathResponses() bool {
	return false
}

func (h *genericAnnounceHandler) ReceivedAnnounce(destHash []byte, ident interface{}, appData []byte, hops uint8) error {
	hashStr := hex.EncodeToString(destHash)
	debug.Log(debug.DEBUG_INFO, "WASM Announce Handler received announce", "dest", hashStr, "hops", hops)
	stats.announcesReceived++

	if announceHandler.IsUndefined() || announceHandler.IsNull() {
		debug.Log(debug.DEBUG_ERROR, "JS announce callback not registered; dropping announce on the floor", "dest", hashStr)
		return nil
	}
	if announceHandler.Type() != js.TypeFunction {
		debug.Log(debug.DEBUG_ERROR, "JS announce callback is not a function", "type", announceHandler.Type().String())
		return nil
	}

	defer func() {
		if r := recover(); r != nil {
			debug.Log(debug.DEBUG_CRITICAL, "JS announce callback panicked", "panic", fmt.Sprintf("%v", r))
		}
	}()

	debug.Log(debug.DEBUG_VERBOSE, "Invoking JS announce callback", "dest", hashStr, "appData_len", len(appData))
	announceHandler.Invoke(js.ValueOf(map[string]interface{}{
		"hash":    hashStr,
		"appData": string(appData),
		"hops":    int(hops),
	}))
	debug.Log(debug.DEBUG_VERBOSE, "JS announce callback completed", "dest", hashStr)
	return nil
}

// SendDataJS is the JS-facing wrapper for SendData
func SendDataJS(this js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return js.ValueOf(map[string]interface{}{
			"error": "Destination hash and data required",
		})
	}

	destHashHex := args[0].String()
	destHash, err := hex.DecodeString(destHashHex)
	if err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Invalid destination hash: %v", err),
		})
	}

	// Support both string and Uint8Array data from JS
	var data []byte
	if args[1].Type() == js.TypeString {
		data = []byte(args[1].String())
	} else {
		data = make([]byte, args[1].Length())
		js.CopyBytesToGo(data, args[1])
	}

	return SendData(destHash, data)
}

// SendData is a generic function to send raw bytes to a destination
func SendData(destHash []byte, data []byte) interface{} {
	if reticulumTransport == nil {
		return js.ValueOf(map[string]interface{}{
			"error": "Reticulum not initialized",
		})
	}

	remoteIdentity, err := identity.Recall(destHash)
	if err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Identity not found. Wait for an announce from this peer!"),
		})
	}

	targetDest, err := destination.FromHash(destHash, remoteIdentity, destination.SINGLE, reticulumTransport)
	if err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Failed to create target destination: %v", err),
		})
	}

	encrypted, err := targetDest.Encrypt(data)
	if err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Encryption failed: %v", err),
		})
	}

	pkt := packet.NewPacket(
		packet.DestinationSingle,
		encrypted,
		packet.PacketTypeData,
		packet.ContextNone,
		packet.PropagationBroadcast,
		packet.HeaderType1,
		nil,
		true,
		packet.FlagUnset,
	)
	pkt.DestinationHash = destHash

	if err := pkt.Pack(); err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Packet packing failed: %v", err),
		})
	}

	if err := reticulumTransport.SendPacket(pkt); err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Packet sending failed: %v", err),
		})
	}

	return js.ValueOf(map[string]interface{}{
		"success": true,
	})
}

// SendAnnounceJS is the JS-facing wrapper for SendAnnounce
func SendAnnounceJS(this js.Value, args []js.Value) interface{} {
	var appData []byte
	if len(args) >= 1 && args[0].Type() == js.TypeString {
		appData = []byte(args[0].String())
	} else if len(args) >= 1 && args[0].Type() == js.TypeObject {
		appData = make([]byte, args[0].Length())
		js.CopyBytesToGo(appData, args[0])
	}
	return SendAnnounce(appData)
}

// SendAnnounce is a generic function to send an announce
func SendAnnounce(appData []byte) interface{} {
	if reticulumDest == nil {
		return js.ValueOf(map[string]interface{}{
			"error": "Reticulum not initialized",
		})
	}

	if len(appData) > 0 {
		reticulumDest.SetDefaultAppData(appData)
	}

	if err := reticulumDest.Announce(false, nil, nil); err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Failed to send announce: %v", err),
		})
	}

	stats.announcesSent++

	return js.ValueOf(map[string]interface{}{
		"success": true,
	})
}
