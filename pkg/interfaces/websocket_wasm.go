//go:build js && wasm
// +build js,wasm

package interfaces

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"syscall/js"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
)

type WebSocketInterface struct {
	BaseInterface
	wsURL        string
	ws           js.Value
	connected    bool
	mutex        sync.RWMutex
	messageQueue [][]byte
}

func NewWebSocketInterface(name string, wsURL string, enabled bool) (*WebSocketInterface, error) {
	ws := &WebSocketInterface{
		BaseInterface: NewBaseInterface(name, common.IF_TYPE_UDP, enabled),
		wsURL:         wsURL,
		messageQueue:  make([][]byte, 0),
	}

	ws.MTU = 1064
	ws.Bitrate = 10000000

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
	wsi.mutex.RLock()
	defer wsi.mutex.RUnlock()
	return wsi.Online && wsi.connected
}

func (wsi *WebSocketInterface) IsDetached() bool {
	wsi.mutex.RLock()
	defer wsi.mutex.RUnlock()
	return wsi.Detached
}

func (wsi *WebSocketInterface) Detach() {
	wsi.mutex.Lock()
	defer wsi.mutex.Unlock()
	wsi.Detached = true
	wsi.Online = false
	wsi.closeWebSocket()
}

func (wsi *WebSocketInterface) Enable() {
	wsi.mutex.Lock()
	defer wsi.mutex.Unlock()
	wsi.Enabled = true
}

func (wsi *WebSocketInterface) Disable() {
	wsi.mutex.Lock()
	defer wsi.mutex.Unlock()
	wsi.Enabled = false
	wsi.closeWebSocket()
}

func (wsi *WebSocketInterface) Start() error {
	wsi.mutex.Lock()
	defer wsi.mutex.Unlock()

	if wsi.ws.Truthy() {
		return fmt.Errorf("WebSocket already started")
	}

	ws := js.Global().Get("WebSocket").New(wsi.wsURL)
	ws.Set("binaryType", "arraybuffer")

	ws.Set("onopen", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		wsi.mutex.Lock()
		wsi.connected = true
		wsi.Online = true
		wsi.mutex.Unlock()

		debug.Log(debug.DEBUG_INFO, "WebSocket connected", "name", wsi.Name, "url", wsi.wsURL)

		wsi.mutex.Lock()
		queue := make([][]byte, len(wsi.messageQueue))
		copy(queue, wsi.messageQueue)
		wsi.messageQueue = wsi.messageQueue[:0]
		wsi.mutex.Unlock()

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

		var packetData []byte
		if data.Type() == js.TypeString {
			packetData = []byte(data.String())
		} else if data.Type() == js.TypeObject {
			array := js.Global().Get("Uint8Array").New(data)
			length := array.Get("length").Int()
			packetData = make([]byte, length)
			js.CopyBytesToGo(packetData, array)
		} else {
			debug.Log(debug.DEBUG_ERROR, "Unknown WebSocket message type", "type", data.Type().String())
			return nil
		}

		if len(packetData) < 4 {
			debug.Log(debug.DEBUG_ERROR, "WebSocket message too short", "bytes", len(packetData))
			return nil
		}

		packetLen := binary.BigEndian.Uint32(packetData[:4])
		if len(packetData) < int(packetLen)+4 {
			debug.Log(debug.DEBUG_ERROR, "WebSocket message incomplete", "expected", packetLen+4, "got", len(packetData))
			return nil
		}

		packet := packetData[4 : 4+packetLen]

		wsi.mutex.Lock()
		wsi.RxBytes += uint64(len(packet))
		wsi.mutex.Unlock()

		wsi.ProcessIncoming(packet)

		return nil
	}))

	ws.Set("onerror", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		debug.Log(debug.DEBUG_ERROR, "WebSocket error", "name", wsi.Name)
		return nil
	}))

	ws.Set("onclose", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		wsi.mutex.Lock()
		wsi.connected = false
		wsi.Online = false
		wsi.mutex.Unlock()

		debug.Log(debug.DEBUG_INFO, "WebSocket closed", "name", wsi.Name)

		if wsi.Enabled && !wsi.Detached {
			time.Sleep(2 * time.Second)
			go wsi.Start()
		}

		return nil
	}))

	wsi.ws = ws

	return nil
}

func (wsi *WebSocketInterface) Stop() error {
	wsi.mutex.Lock()
	defer wsi.mutex.Unlock()
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

	wsi.mutex.Lock()
	wsi.TxBytes += uint64(len(data))
	wsi.mutex.Unlock()

	if !wsi.connected {
		wsi.mutex.Lock()
		wsi.messageQueue = append(wsi.messageQueue, data)
		wsi.mutex.Unlock()
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

	packetLen := uint32(len(data))
	packet := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(packet[:4], packetLen)
	copy(packet[4:], data)

	array := js.Global().Get("Uint8Array").New(len(packet))
	js.CopyBytesToJS(array, packet)

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
	wsi.mutex.RLock()
	defer wsi.mutex.RUnlock()
	return wsi.Enabled && wsi.Online && !wsi.Detached
}
