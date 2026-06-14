// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build js && wasm

package interfaces

import (
	"fmt"
	"net"
	"syscall/js"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

type WebSocketInterface struct {
	BaseInterface
	wsURL        string
	ws           js.Value
	connected    bool
	messageQueue [][]byte

	// Keep references to JS callbacks to prevent them from being garbage collected
	onOpenFunc    js.Func
	onMessageFunc js.Func
	onErrorFunc   js.Func
	onCloseFunc   js.Func
}

func NewWebSocketInterface(name string, wsURL string, enabled bool) (*WebSocketInterface, error) {
	ws := &WebSocketInterface{
		BaseInterface: NewBaseInterface(name, common.IFTypeUDP, enabled),
		wsURL:         wsURL,
		messageQueue:  make([][]byte, 0),
	}

	ws.MTU = WSMTU
	ws.Bitrate = WSBitrate

	return ws, nil
}

func (wsi *WebSocketInterface) GetName() string {
	return wsi.Name
}

func (wsi *WebSocketInterface) GetType() common.InterfaceType {
	return wsi.Type
}

func (wsi *WebSocketInterface) GetMode() common.InterfaceMode {
	return wsi.Mode
}

func (wsi *WebSocketInterface) IsOnline() bool {
	wsi.Mutex.RLock()
	defer wsi.Mutex.RUnlock()
	return wsi.Online && wsi.connected
}

func (wsi *WebSocketInterface) IsDetached() bool {
	wsi.Mutex.RLock()
	defer wsi.Mutex.RUnlock()
	return wsi.Detached
}

func (wsi *WebSocketInterface) Detach() {
	wsi.Mutex.Lock()
	defer wsi.Mutex.Unlock()
	wsi.Detached = true
	wsi.Online = false
	wsi.closeWebSocket()
}

func (wsi *WebSocketInterface) Enable() {
	wsi.Mutex.Lock()
	defer wsi.Mutex.Unlock()
	wsi.Enabled = true
}

func (wsi *WebSocketInterface) Disable() {
	wsi.Mutex.Lock()
	defer wsi.Mutex.Unlock()
	wsi.Enabled = false
	wsi.closeWebSocket()
}

func (wsi *WebSocketInterface) Start() error {
	wsi.Mutex.Lock()
	defer wsi.Mutex.Unlock()

	if wsi.ws.Truthy() {
		readyState := wsi.ws.Get("readyState").Int()
		if readyState == 1 { // OPEN
			return nil
		}
		// If connecting, closing or closed, clean up first
		wsi.closeWebSocket()
	}

	// Ensure old callbacks are released before creating new ones
	wsi.releaseCallbacks()

	ws := js.Global().Get("WebSocket").New(wsi.wsURL)
	ws.Set("binaryType", "arraybuffer")

	wsi.onOpenFunc = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		wsi.Mutex.Lock()
		wsi.connected = true
		wsi.Online = true
		wsi.Mutex.Unlock()

		debug.Log(debug.DebugInfo, "WebSocket connected", "name", wsi.Name, "url", wsi.wsURL)

		wsi.Mutex.Lock()
		queue := make([][]byte, len(wsi.messageQueue))
		copy(queue, wsi.messageQueue)
		wsi.messageQueue = wsi.messageQueue[:0]
		wsi.Mutex.Unlock()

		for _, msg := range queue {
			wsi.sendWebSocketMessage(msg)
		}

		return nil
	})
	ws.Set("onopen", wsi.onOpenFunc)

	wsi.onMessageFunc = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}

		event := args[0]
		data := event.Get("data")

		handlePacket := func(buf js.Value) {
			array := js.Global().Get("Uint8Array").New(buf)
			length := array.Get("length").Int()
			if length < 1 {
				return
			}
			packet := make([]byte, length)
			js.CopyBytesToGo(packet, array)
			debug.Log(debug.DebugVerbose, "WASM WebSocket received binary data", "name", wsi.Name, "length", length, "first_byte", fmt.Sprintf("0x%02x", packet[0]))
			wsi.ProcessIncoming(packet)
		}

		if data.Type() == js.TypeString {
			packet := []byte(data.String())
			debug.Log(debug.DebugTrace, "WebSocket received string data", "name", wsi.Name, "length", len(packet))
			wsi.ProcessIncoming(packet)
		} else if data.InstanceOf(js.Global().Get("ArrayBuffer")) {
			handlePacket(data)
		} else if data.InstanceOf(js.Global().Get("Blob")) {
			// Handle Blob by converting to ArrayBuffer
			data.Call("arrayBuffer").Call("then", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				if len(args) > 0 {
					handlePacket(args[0])
				}
				return nil
			}))
		} else if data.Type() == js.TypeObject {
			// Fallback for other object types that might be TypedArrays
			handlePacket(data)
		} else {
			debug.Log(debug.DebugError, "Unknown WebSocket message type", "type", data.Type().String())
		}

		return nil
	})
	ws.Set("onmessage", wsi.onMessageFunc)

	wsi.onErrorFunc = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		debug.Log(debug.DebugError, "WebSocket error", "name", wsi.Name)
		return nil
	})
	ws.Set("onerror", wsi.onErrorFunc)

	wsi.onCloseFunc = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		wsi.Mutex.Lock()
		wsi.connected = false
		wsi.Online = false
		wsi.Mutex.Unlock()

		debug.Log(debug.DebugInfo, "WebSocket closed", "name", wsi.Name)

		wsi.releaseCallbacks()

		if wsi.Enabled && !wsi.Detached {
			go func() {
				time.Sleep(WSReconnectDelay)
				_ = wsi.Start()
			}()
		}

		return nil
	})
	ws.Set("onclose", wsi.onCloseFunc)

	wsi.ws = ws

	return nil
}

func (wsi *WebSocketInterface) Stop() error {
	wsi.Mutex.Lock()
	defer wsi.Mutex.Unlock()
	wsi.Enabled = false
	wsi.closeWebSocket()
	return nil
}

func (wsi *WebSocketInterface) closeWebSocket() {
	if wsi.ws.Truthy() {
		wsi.ws.Call("close")
		wsi.ws = js.Value{}
	}

	wsi.releaseCallbacks()

	wsi.connected = false
	wsi.Online = false
}

// Send routes through the concrete ProcessOutgoing; without this
// override, the embedded BaseInterface.Send dispatches to its own
// abstract ProcessOutgoing stub.
func (wsi *WebSocketInterface) Send(data []byte, _ string) error {
	wsi.Mutex.RLock()
	enabled := wsi.Enabled
	detached := wsi.Detached
	wsi.Mutex.RUnlock()
	if !enabled || detached {
		return fmt.Errorf("interface not enabled")
	}
	wsi.Mutex.Lock()
	wsi.TxBytes += uint64(len(data))
	wsi.TxPackets++
	wsi.Mutex.Unlock()
	return wsi.ProcessOutgoing(data)
}

func (wsi *WebSocketInterface) ProcessOutgoing(data []byte) error {
	if !wsi.connected {
		wsi.Mutex.Lock()
		wsi.messageQueue = append(wsi.messageQueue, data)
		wsi.Mutex.Unlock()
		return nil
	}

	return wsi.sendWebSocketMessage(data)
}

func (wsi *WebSocketInterface) sendWebSocketMessage(data []byte) error {
	if !wsi.ws.Truthy() {
		return fmt.Errorf("WebSocket not initialized")
	}

	if wsi.ws.Get("readyState").Int() != 1 {
		return fmt.Errorf("WebSocket not open")
	}

	array := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(array, data)

	wsi.ws.Call("send", array)

	debug.Log(debug.DebugVerbose, "WebSocket sent packet", "name", wsi.Name, "bytes", len(data))
	return nil
}

func (wsi *WebSocketInterface) GetConn() net.Conn {
	return nil
}

func (wsi *WebSocketInterface) GetMTU() int {
	return wsi.MTU
}

func (wsi *WebSocketInterface) releaseCallbacks() {
	if wsi.onOpenFunc.Truthy() {
		wsi.onOpenFunc.Release()
		wsi.onOpenFunc = js.Func{}
	}
	if wsi.onMessageFunc.Truthy() {
		wsi.onMessageFunc.Release()
		wsi.onMessageFunc = js.Func{}
	}
	if wsi.onErrorFunc.Truthy() {
		wsi.onErrorFunc.Release()
		wsi.onErrorFunc = js.Func{}
	}
	if wsi.onCloseFunc.Truthy() {
		wsi.onCloseFunc.Release()
		wsi.onCloseFunc = js.Func{}
	}
}

func (wsi *WebSocketInterface) IsEnabled() bool {
	wsi.Mutex.RLock()
	defer wsi.Mutex.RUnlock()
	return wsi.Enabled && wsi.Online && !wsi.Detached
}
