// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
//go:build js && wasm
// +build js,wasm

package interfaces

import (
	"fmt"
	"net"
	"syscall/js"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
)

const (
	WS_MTU             = 1064
	WS_BITRATE         = 10000000
	WS_RECONNECT_DELAY = 2 * time.Second
)

type WebSocketInterface struct {
	BaseInterface
	wsURL        string
	ws           js.Value
	connected    bool
	messageQueue [][]byte
}

func NewWebSocketInterface(name string, wsURL string, enabled bool) (*WebSocketInterface, error) {
	ws := &WebSocketInterface{
		BaseInterface: NewBaseInterface(name, common.IF_TYPE_UDP, enabled),
		wsURL:         wsURL,
		messageQueue:  make([][]byte, 0),
	}

	ws.MTU = WS_MTU
	ws.Bitrate = WS_BITRATE

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

	ws := js.Global().Get("WebSocket").New(wsi.wsURL)
	ws.Set("binaryType", "arraybuffer")

	ws.Set("onopen", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		wsi.Mutex.Lock()
		wsi.connected = true
		wsi.Online = true
		wsi.Mutex.Unlock()

		debug.Log(debug.DEBUG_INFO, "WebSocket connected", "name", wsi.Name, "url", wsi.wsURL)

		wsi.Mutex.Lock()
		queue := make([][]byte, len(wsi.messageQueue))
		copy(queue, wsi.messageQueue)
		wsi.messageQueue = wsi.messageQueue[:0]
		wsi.Mutex.Unlock()

		for _, msg := range queue {
			wsi.sendWebSocketMessage(msg)
		}

		return nil
	}))

	ws.Set("onmessage", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}

		event := args[0]
		data := event.Get("data")

		var packet []byte
		if data.Type() == js.TypeString {
			packet = []byte(data.String())
		} else if data.Type() == js.TypeObject {
			array := js.Global().Get("Uint8Array").New(data)
			length := array.Get("length").Int()
			packet = make([]byte, length)
			js.CopyBytesToGo(packet, array)
		} else {
			debug.Log(debug.DEBUG_ERROR, "Unknown WebSocket message type", "type", data.Type().String())
			return nil
		}

		if len(packet) < 1 {
			debug.Log(debug.DEBUG_ERROR, "WebSocket message empty")
			return nil
		}

		wsi.Mutex.Lock()
		wsi.RxBytes += uint64(len(packet))
		wsi.Mutex.Unlock()

		wsi.ProcessIncoming(packet)

		return nil
	}))

	ws.Set("onerror", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		debug.Log(debug.DEBUG_ERROR, "WebSocket error", "name", wsi.Name)
		return nil
	}))

	ws.Set("onclose", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		wsi.Mutex.Lock()
		wsi.connected = false
		wsi.Online = false
		wsi.Mutex.Unlock()

		debug.Log(debug.DEBUG_INFO, "WebSocket closed", "name", wsi.Name)

		if wsi.Enabled && !wsi.Detached {
			time.Sleep(WS_RECONNECT_DELAY)
			go wsi.Start()
		}

		return nil
	}))

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
	wsi.connected = false
	wsi.Online = false
}

func (wsi *WebSocketInterface) Send(data []byte, addr string) error {
	if !wsi.IsEnabled() {
		return fmt.Errorf("interface not enabled")
	}

	wsi.Mutex.Lock()
	wsi.TxBytes += uint64(len(data))
	wsi.Mutex.Unlock()

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

	debug.Log(debug.DEBUG_VERBOSE, "WebSocket sent packet", "name", wsi.Name, "bytes", len(data))
	return nil
}

func (wsi *WebSocketInterface) ProcessOutgoing(data []byte) error {
	return wsi.Send(data, "")
}

func (wsi *WebSocketInterface) GetConn() net.Conn {
	return nil
}

func (wsi *WebSocketInterface) GetMTU() int {
	return wsi.MTU
}

func (wsi *WebSocketInterface) IsEnabled() bool {
	wsi.Mutex.RLock()
	defer wsi.Mutex.RUnlock()
	return wsi.Enabled && wsi.Online && !wsi.Detached
}
