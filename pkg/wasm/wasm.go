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
	userName           string
	peerMap            = make(map[string]string)
	stats              = struct {
		packetsSent     int
		packetsReceived int
		bytesSent       int
		bytesReceived   int
	}{}
)

// RegisterJSFunctions registers the Reticulum WASM API to the JavaScript global scope.
func RegisterJSFunctions() {
	js.Global().Set("reticulum", js.ValueOf(map[string]interface{}{
		"init":           js.FuncOf(InitReticulum),
		"getIdentity":    js.FuncOf(GetIdentity),
		"getDestination": js.FuncOf(GetDestination),
		"announce":       js.FuncOf(SendAnnounce),
		"connect":        js.FuncOf(ConnectWebSocket),
		"disconnect":     js.FuncOf(DisconnectWebSocket),
		"isConnected":    js.FuncOf(IsConnected),
		"sendMessage":    js.FuncOf(SendMessage),
		"getStats":       js.FuncOf(GetStats),
	}))
}

func GetStats(this js.Value, args []js.Value) interface{} {
	return js.ValueOf(map[string]interface{}{
		"packetsSent":     stats.packetsSent,
		"packetsReceived": stats.packetsReceived,
		"bytesSent":       stats.bytesSent,
		"bytesReceived":   stats.bytesReceived,
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
	if len(args) >= 2 {
		userName = args[1].String()
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

	dest, err := destination.New(
		id,
		destination.IN,
		destination.SINGLE,
		"wasm_core",
		t,
		"browser",
	)
	if err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Failed to create destination: %v", err),
		})
	}

	if userName != "" {
		dest.SetDefaultAppData([]byte(userName))
	}

	dest.SetPacketCallback(func(data []byte, ni common.NetworkInterface) {
		stats.packetsReceived++
		stats.bytesReceived += len(data)

		var from string
		var text string
		if len(data) >= 16 {
			from = hex.EncodeToString(data[:16])
			text = string(data[16:])
		} else {
			from = ""
			text = string(data)
		}

		js.Global().Call("onChatMessage", js.ValueOf(map[string]interface{}{
			"text": text,
			"from": from,
		}))
	})

	dest.SetProofStrategy(destination.PROVE_ALL)

	t.RegisterAnnounceHandler(&announceHandler{})

	wsInterface, err := interfaces.NewWebSocketInterface("wasm0", wsURL, true)
	if err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Failed to create WebSocket interface: %v", err),
		})
	}

	wsInterface.SetPacketCallback(func(data []byte, ni common.NetworkInterface) {
		msg := fmt.Sprintf("Received packet: %d bytes (type: 0x%02x)", len(data), data[0])
		js.Global().Call("log", msg, "success")
		debug.Log(debug.DEBUG_INFO, "WASM received packet", "bytes", len(data), "type", fmt.Sprintf("0x%02x", data[0]))
		t.HandlePacket(data, ni)
	})

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

	return js.ValueOf(map[string]interface{}{
		"success":     true,
		"identity":    id.GetHexHash(),
		"privateKey":  hex.EncodeToString(id.GetPrivateKey()),
		"destination": fmt.Sprintf("%x", dest.GetHash()),
	})
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

func SendAnnounce(this js.Value, args []js.Value) interface{} {
	if reticulumDest == nil {
		return js.ValueOf(map[string]interface{}{
			"error": "Reticulum not initialized",
		})
	}

	var appData []byte
	if len(args) >= 1 && args[0].String() != "" {
		appData = []byte(args[0].String())
		userName = args[0].String()
	} else if userName != "" {
		appData = []byte(userName)
	}

	if len(appData) > 0 {
		reticulumDest.SetDefaultAppData(appData)
	}

	if err := reticulumDest.Announce(false, nil, nil); err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Failed to send announce: %v", err),
		})
	}

	return js.ValueOf(map[string]interface{}{
		"success": true,
	})
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
				"success": true,
				"interface": name,
			})
		}
		if err := iface.Start(); err != nil {
			return js.ValueOf(map[string]interface{}{
				"error": fmt.Sprintf("Failed to connect: %v", err),
			})
		}
		return js.ValueOf(map[string]interface{}{
			"success": true,
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

type announceHandler struct{}

func (h *announceHandler) AspectFilter() []string {
	return nil
}

func (h *announceHandler) ReceivePathResponses() bool {
	return false
}

func (h *announceHandler) ReceivedAnnounce(destHash []byte, ident interface{}, appData []byte) error {
	hashStr := hex.EncodeToString(destHash)
	peerMap[hashStr] = string(appData)
	js.Global().Call("onPeerDiscovered", js.ValueOf(map[string]interface{}{
		"hash":    hashStr,
		"appData": string(appData),
	}))
	return nil
}

func SendMessage(this js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return js.ValueOf(map[string]interface{}{
			"error": "Destination hash and message required",
		})
	}

	destHashHex := args[0].String()
	message := args[1].String()

	destHash, err := hex.DecodeString(destHashHex)
	if err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Invalid destination hash: %v", err),
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

	// Prepend sender hash to message
	senderHash := reticulumDest.GetHash()
	payload := append(senderHash, []byte(message)...)

	encrypted, err := targetDest.Encrypt(payload)
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

	stats.packetsSent++
	stats.bytesSent += len(message)

	return js.ValueOf(map[string]interface{}{
		"success": true,
	})
}

