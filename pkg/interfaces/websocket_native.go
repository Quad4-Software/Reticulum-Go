//go:build !js
// +build !js

// WebSocketInterface is a native implementation of the WebSocket interface.
// It is used to connect to the WebSocket server and send/receive data.
package interfaces

import (
	"bufio"
	"crypto/rand"
	// bearer:disable go_gosec_blocklist_sha1
	"crypto/sha1" // #nosec G505
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
)

const (
	wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

	WS_BUFFER_SIZE         = 4096
	WS_MTU                 = 1064
	WS_BITRATE             = 10000000
	WS_HTTPS_PORT          = 443
	WS_HTTP_PORT           = 80
	WS_VERSION             = "13"
	WS_CONNECT_TIMEOUT     = 10 * time.Second
	WS_RECONNECT_DELAY     = 2 * time.Second
	WS_KEY_SIZE            = 16
	WS_MASK_KEY_SIZE       = 4
	WS_HEADER_SIZE         = 2
	WS_PAYLOAD_LEN_16BIT   = 126
	WS_PAYLOAD_LEN_64BIT   = 127
	WS_MAX_PAYLOAD_16BIT   = 65536
	WS_FRAME_HEADER_FIN    = 0x80
	WS_FRAME_HEADER_OPCODE = 0x0F
	WS_FRAME_HEADER_MASKED = 0x80
	WS_FRAME_HEADER_LEN    = 0x7F
	WS_OPCODE_CONTINUATION = 0x00
	WS_OPCODE_TEXT         = 0x01
	WS_OPCODE_BINARY       = 0x02
	WS_OPCODE_CLOSE        = 0x08
	WS_OPCODE_PING         = 0x09
	WS_OPCODE_PONG         = 0x0A
)

type WebSocketInterface struct {
	BaseInterface
	wsURL        string
	conn         net.Conn
	reader       *bufio.Reader
	connected    bool
	messageQueue [][]byte
	readBuffer   []byte
	writeBuffer  []byte
	done         chan struct{}
	stopOnce     sync.Once
}

func NewWebSocketInterface(name string, wsURL string, enabled bool) (*WebSocketInterface, error) {
	ws := &WebSocketInterface{
		BaseInterface: NewBaseInterface(name, common.IF_TYPE_UDP, enabled),
		wsURL:         wsURL,
		messageQueue:  make([][]byte, 0),
		readBuffer:    make([]byte, WS_BUFFER_SIZE),
		writeBuffer:   make([]byte, WS_BUFFER_SIZE),
		done:          make(chan struct{}),
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
	wsi.closeWebSocketLocked()
}

func (wsi *WebSocketInterface) Enable() {
	wsi.Mutex.Lock()
	defer wsi.Mutex.Unlock()
	wsi.Enabled = true
	wsi.Online = true
}

func (wsi *WebSocketInterface) Disable() {
	wsi.Mutex.Lock()
	defer wsi.Mutex.Unlock()
	wsi.Enabled = false
	wsi.closeWebSocketLocked()
}

func (wsi *WebSocketInterface) Start() error {
	wsi.Mutex.Lock()
	if !wsi.Enabled || wsi.Detached {
		wsi.Mutex.Unlock()
		return fmt.Errorf("interface not enabled or detached")
	}
	if wsi.conn != nil {
		wsi.Mutex.Unlock()
		return fmt.Errorf("WebSocket already started")
	}
	// Only recreate done if it's nil or was closed
	select {
	case <-wsi.done:
		wsi.done = make(chan struct{})
		wsi.stopOnce = sync.Once{}
	default:
		if wsi.done == nil {
			wsi.done = make(chan struct{})
			wsi.stopOnce = sync.Once{}
		}
	}
	wsi.Mutex.Unlock()

	u, err := url.Parse(wsi.wsURL)
	if err != nil {
		return fmt.Errorf("invalid WebSocket URL: %v", err)
	}

	var conn net.Conn
	var host string

	if u.Scheme == "wss" {
		host = u.Host
		if !strings.Contains(host, ":") {
			host += fmt.Sprintf(":%d", WS_HTTPS_PORT)
		}
		tcpConn, err := net.DialTimeout("tcp", host, WS_CONNECT_TIMEOUT)
		if err != nil {
			return fmt.Errorf("failed to connect: %v", err)
		}
		tlsConn := tls.Client(tcpConn, &tls.Config{
			ServerName:         u.Hostname(),
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		})
		if err := tlsConn.Handshake(); err != nil {
			_ = tcpConn.Close()
			return fmt.Errorf("TLS handshake failed: %v", err)
		}
		conn = tlsConn
	} else if u.Scheme == "ws" {
		host = u.Host
		if !strings.Contains(host, ":") {
			host += fmt.Sprintf(":%d", WS_HTTP_PORT)
		}
		tcpConn, err := net.DialTimeout("tcp", host, WS_CONNECT_TIMEOUT)
		if err != nil {
			return fmt.Errorf("failed to connect: %v", err)
		}
		conn = tcpConn
	} else {
		return fmt.Errorf("unsupported scheme: %s (use ws:// or wss://)", u.Scheme)
	}

	key, err := generateWebSocketKey()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to generate key: %v", err)
	}

	path := u.Path
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}

	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Host = u.Host
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", key)
	req.Header.Set("Sec-WebSocket-Version", WS_VERSION)
	req.Header.Set("User-Agent", "Reticulum-Go/1.0")

	if err := req.Write(conn); err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to send handshake: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to read handshake response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		_ = conn.Close()
		return fmt.Errorf("handshake failed: status %d", resp.StatusCode)
	}

	if strings.ToLower(resp.Header.Get("Upgrade")) != "websocket" {
		_ = conn.Close()
		return fmt.Errorf("invalid upgrade header")
	}

	accept := resp.Header.Get("Sec-WebSocket-Accept")
	expectedAccept := computeAcceptKey(key)
	if accept != expectedAccept {
		_ = conn.Close()
		return fmt.Errorf("invalid accept key")
	}

	wsi.Mutex.Lock()
	wsi.conn = conn
	wsi.reader = bufio.NewReader(conn)
	wsi.connected = true
	wsi.Online = true

	debug.Log(debug.DEBUG_INFO, "WebSocket connected", "name", wsi.Name, "url", wsi.wsURL)

	queue := make([][]byte, len(wsi.messageQueue))
	copy(queue, wsi.messageQueue)
	wsi.messageQueue = wsi.messageQueue[:0]
	wsi.Mutex.Unlock() // Unlock after copying queue, before I/O

	for _, msg := range queue {
		_ = wsi.sendWebSocketMessage(msg)
	}

	go wsi.readLoop()

	return nil
}

func (wsi *WebSocketInterface) Stop() error {
	wsi.Mutex.Lock()
	defer wsi.Mutex.Unlock()

	wsi.Enabled = false
	wsi.Online = false

	wsi.stopOnce.Do(func() {
		if wsi.done != nil {
			close(wsi.done)
		}
	})

	wsi.closeWebSocketLocked()
	return nil
}

func (wsi *WebSocketInterface) closeWebSocket() {
	wsi.Mutex.Lock()
	defer wsi.Mutex.Unlock()
	wsi.closeWebSocketLocked()
}

func (wsi *WebSocketInterface) closeWebSocketLocked() {
	if wsi.conn != nil {
		wsi.sendCloseFrameLocked()
		_ = wsi.conn.Close()
		wsi.conn = nil
		wsi.reader = nil
	}
	wsi.connected = false
	wsi.Online = false
}

func (wsi *WebSocketInterface) readLoop() {
	for {
		wsi.Mutex.RLock()
		conn := wsi.conn
		reader := wsi.reader
		done := wsi.done
		wsi.Mutex.RUnlock()

		if conn == nil || reader == nil {
			return
		}

		select {
		case <-done:
			return
		default:
		}

		data, err := wsi.readFrame()
		if err != nil {
			wsi.Mutex.Lock()
			wsi.connected = false
			wsi.Online = false
			if wsi.conn != nil {
				_ = wsi.conn.Close()
				wsi.conn = nil
				wsi.reader = nil
			}
			wsi.Mutex.Unlock()

			debug.Log(debug.DEBUG_INFO, "WebSocket closed", "name", wsi.Name, "error", err)

			time.Sleep(WS_RECONNECT_DELAY)

			wsi.Mutex.RLock()
			stillEnabled := wsi.Enabled && !wsi.Detached
			wsi.Mutex.RUnlock()

			if stillEnabled {
				go wsi.Start()
			}
			return
		}

		if len(data) > 0 {
			wsi.Mutex.Lock()
			wsi.RxBytes += uint64(len(data))
			wsi.Mutex.Unlock()

			wsi.ProcessIncoming(data)
		}
	}
}

func (wsi *WebSocketInterface) readFrame() ([]byte, error) {
	wsi.Mutex.RLock()
	reader := wsi.reader
	wsi.Mutex.RUnlock()

	if reader == nil {
		return nil, io.EOF
	}

	header := make([]byte, WS_HEADER_SIZE)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, err
	}

	fin := (header[0] & WS_FRAME_HEADER_FIN) != 0
	opcode := header[0] & WS_FRAME_HEADER_OPCODE
	masked := (header[1] & WS_FRAME_HEADER_MASKED) != 0
	payloadLen := int(header[1] & WS_FRAME_HEADER_LEN)

	if opcode == WS_OPCODE_CLOSE {
		return nil, io.EOF
	}

	if opcode == WS_OPCODE_PING {
		return wsi.handlePingFrame(reader, payloadLen, masked)
	}

	if opcode == WS_OPCODE_PONG {
		return wsi.handlePongFrame(reader, payloadLen, masked)
	}

	if opcode != WS_OPCODE_BINARY {
		return nil, fmt.Errorf("unsupported opcode: %d", opcode)
	}

	if payloadLen == WS_PAYLOAD_LEN_16BIT {
		lenBytes := make([]byte, 2)
		if _, err := io.ReadFull(reader, lenBytes); err != nil {
			return nil, err
		}
		payloadLen = int(binary.BigEndian.Uint16(lenBytes))
	} else if payloadLen == WS_PAYLOAD_LEN_64BIT {
		lenBytes := make([]byte, 8)
		if _, err := io.ReadFull(reader, lenBytes); err != nil {
			return nil, err
		}
		val := binary.BigEndian.Uint64(lenBytes)
		if val > uint64(math.MaxInt) {
			return nil, fmt.Errorf("payload length exceeds maximum integer value")
		}
		payloadLen = int(val) // #nosec G115
	}

	maskKey := make([]byte, WS_MASK_KEY_SIZE)
	if masked {
		if _, err := io.ReadFull(reader, maskKey); err != nil {
			return nil, err
		}
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}

	if masked {
		for i := 0; i < payloadLen; i++ {
			payload[i] ^= maskKey[i%WS_MASK_KEY_SIZE]
		}
	}

	if !fin {
		nextFrame, err := wsi.readFrame()
		if err != nil {
			return nil, err
		}
		return append(payload, nextFrame...), nil
	}

	return payload, nil
}

func (wsi *WebSocketInterface) Send(data []byte, addr string) error {
	wsi.Mutex.RLock()
	enabled := wsi.Enabled
	detached := wsi.Detached
	connected := wsi.connected
	wsi.Mutex.RUnlock()

	if !enabled || detached {
		return fmt.Errorf("interface not enabled")
	}

	wsi.Mutex.Lock()
	wsi.TxBytes += uint64(len(data))
	wsi.Mutex.Unlock()

	if !connected {
		wsi.Mutex.Lock()
		wsi.messageQueue = append(wsi.messageQueue, data)
		wsi.Mutex.Unlock()
		return nil
	}

	return wsi.sendWebSocketMessage(data)
}

func (wsi *WebSocketInterface) sendWebSocketMessage(data []byte) error {
	wsi.Mutex.RLock()
	conn := wsi.conn
	wsi.Mutex.RUnlock()

	if conn == nil {
		return fmt.Errorf("WebSocket not initialized")
	}

	frame := wsi.createFrame(data, WS_OPCODE_BINARY, true)
	wsi.Mutex.Lock()
	_, err := conn.Write(frame)
	wsi.Mutex.Unlock()

	if err != nil {
		return fmt.Errorf("failed to send: %v", err)
	}

	debug.Log(debug.DEBUG_VERBOSE, "WebSocket sent packet", "name", wsi.Name, "bytes", len(data))
	return nil
}

func (wsi *WebSocketInterface) sendCloseFrame() {
	wsi.Mutex.RLock()
	defer wsi.Mutex.RUnlock()
	wsi.sendCloseFrameLocked()
}

func (wsi *WebSocketInterface) sendCloseFrameLocked() {
	conn := wsi.conn
	if conn == nil {
		return
	}

	frame := wsi.createFrame(nil, WS_OPCODE_CLOSE, true)
	_, _ = conn.Write(frame)
}

func (wsi *WebSocketInterface) handlePingFrame(reader *bufio.Reader, payloadLen int, masked bool) ([]byte, error) {
	if payloadLen == WS_PAYLOAD_LEN_16BIT {
		lenBytes := make([]byte, 2)
		if _, err := io.ReadFull(reader, lenBytes); err != nil {
			return nil, err
		}
		payloadLen = int(binary.BigEndian.Uint16(lenBytes))
	} else if payloadLen == WS_PAYLOAD_LEN_64BIT {
		lenBytes := make([]byte, 8)
		if _, err := io.ReadFull(reader, lenBytes); err != nil {
			return nil, err
		}
		val := binary.BigEndian.Uint64(lenBytes)
		if val > uint64(math.MaxInt) {
			return nil, fmt.Errorf("payload length exceeds maximum integer value")
		}
		payloadLen = int(val) // #nosec G115
	}

	maskKey := make([]byte, WS_MASK_KEY_SIZE)
	if masked {
		if _, err := io.ReadFull(reader, maskKey); err != nil {
			return nil, err
		}
	}

	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(reader, payload); err != nil {
			return nil, err
		}

		if masked {
			for i := 0; i < payloadLen; i++ {
				payload[i] ^= maskKey[i%WS_MASK_KEY_SIZE]
			}
		}
	}

	wsi.sendPongFrame(payload)
	return nil, nil
}

func (wsi *WebSocketInterface) handlePongFrame(reader *bufio.Reader, payloadLen int, masked bool) ([]byte, error) {
	if payloadLen == WS_PAYLOAD_LEN_16BIT {
		lenBytes := make([]byte, 2)
		if _, err := io.ReadFull(reader, lenBytes); err != nil {
			return nil, err
		}
		payloadLen = int(binary.BigEndian.Uint16(lenBytes))
	} else if payloadLen == WS_PAYLOAD_LEN_64BIT {
		lenBytes := make([]byte, 8)
		if _, err := io.ReadFull(reader, lenBytes); err != nil {
			return nil, err
		}
		val := binary.BigEndian.Uint64(lenBytes)
		if val > uint64(math.MaxInt) {
			return nil, fmt.Errorf("payload length exceeds maximum integer value")
		}
		payloadLen = int(val) // #nosec G115
	}

	maskKey := make([]byte, WS_MASK_KEY_SIZE)
	if masked {
		if _, err := io.ReadFull(reader, maskKey); err != nil {
			return nil, err
		}
	}

	if payloadLen > 0 {
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func (wsi *WebSocketInterface) sendPongFrame(data []byte) {
	wsi.Mutex.RLock()
	conn := wsi.conn
	wsi.Mutex.RUnlock()

	if conn == nil {
		return
	}

	frame := wsi.createFrame(data, WS_OPCODE_PONG, true)
	wsi.Mutex.Lock()
	_, _ = conn.Write(frame)
	wsi.Mutex.Unlock()
}

func (wsi *WebSocketInterface) createFrame(data []byte, opcode byte, fin bool) []byte {
	payloadLen := len(data)
	frame := make([]byte, WS_HEADER_SIZE)

	if fin {
		frame[0] |= WS_FRAME_HEADER_FIN
	}
	frame[0] |= opcode

	if payloadLen < WS_PAYLOAD_LEN_16BIT {
		frame[1] = byte(payloadLen)
		frame = append(frame, data...)
	} else if payloadLen < WS_MAX_PAYLOAD_16BIT {
		frame[1] = WS_PAYLOAD_LEN_16BIT // #nosec G602
		lenBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(lenBytes, uint16(payloadLen)) // #nosec G115
		frame = append(frame, lenBytes...)
		frame = append(frame, data...)
	} else {
		frame[1] = WS_PAYLOAD_LEN_64BIT // #nosec G602
		lenBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(lenBytes, uint64(payloadLen)) // #nosec G115
		frame = append(frame, lenBytes...)
		frame = append(frame, data...)
	}

	return frame
}

func (wsi *WebSocketInterface) ProcessOutgoing(data []byte) error {
	return wsi.Send(data, "")
}

func (wsi *WebSocketInterface) GetConn() net.Conn {
	wsi.Mutex.RLock()
	defer wsi.Mutex.RUnlock()
	return wsi.conn
}

func (wsi *WebSocketInterface) GetMTU() int {
	return wsi.MTU
}

func (wsi *WebSocketInterface) IsEnabled() bool {
	wsi.Mutex.RLock()
	defer wsi.Mutex.RUnlock()
	return wsi.Enabled && wsi.Online && !wsi.Detached
}

func (wsi *WebSocketInterface) SendPathRequest(packet []byte) error {
	return wsi.Send(packet, "")
}

func (wsi *WebSocketInterface) SendLinkPacket(dest []byte, data []byte, timestamp time.Time) error {
	frame := make([]byte, 0, len(dest)+len(data)+9)
	frame = append(frame, WS_OPCODE_BINARY)
	frame = append(frame, dest...)
	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, uint64(timestamp.Unix())) // #nosec G115
	frame = append(frame, ts...)
	frame = append(frame, data...)
	return wsi.Send(frame, "")
}

func (wsi *WebSocketInterface) GetBandwidthAvailable() bool {
	return wsi.BaseInterface.GetBandwidthAvailable()
}

func generateWebSocketKey() (string, error) {
	key := make([]byte, WS_KEY_SIZE)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

func computeAcceptKey(key string) string {
	// bearer:disable go_gosec_crypto_weak_crypto
	h := sha1.New() // #nosec G401
	h.Write([]byte(key))
	h.Write([]byte(wsGUID))
	// bearer:disable go_lang_weak_hash_sha1
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
