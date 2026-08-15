// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build !js

package interfaces

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

const (
	wtDefaultPath      = "/rns"
	wtAppProtocol      = "rns"
	wtModeDatagram     = "datagram"
	wtModeStream       = "stream"
	wtModeDual         = "dual"
	wtDialTimeout      = 10 * time.Second
	wtHandshakeTimeout = 10 * time.Second
	wtIdleTimeout      = 60 * time.Second
)

// WebTransportClientOptions holds optional TLS and carriage settings for a client.
type WebTransportClientOptions struct {
	CertFile      string
	KeyFile       string
	PeerKey       string
	SNI           string
	TransportMode string
}

// WebTransportServerOptions holds optional TLS and carriage settings for a server.
type WebTransportServerOptions struct {
	CertFile      string
	KeyFile       string
	PeerKey       string
	TransportMode string
}

func normalizeWTPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return wtDefaultPath
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

func parseWTTransportMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", wtModeDatagram:
		return wtModeDatagram, nil
	case wtModeStream:
		return wtModeStream, nil
	case wtModeDual:
		return wtModeDual, nil
	default:
		return "", fmt.Errorf("unsupported transport_mode %q (want datagram, stream, or dual)", mode)
	}
}

func wtQUICConfig() *quic.Config {
	return &quic.Config{
		HandshakeIdleTimeout:             wtHandshakeTimeout,
		MaxIdleTimeout:                   wtIdleTimeout,
		KeepAlivePeriod:                  15 * time.Second,
		EnableDatagrams:                  true,
		EnableStreamResetPartialDelivery: true,
		MaxIncomingStreams:               16,
	}
}

func buildWTClientTLS(sni string, peerPin []byte, clientCert tls.Certificate) *tls.Config {
	cfg := buildQUICClientTLS(sni, peerPin, clientCert)
	cfg.NextProtos = []string{http3.NextProtoH3}
	return cfg
}

func buildWTServerTLS(cert tls.Certificate, peerPin []byte) *tls.Config {
	return http3.ConfigureTLSConfig(buildQUICServerTLS(cert, peerPin))
}

// wtClientConn wraps a WebTransport session for reconnectDriver and I/O.
type wtClientConn struct {
	sess    *webtransport.Session
	stream  *webtransport.Stream
	mode    string
	writeMu sync.Mutex
}

func (c *wtClientConn) Read(b []byte) (int, error) {
	c.writeMu.Lock()
	stream := c.stream
	c.writeMu.Unlock()
	if stream != nil {
		return stream.Read(b)
	}
	<-c.sess.Context().Done()
	return 0, net.ErrClosed
}

func (c *wtClientConn) Write(b []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	switch c.mode {
	case wtModeStream:
		if c.stream == nil {
			return 0, fmt.Errorf("no stream")
		}
		return c.stream.Write(b)
	default:
		if err := c.sess.SendDatagram(b); err != nil {
			return 0, err
		}
		return len(b), nil
	}
}

func (c *wtClientConn) Close() error {
	c.writeMu.Lock()
	stream := c.stream
	c.stream = nil
	c.writeMu.Unlock()
	if stream != nil {
		_ = stream.Close()
	}
	return c.sess.CloseWithError(0, "")
}

func (c *wtClientConn) LocalAddr() net.Addr           { return c.sess.LocalAddr() }
func (c *wtClientConn) RemoteAddr() net.Addr          { return c.sess.RemoteAddr() }
func (c *wtClientConn) SetDeadline(t time.Time) error { return c.SetReadDeadline(t) }
func (c *wtClientConn) SetReadDeadline(t time.Time) error {
	c.writeMu.Lock()
	stream := c.stream
	c.writeMu.Unlock()
	if stream != nil {
		return stream.SetReadDeadline(t)
	}
	return nil
}
func (c *wtClientConn) SetWriteDeadline(t time.Time) error {
	c.writeMu.Lock()
	stream := c.stream
	c.writeMu.Unlock()
	if stream != nil {
		return stream.SetWriteDeadline(t)
	}
	return nil
}

// WebTransportClientInterface dials a WebTransport peer.
type WebTransportClientInterface struct {
	BaseInterface
	conn              *wtClientConn
	targetHost        string
	targetPort        int
	path              string
	mode              string
	certFile          string
	keyFile           string
	peerKey           string
	sni               string
	peerPin           []byte
	clientCert        tls.Certificate
	maxReconnectTries int
	done              chan struct{}
	stopOnce          sync.Once
	reconnect         *reconnectDriver
	txFrame           []byte
	txMu              sync.Mutex
	readBuf           []byte
	recvCancel        context.CancelFunc

	DatagramsRX    atomic.Uint64
	DatagramsTX    atomic.Uint64
	StreamFramesRX atomic.Uint64
	StreamFramesTX atomic.Uint64
}

// NewWebTransportClientInterface constructs a client with unlimited reconnect by default.
func NewWebTransportClientInterface(name, host string, port int, path string, enabled bool, opts WebTransportClientOptions) (*WebTransportClientInterface, error) {
	return NewWebTransportClientInterfaceWithRetries(name, host, port, path, enabled, 0, opts)
}

// NewWebTransportClientInterfaceWithRetries constructs a client with reconnect limit.
func NewWebTransportClientInterfaceWithRetries(name, host string, port int, path string, enabled bool, maxTries int, opts WebTransportClientOptions) (*WebTransportClientInterface, error) {
	mode, err := parseWTTransportMode(opts.TransportMode)
	if err != nil {
		return nil, err
	}
	pin, err := parsePeerKeyPin(opts.PeerKey)
	if err != nil {
		return nil, err
	}
	cert, err := loadOrGenerateQUICCert(opts.CertFile, opts.KeyFile)
	if err != nil {
		return nil, err
	}
	wc := &WebTransportClientInterface{
		BaseInterface:     NewBaseInterface(name, common.IFTypeWebTransport, enabled),
		targetHost:        host,
		targetPort:        port,
		path:              normalizeWTPath(path),
		mode:              mode,
		certFile:          opts.CertFile,
		keyFile:           opts.KeyFile,
		peerKey:           opts.PeerKey,
		sni:               opts.SNI,
		peerPin:           pin,
		clientCert:        cert,
		maxReconnectTries: NormalizeMaxReconnectTries(maxTries),
		done:              make(chan struct{}),
		txFrame:           make([]byte, 0, DefaultMTU*2+4),
	}
	wc.MTU = DefaultMTU
	wc.initReconnectDriver()
	if enabled {
		wc.startReconnect()
	}
	return wc, nil
}

func (wc *WebTransportClientInterface) initReconnectDriver() {
	label := net.JoinHostPort(wc.targetHost, fmt.Sprintf("%d", wc.targetPort))
	wc.reconnect = newReconnectDriver(label, wc.maxReconnectTries, wc.done, wc.dialSession, wc.onConnected)
	wc.reconnect.setOnExhausted(func() {
		_ = wc.Stop()
	})
}

func (wc *WebTransportClientInterface) dialSession() (net.Conn, error) {
	url := fmt.Sprintf("https://%s%s", net.JoinHostPort(wc.targetHost, fmt.Sprintf("%d", wc.targetPort)), wc.path)
	tlsConf := buildWTClientTLS(wc.sni, wc.peerPin, wc.clientCert)
	d := &webtransport.Dialer{
		TLSClientConfig:      tlsConf,
		QUICConfig:           wtQUICConfig(),
		ApplicationProtocols: []string{wtAppProtocol},
	}
	defer d.Close()

	ctx, cancel := context.WithTimeout(context.Background(), wtDialTimeout)
	defer cancel()
	_, sess, err := d.Dial(ctx, url, nil)
	if err != nil {
		return nil, err
	}

	conn := &wtClientConn{sess: sess, mode: wc.mode}
	if wc.mode == wtModeStream {
		stream, err := sess.OpenStreamSync(ctx)
		if err != nil {
			_ = sess.CloseWithError(0, "open stream failed")
			return nil, err
		}
		if _, err := stream.Write([]byte{HDLCFlag, HDLCFlag}); err != nil {
			_ = stream.Close()
			_ = sess.CloseWithError(0, "stream prime failed")
			return nil, err
		}
		conn.stream = stream
	}
	return conn, nil
}

func (wc *WebTransportClientInterface) onConnected(conn net.Conn) {
	wcConn, ok := conn.(*wtClientConn)
	if !ok {
		_ = conn.Close()
		return
	}
	if !wc.adoptConn(wcConn) {
		_ = wcConn.Close()
		return
	}
	recvCtx, recvCancel := context.WithCancel(context.Background())
	wc.Mutex.Lock()
	wc.recvCancel = recvCancel
	wc.Mutex.Unlock()
	switch wc.mode {
	case wtModeStream:
		go wc.streamReadLoop()
	case wtModeDual:
		go wc.datagramRecvLoop(recvCtx)
		go wc.acceptStreamLoop(recvCtx)
	default:
		go wc.datagramRecvLoop(recvCtx)
	}
}

func (wc *WebTransportClientInterface) adoptConn(conn *wtClientConn) bool {
	wc.Mutex.Lock()
	defer wc.Mutex.Unlock()
	if wc.Detached {
		return false
	}
	select {
	case <-wc.done:
		return false
	default:
	}
	wc.conn = conn
	wc.Online = true
	return true
}

// SetConnectivityHooks registers reconnect up/down callbacks.
func (wc *WebTransportClientInterface) SetConnectivityHooks(onDown, onUp func()) {
	wc.Mutex.Lock()
	reconnect := wc.reconnect
	wc.Mutex.Unlock()
	if reconnect != nil {
		reconnect.setHooks(onDown, onUp)
	}
}

func (wc *WebTransportClientInterface) startReconnect() {
	wc.Mutex.Lock()
	reconnect := wc.reconnect
	wc.Mutex.Unlock()
	if reconnect != nil {
		reconnect.start()
	}
}

// Start begins or resumes the reconnect dial loop.
func (wc *WebTransportClientInterface) Start() error {
	wc.Mutex.Lock()
	if !wc.Enabled || wc.Detached {
		wc.Mutex.Unlock()
		return fmt.Errorf("interface not enabled or detached")
	}
	if wc.conn != nil {
		wc.Online = true
		recvCtx, recvCancel := context.WithCancel(context.Background())
		wc.recvCancel = recvCancel
		mode := wc.mode
		wc.Mutex.Unlock()
		switch mode {
		case wtModeStream:
			go wc.streamReadLoop()
		case wtModeDual:
			go wc.datagramRecvLoop(recvCtx)
			go wc.acceptStreamLoop(recvCtx)
		default:
			go wc.datagramRecvLoop(recvCtx)
		}
		return nil
	}
	select {
	case <-wc.done:
		wc.done = make(chan struct{})
		wc.stopOnce = sync.Once{}
		wc.initReconnectDriver()
	default:
		if wc.done == nil {
			wc.done = make(chan struct{})
			wc.stopOnce = sync.Once{}
		}
		if wc.reconnect == nil {
			wc.initReconnectDriver()
		}
	}
	wc.Mutex.Unlock()
	wc.startReconnect()
	return nil
}

// Stop closes the session and stops reconnect.
func (wc *WebTransportClientInterface) Stop() error {
	wc.Mutex.Lock()
	wc.Enabled = false
	wc.Online = false
	cancel := wc.recvCancel
	wc.recvCancel = nil
	conn := wc.conn
	wc.conn = nil
	wc.Mutex.Unlock()
	if cancel != nil {
		cancel()
	}
	if conn != nil {
		_ = conn.Close()
	}
	wc.stopOnce.Do(func() {
		if wc.done != nil {
			close(wc.done)
		}
	})
	return nil
}

// ProcessOutgoing transmits a packet according to the configured transport mode.
func (wc *WebTransportClientInterface) ProcessOutgoing(data []byte) error {
	wc.Mutex.RLock()
	online := wc.Online
	conn := wc.conn
	mode := wc.mode
	wc.Mutex.RUnlock()
	if !online || conn == nil {
		return fmt.Errorf("interface offline")
	}

	var err error
	switch mode {
	case wtModeStream:
		wc.txMu.Lock()
		frame := appendFrameHDLC(wc.txFrame[:0], data)
		wc.txFrame = frame
		out := append([]byte(nil), frame...)
		wc.txMu.Unlock()
		_, err = conn.Write(out)
		if err == nil {
			wc.StreamFramesTX.Add(1)
		}
	default:
		_, err = conn.Write(data)
		if err == nil {
			wc.DatagramsTX.Add(1)
		}
	}
	if err != nil {
		wc.Mutex.Lock()
		wc.Online = false
		detached := wc.Detached
		reconnect := wc.reconnect
		wc.Mutex.Unlock()
		if !detached && reconnect != nil {
			wc.teardownConn()
			reconnect.notifyFailure()
		}
	}
	return err
}

// Send applies IFAC then ProcessOutgoing.
func (wc *WebTransportClientInterface) Send(data []byte, address string) error {
	_ = address
	if err := common.RejectReceiveOnly(wc); err != nil {
		return err
	}
	masked, err := common.ApplyIFACOutbound(wc, data)
	if err != nil {
		return err
	}
	if err := wc.ProcessOutgoing(masked); err != nil {
		return err
	}
	wc.updateBandwidthStats(uint64(len(masked)))
	return nil
}

func (wc *WebTransportClientInterface) streamReadLoop() {
	decoder := newHDLCToggleStreamDecoder(wc.MTU, func(payload []byte) {
		if len(payload) == 0 {
			return
		}
		wc.StreamFramesRX.Add(1)
		wc.ProcessIncoming(payload)
	})
	if cap(wc.readBuf) < wc.MTU {
		wc.readBuf = make([]byte, wc.MTU)
	}
	buffer := wc.readBuf[:wc.MTU]
	for {
		wc.Mutex.RLock()
		conn := wc.conn
		done := wc.done
		wc.Mutex.RUnlock()
		if conn == nil {
			return
		}
		conn.writeMu.Lock()
		stream := conn.stream
		conn.writeMu.Unlock()
		if stream == nil {
			return
		}
		select {
		case <-done:
			return
		default:
		}
		n, err := stream.Read(buffer)
		if n > 0 {
			decoder.feed(buffer[:n])
		}
		if err != nil {
			if err != io.EOF {
				debug.Log(debug.DebugVerbose, "WebTransport client stream read ended", "name", wc.Name, "error", err)
			}
			wc.handleReadFailure()
			return
		}
	}
}

func (wc *WebTransportClientInterface) datagramRecvLoop(ctx context.Context) {
	for {
		wc.Mutex.RLock()
		conn := wc.conn
		done := wc.done
		wc.Mutex.RUnlock()
		if conn == nil {
			return
		}
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		default:
		}
		msg, err := conn.sess.ReceiveDatagram(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			default:
			}
			debug.Log(debug.DebugVerbose, "WebTransport client datagram recv ended", "name", wc.Name, "error", err)
			wc.handleReadFailure()
			return
		}
		if len(msg) == 0 {
			continue
		}
		wc.DatagramsRX.Add(1)
		wc.ProcessIncoming(msg)
	}
}

func (wc *WebTransportClientInterface) acceptStreamLoop(ctx context.Context) {
	for {
		wc.Mutex.RLock()
		conn := wc.conn
		done := wc.done
		wc.Mutex.RUnlock()
		if conn == nil {
			return
		}
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		default:
		}
		stream, err := conn.sess.AcceptStream(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			default:
			}
			debug.Log(debug.DebugVerbose, "WebTransport client accept stream ended", "name", wc.Name, "error", err)
			return
		}
		go wc.readHDLCStream(stream)
	}
}

func (wc *WebTransportClientInterface) readHDLCStream(stream *webtransport.Stream) {
	defer stream.Close()
	decoder := newHDLCToggleStreamDecoder(wc.MTU, func(payload []byte) {
		if len(payload) == 0 {
			return
		}
		wc.StreamFramesRX.Add(1)
		wc.ProcessIncoming(payload)
	})
	buf := make([]byte, wc.MTU)
	for {
		wc.Mutex.RLock()
		done := wc.done
		wc.Mutex.RUnlock()
		select {
		case <-done:
			return
		default:
		}
		n, err := stream.Read(buf)
		if n > 0 {
			decoder.feed(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func (wc *WebTransportClientInterface) handleReadFailure() {
	wc.Mutex.Lock()
	wc.Online = false
	detached := wc.Detached
	reconnect := wc.reconnect
	wc.Mutex.Unlock()
	if !detached && reconnect != nil {
		wc.teardownConn()
		reconnect.notifyFailure()
	} else {
		wc.teardownConn()
	}
}

func (wc *WebTransportClientInterface) teardownConn() {
	wc.Mutex.Lock()
	cancel := wc.recvCancel
	wc.recvCancel = nil
	conn := wc.conn
	wc.conn = nil
	wc.Online = false
	wc.Mutex.Unlock()
	if cancel != nil {
		cancel()
	}
	if conn != nil {
		_ = conn.Close()
	}
}

// LeafSPKIPinHex returns the client's leaf SPKI pin for peer_key on the remote side.
func (wc *WebTransportClientInterface) LeafSPKIPinHex() (string, error) {
	leaf, err := leafCertificate(wc.clientCert)
	if err != nil {
		return "", err
	}
	return SPKIPinHex(leaf), nil
}

type wtServerSess struct {
	sess    *webtransport.Session
	stream  *webtransport.Stream
	writeMu sync.Mutex
}

func (s *wtServerSess) close() {
	s.writeMu.Lock()
	if s.stream != nil {
		_ = s.stream.Close()
		s.stream = nil
	}
	s.writeMu.Unlock()
	_ = s.sess.CloseWithError(0, "")
}

// WebTransportServerInterface accepts WebTransport sessions and fans out packets.
type WebTransportServerInterface struct {
	BaseInterface
	sessions   map[string]*wtServerSess
	server     *webtransport.Server
	packetConn net.PacketConn
	bindAddr   string
	bindPort   int
	path       string
	mode       string
	certFile   string
	keyFile    string
	peerKey    string
	peerPin    []byte
	serverCert tls.Certificate
	done       chan struct{}
	stopOnce   sync.Once
	txFrame    []byte
	txMu       sync.Mutex
	serveWg    sync.WaitGroup
	listenAddr net.Addr

	DatagramsRX    atomic.Uint64
	DatagramsTX    atomic.Uint64
	StreamFramesRX atomic.Uint64
	StreamFramesTX atomic.Uint64
}

// NewWebTransportServerInterface constructs a WebTransport server interface.
func NewWebTransportServerInterface(name, bindAddr string, bindPort int, path string, opts WebTransportServerOptions) (*WebTransportServerInterface, error) {
	mode, err := parseWTTransportMode(opts.TransportMode)
	if err != nil {
		return nil, err
	}
	pin, err := parsePeerKeyPin(opts.PeerKey)
	if err != nil {
		return nil, err
	}
	cert, err := loadOrGenerateQUICCert(opts.CertFile, opts.KeyFile)
	if err != nil {
		return nil, err
	}
	ws := &WebTransportServerInterface{
		BaseInterface: NewBaseInterface(name, common.IFTypeWebTransport, true),
		sessions:      make(map[string]*wtServerSess),
		bindAddr:      bindAddr,
		bindPort:      bindPort,
		path:          normalizeWTPath(path),
		mode:          mode,
		certFile:      opts.CertFile,
		keyFile:       opts.KeyFile,
		peerKey:       opts.PeerKey,
		peerPin:       pin,
		serverCert:    cert,
		done:          make(chan struct{}),
		txFrame:       make([]byte, 0, DefaultMTU*2+4),
	}
	ws.MTU = DefaultMTU
	return ws, nil
}

// LeafSPKIPinHex returns the server leaf SPKI pin for client peer_key.
func (ws *WebTransportServerInterface) LeafSPKIPinHex() (string, error) {
	leaf, err := leafCertificate(ws.serverCert)
	if err != nil {
		return "", err
	}
	return SPKIPinHex(leaf), nil
}

// Start listens and accepts WebTransport sessions.
func (ws *WebTransportServerInterface) Start() error {
	ws.serveWg.Wait()
	ws.Mutex.Lock()
	if ws.server != nil {
		ws.Mutex.Unlock()
		return fmt.Errorf("WebTransport server already started")
	}
	select {
	case <-ws.done:
		ws.done = make(chan struct{})
		ws.stopOnce = sync.Once{}
	default:
		if ws.done == nil {
			ws.done = make(chan struct{})
			ws.stopOnce = sync.Once{}
		}
	}
	ws.Mutex.Unlock()

	addr := net.JoinHostPort(ws.bindAddr, fmt.Sprintf("%d", ws.bindPort))
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to resolve WebTransport bind: %w", err)
	}
	pc, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("failed to start WebTransport server: %w", err)
	}

	tlsConf := buildWTServerTLS(ws.serverCert, ws.peerPin)
	h3 := &http3.Server{
		TLSConfig:  tlsConf,
		QUICConfig: wtQUICConfig(),
	}
	webtransport.ConfigureHTTP3Server(h3)
	mux := http.NewServeMux()
	h3.Handler = mux

	srv := &webtransport.Server{
		H3:                   h3,
		ApplicationProtocols: []string{wtAppProtocol},
		CheckOrigin:          func(*http.Request) bool { return true },
	}

	mux.HandleFunc(ws.path, func(w http.ResponseWriter, r *http.Request) {
		sess, err := srv.Upgrade(w, r)
		if err != nil {
			debug.Log(debug.DebugVerbose, "WebTransport upgrade failed", "name", ws.Name, "error", err)
			return
		}
		go ws.handleSession(sess)
	})

	ws.Mutex.Lock()
	ws.packetConn = pc
	ws.server = srv
	ws.listenAddr = pc.LocalAddr()
	ws.Online = true
	ws.Mutex.Unlock()

	ws.serveWg.Go(func() {
		err := srv.Serve(pc)
		if err != nil {
			ws.Mutex.RLock()
			online := ws.Online
			ws.Mutex.RUnlock()
			if online {
				debug.Log(debug.DebugVerbose, "WebTransport serve ended", "name", ws.Name, "error", err)
			}
		}
	})
	return nil
}

func (ws *WebTransportServerInterface) handleSession(sess *webtransport.Session) {
	id := fmt.Sprintf("%s-%p", sess.RemoteAddr(), sess)
	entry := &wtServerSess{sess: sess}
	ws.Mutex.Lock()
	ws.sessions[id] = entry
	ws.Mutex.Unlock()
	defer func() {
		ws.Mutex.Lock()
		delete(ws.sessions, id)
		ws.Mutex.Unlock()
		entry.close()
	}()

	ctx := sess.Context()
	switch ws.mode {
	case wtModeStream:
		stream, err := sess.AcceptStream(ctx)
		if err != nil {
			return
		}
		entry.writeMu.Lock()
		entry.stream = stream
		entry.writeMu.Unlock()
		ws.readHDLCStream(stream)
	case wtModeDual:
		go ws.datagramRecvLoop(sess)
		ws.acceptStreamLoop(sess)
	default:
		ws.datagramRecvLoop(sess)
	}
}

func (ws *WebTransportServerInterface) datagramRecvLoop(sess *webtransport.Session) {
	ctx := sess.Context()
	for {
		ws.Mutex.RLock()
		done := ws.done
		ws.Mutex.RUnlock()
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		default:
		}
		msg, err := sess.ReceiveDatagram(ctx)
		if err != nil {
			return
		}
		if len(msg) == 0 {
			continue
		}
		ws.DatagramsRX.Add(1)
		ws.ProcessIncoming(msg)
	}
}

func (ws *WebTransportServerInterface) acceptStreamLoop(sess *webtransport.Session) {
	ctx := sess.Context()
	for {
		ws.Mutex.RLock()
		done := ws.done
		ws.Mutex.RUnlock()
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		default:
		}
		stream, err := sess.AcceptStream(ctx)
		if err != nil {
			return
		}
		go ws.readHDLCStream(stream)
	}
}

func (ws *WebTransportServerInterface) readHDLCStream(stream *webtransport.Stream) {
	defer stream.Close()
	decoder := newHDLCToggleStreamDecoder(ws.MTU, func(payload []byte) {
		if len(payload) == 0 {
			return
		}
		ws.StreamFramesRX.Add(1)
		ws.ProcessIncoming(payload)
	})
	buf := make([]byte, ws.MTU)
	for {
		ws.Mutex.RLock()
		done := ws.done
		ws.Mutex.RUnlock()
		select {
		case <-done:
			return
		default:
		}
		n, err := stream.Read(buf)
		if n > 0 {
			decoder.feed(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// SessionCount returns the number of active WebTransport sessions.
func (ws *WebTransportServerInterface) SessionCount() int {
	ws.Mutex.RLock()
	defer ws.Mutex.RUnlock()
	return len(ws.sessions)
}

// Stop closes the listener and all sessions.
func (ws *WebTransportServerInterface) Stop() error {
	ws.Mutex.Lock()
	ws.Online = false
	srv := ws.server
	ws.server = nil
	pc := ws.packetConn
	ws.packetConn = nil
	ws.listenAddr = nil
	sessions := make([]*wtServerSess, 0, len(ws.sessions))
	for id, s := range ws.sessions {
		sessions = append(sessions, s)
		delete(ws.sessions, id)
	}
	ws.Mutex.Unlock()

	for _, s := range sessions {
		s.close()
	}
	if srv != nil {
		_ = srv.Close()
	}
	if pc != nil {
		_ = pc.Close()
	}
	ws.stopOnce.Do(func() {
		if ws.done != nil {
			close(ws.done)
		}
	})
	ws.serveWg.Wait()
	return nil
}

// ProcessOutgoing fans out a packet to all sessions according to transport mode.
func (ws *WebTransportServerInterface) ProcessOutgoing(data []byte) error {
	ws.Mutex.RLock()
	online := ws.Online
	mode := ws.mode
	ws.Mutex.RUnlock()
	if !online {
		return fmt.Errorf("interface offline")
	}

	ws.Mutex.Lock()
	sessions := make([]*wtServerSess, 0, len(ws.sessions))
	for _, s := range ws.sessions {
		sessions = append(sessions, s)
	}
	ws.Mutex.Unlock()
	if len(sessions) == 0 {
		return fmt.Errorf("no WebTransport sessions")
	}

	var out []byte
	if mode == wtModeStream {
		ws.txMu.Lock()
		frame := appendFrameHDLC(ws.txFrame[:0], data)
		ws.txFrame = frame
		out = append([]byte(nil), frame...)
		ws.txMu.Unlock()
	} else {
		out = data
	}

	var lastErr error
	for _, s := range sessions {
		var err error
		switch mode {
		case wtModeStream:
			s.writeMu.Lock()
			stream := s.stream
			s.writeMu.Unlock()
			if stream == nil {
				err = fmt.Errorf("no stream")
			} else {
				s.writeMu.Lock()
				_, err = stream.Write(out)
				s.writeMu.Unlock()
				if err == nil {
					ws.StreamFramesTX.Add(1)
				}
			}
		default:
			s.writeMu.Lock()
			err = s.sess.SendDatagram(out)
			s.writeMu.Unlock()
			if err == nil {
				ws.DatagramsTX.Add(1)
			}
		}
		if err != nil {
			debug.Log(debug.DebugVerbose, "WebTransport server write failed", "address", s.sess.RemoteAddr(), "error", err)
			lastErr = err
		}
	}
	return lastErr
}

// Send applies IFAC then ProcessOutgoing.
func (ws *WebTransportServerInterface) Send(data []byte, address string) error {
	_ = address
	if err := common.RejectReceiveOnly(ws); err != nil {
		return err
	}
	masked, err := common.ApplyIFACOutbound(ws, data)
	if err != nil {
		return err
	}
	if err := ws.ProcessOutgoing(masked); err != nil {
		return err
	}
	ws.updateBandwidthStats(uint64(len(masked)))
	return nil
}

// ListenAddr returns the bound UDP address after Start (for tests).
func (ws *WebTransportServerInterface) ListenAddr() net.Addr {
	ws.Mutex.RLock()
	defer ws.Mutex.RUnlock()
	return ws.listenAddr
}
