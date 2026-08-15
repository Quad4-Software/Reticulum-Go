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
	"sync"
	"time"

	"github.com/quic-go/quic-go"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

const (
	quicDialTimeout      = 10 * time.Second
	quicHandshakeTimeout = 10 * time.Second
	quicIdleTimeout      = 60 * time.Second
)

// quicSessionConn wraps a bidirectional QUIC stream and its parent connection as net.Conn.
type quicSessionConn struct {
	stream  *quic.Stream
	conn    *quic.Conn
	writeMu sync.Mutex
}

func (c *quicSessionConn) Read(b []byte) (int, error) { return c.stream.Read(b) }
func (c *quicSessionConn) Write(b []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.stream.Write(b)
}
func (c *quicSessionConn) Close() error {
	c.writeMu.Lock()
	_ = c.stream.Close()
	c.writeMu.Unlock()
	return c.conn.CloseWithError(0, "")
}
func (c *quicSessionConn) LocalAddr() net.Addr                { return c.conn.LocalAddr() }
func (c *quicSessionConn) RemoteAddr() net.Addr               { return c.conn.RemoteAddr() }
func (c *quicSessionConn) SetDeadline(t time.Time) error      { return c.stream.SetDeadline(t) }
func (c *quicSessionConn) SetReadDeadline(t time.Time) error  { return c.stream.SetReadDeadline(t) }
func (c *quicSessionConn) SetWriteDeadline(t time.Time) error { return c.stream.SetWriteDeadline(t) }

func quicConfig() *quic.Config {
	return &quic.Config{
		HandshakeIdleTimeout: quicHandshakeTimeout,
		MaxIdleTimeout:       quicIdleTimeout,
		KeepAlivePeriod:      15 * time.Second,
		MaxIncomingStreams:   4,
	}
}

// QUICClientInterface dials a QUIC peer and carries HDLC frames on one stream.
type QUICClientInterface struct {
	BaseInterface
	conn              net.Conn
	targetAddr        string
	targetPort        int
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
}

// QUICClientOptions holds optional TLS settings for a QUIC client.
type QUICClientOptions struct {
	CertFile string
	KeyFile  string
	PeerKey  string
	SNI      string
}

// NewQUICClientInterface constructs a QUIC client with unlimited reconnect by default.
func NewQUICClientInterface(name, targetHost string, targetPort int, enabled bool, opts QUICClientOptions) (*QUICClientInterface, error) {
	return NewQUICClientInterfaceWithRetries(name, targetHost, targetPort, enabled, 0, opts)
}

// NewQUICClientInterfaceWithRetries constructs a QUIC client with reconnect limit.
func NewQUICClientInterfaceWithRetries(name, targetHost string, targetPort int, enabled bool, maxReconnectTries int, opts QUICClientOptions) (*QUICClientInterface, error) {
	pin, err := parsePeerKeyPin(opts.PeerKey)
	if err != nil {
		return nil, err
	}
	cert, err := loadOrGenerateQUICCert(opts.CertFile, opts.KeyFile)
	if err != nil {
		return nil, err
	}
	qc := &QUICClientInterface{
		BaseInterface:     NewBaseInterface(name, common.IFTypeQUIC, enabled),
		targetAddr:        targetHost,
		targetPort:        targetPort,
		certFile:          opts.CertFile,
		keyFile:           opts.KeyFile,
		peerKey:           opts.PeerKey,
		sni:               opts.SNI,
		peerPin:           pin,
		clientCert:        cert,
		maxReconnectTries: NormalizeMaxReconnectTries(maxReconnectTries),
		done:              make(chan struct{}),
		txFrame:           make([]byte, 0, DefaultMTU*2+4),
	}
	qc.MTU = DefaultMTU
	qc.initReconnectDriver()
	if enabled {
		qc.startReconnect()
	}
	return qc, nil
}

func (qc *QUICClientInterface) initReconnectDriver() {
	label := net.JoinHostPort(qc.targetAddr, fmt.Sprintf("%d", qc.targetPort))
	qc.reconnect = newReconnectDriver(label, qc.maxReconnectTries, qc.done, qc.dialSession, qc.onConnected)
	qc.reconnect.setOnExhausted(func() {
		_ = qc.Stop()
	})
}

func (qc *QUICClientInterface) dialSession() (net.Conn, error) {
	addr := net.JoinHostPort(qc.targetAddr, fmt.Sprintf("%d", qc.targetPort))
	tlsConf := buildQUICClientTLS(qc.sni, qc.peerPin, qc.clientCert)
	ctx, cancel := context.WithTimeout(context.Background(), quicDialTimeout)
	defer cancel()
	conn, err := quic.DialAddr(ctx, addr, tlsConf, quicConfig())
	if err != nil {
		return nil, err
	}
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		_ = conn.CloseWithError(0, "open stream failed")
		return nil, err
	}
	// quic-go only delivers streams to AcceptStream after the first write.
	if _, err := stream.Write([]byte{HDLCFlag, HDLCFlag}); err != nil {
		_ = stream.Close()
		_ = conn.CloseWithError(0, "stream prime failed")
		return nil, err
	}
	return &quicSessionConn{stream: stream, conn: conn}, nil
}

func (qc *QUICClientInterface) onConnected(conn net.Conn) {
	if !qc.adoptConn(conn) {
		_ = conn.Close()
		return
	}
	go qc.readLoop()
}

func (qc *QUICClientInterface) adoptConn(conn net.Conn) bool {
	qc.Mutex.Lock()
	defer qc.Mutex.Unlock()
	if qc.Detached {
		return false
	}
	select {
	case <-qc.done:
		return false
	default:
	}
	qc.conn = conn
	qc.Online = true
	return true
}

// SetConnectivityHooks registers reconnect up/down callbacks.
func (qc *QUICClientInterface) SetConnectivityHooks(onDown, onUp func()) {
	qc.Mutex.Lock()
	reconnect := qc.reconnect
	qc.Mutex.Unlock()
	if reconnect != nil {
		reconnect.setHooks(onDown, onUp)
	}
}

func (qc *QUICClientInterface) startReconnect() {
	qc.Mutex.Lock()
	reconnect := qc.reconnect
	qc.Mutex.Unlock()
	if reconnect != nil {
		reconnect.start()
	}
}

// Start begins or resumes the reconnect dial loop.
func (qc *QUICClientInterface) Start() error {
	qc.Mutex.Lock()
	if !qc.Enabled || qc.Detached {
		qc.Mutex.Unlock()
		return fmt.Errorf("interface not enabled or detached")
	}
	if qc.conn != nil {
		qc.Online = true
		go qc.readLoop()
		qc.Mutex.Unlock()
		return nil
	}
	select {
	case <-qc.done:
		qc.done = make(chan struct{})
		qc.stopOnce = sync.Once{}
		qc.initReconnectDriver()
	default:
		if qc.done == nil {
			qc.done = make(chan struct{})
			qc.stopOnce = sync.Once{}
		}
		if qc.reconnect == nil {
			qc.initReconnectDriver()
		}
	}
	qc.Mutex.Unlock()
	qc.startReconnect()
	return nil
}

// Stop closes the session and stops reconnect.
func (qc *QUICClientInterface) Stop() error {
	qc.Mutex.Lock()
	qc.Enabled = false
	qc.Online = false
	if qc.conn != nil {
		_ = qc.conn.Close()
		qc.conn = nil
	}
	qc.Mutex.Unlock()
	qc.stopOnce.Do(func() {
		if qc.done != nil {
			close(qc.done)
		}
	})
	return nil
}

// ProcessOutgoing writes an HDLC-framed payload on the QUIC stream.
func (qc *QUICClientInterface) ProcessOutgoing(data []byte) error {
	qc.Mutex.RLock()
	online := qc.Online
	conn := qc.conn
	qc.Mutex.RUnlock()
	if !online || conn == nil {
		return fmt.Errorf("interface offline")
	}
	qc.txMu.Lock()
	frame := appendFrameHDLC(qc.txFrame[:0], data)
	qc.txFrame = frame
	out := append([]byte(nil), frame...)
	qc.txMu.Unlock()
	_, err := conn.Write(out)
	if err != nil {
		qc.Mutex.Lock()
		qc.Online = false
		detached := qc.Detached
		reconnect := qc.reconnect
		qc.Mutex.Unlock()
		if !detached && reconnect != nil {
			qc.teardownConn()
			reconnect.notifyFailure()
		}
	}
	return err
}

// Send applies IFAC then ProcessOutgoing.
func (qc *QUICClientInterface) Send(data []byte, address string) error {
	_ = address
	if err := common.RejectReceiveOnly(qc); err != nil {
		return err
	}
	masked, err := common.ApplyIFACOutbound(qc, data)
	if err != nil {
		return err
	}
	if err := qc.ProcessOutgoing(masked); err != nil {
		return err
	}
	qc.updateBandwidthStats(uint64(len(masked)))
	return nil
}

func (qc *QUICClientInterface) readLoop() {
	decoder := newHDLCToggleStreamDecoder(qc.MTU, func(payload []byte) {
		if len(payload) == 0 {
			return
		}
		qc.ProcessIncoming(payload)
	})
	if cap(qc.readBuf) < qc.MTU {
		qc.readBuf = make([]byte, qc.MTU)
	}
	buffer := qc.readBuf[:qc.MTU]
	for {
		qc.Mutex.RLock()
		conn := qc.conn
		done := qc.done
		qc.Mutex.RUnlock()
		if conn == nil {
			return
		}
		select {
		case <-done:
			return
		default:
		}
		n, err := conn.Read(buffer)
		if n > 0 {
			decoder.feed(buffer[:n])
		}
		if err != nil {
			if err != io.EOF {
				debug.Log(debug.DebugVerbose, "QUIC client read ended", "name", qc.Name, "error", err)
			}
			qc.Mutex.Lock()
			qc.Online = false
			detached := qc.Detached
			reconnect := qc.reconnect
			qc.Mutex.Unlock()
			if !detached && reconnect != nil {
				qc.teardownConn()
				reconnect.notifyFailure()
			} else {
				qc.teardownConn()
			}
			return
		}
	}
}

func (qc *QUICClientInterface) teardownConn() {
	qc.Mutex.Lock()
	if qc.conn != nil {
		_ = qc.conn.Close()
		qc.conn = nil
	}
	qc.Mutex.Unlock()
}

// LeafSPKIPinHex returns the client's leaf SPKI pin for peer_key on the remote side.
func (qc *QUICClientInterface) LeafSPKIPinHex() (string, error) {
	leaf, err := leafCertificate(qc.clientCert)
	if err != nil {
		return "", err
	}
	return SPKIPinHex(leaf), nil
}

// QUICServerInterface listens for QUIC peers and fans out HDLC frames.
type QUICServerInterface struct {
	BaseInterface
	connections  map[string]net.Conn
	listener     *quic.Listener
	bindAddr     string
	bindPort     int
	certFile     string
	keyFile      string
	peerKey      string
	peerPin      []byte
	serverCert   tls.Certificate
	done         chan struct{}
	stopOnce     sync.Once
	txFrame      []byte
	cancelAccept context.CancelFunc
	txMu         sync.Mutex
	acceptWg     sync.WaitGroup
}

// QUICServerOptions holds optional TLS settings for a QUIC server.
type QUICServerOptions struct {
	CertFile string
	KeyFile  string
	PeerKey  string
}

// NewQUICServerInterface constructs a QUIC server interface.
func NewQUICServerInterface(name, bindAddr string, bindPort int, opts QUICServerOptions) (*QUICServerInterface, error) {
	pin, err := parsePeerKeyPin(opts.PeerKey)
	if err != nil {
		return nil, err
	}
	cert, err := loadOrGenerateQUICCert(opts.CertFile, opts.KeyFile)
	if err != nil {
		return nil, err
	}
	qs := &QUICServerInterface{
		BaseInterface: NewBaseInterface(name, common.IFTypeQUIC, true),
		connections:   make(map[string]net.Conn),
		bindAddr:      bindAddr,
		bindPort:      bindPort,
		certFile:      opts.CertFile,
		keyFile:       opts.KeyFile,
		peerKey:       opts.PeerKey,
		peerPin:       pin,
		serverCert:    cert,
		done:          make(chan struct{}),
		txFrame:       make([]byte, 0, DefaultMTU*2+4),
	}
	qs.MTU = DefaultMTU
	return qs, nil
}

// LeafSPKIPinHex returns the server leaf SPKI pin for client peer_key.
func (qs *QUICServerInterface) LeafSPKIPinHex() (string, error) {
	leaf, err := leafCertificate(qs.serverCert)
	if err != nil {
		return "", err
	}
	return SPKIPinHex(leaf), nil
}

// Start listens and accepts QUIC connections.
func (qs *QUICServerInterface) Start() error {
	qs.acceptWg.Wait()
	qs.Mutex.Lock()
	if qs.listener != nil {
		qs.Mutex.Unlock()
		return fmt.Errorf("QUIC server already started")
	}
	select {
	case <-qs.done:
		qs.done = make(chan struct{})
		qs.stopOnce = sync.Once{}
	default:
		if qs.done == nil {
			qs.done = make(chan struct{})
			qs.stopOnce = sync.Once{}
		}
	}
	qs.Mutex.Unlock()

	addr := net.JoinHostPort(qs.bindAddr, fmt.Sprintf("%d", qs.bindPort))
	tlsConf := buildQUICServerTLS(qs.serverCert, qs.peerPin)
	ln, err := quic.ListenAddr(addr, tlsConf, quicConfig())
	if err != nil {
		return fmt.Errorf("failed to start QUIC server: %w", err)
	}

	qs.Mutex.Lock()
	qs.listener = ln
	qs.Online = true
	qs.Mutex.Unlock()

	qs.acceptWg.Go(func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		qs.Mutex.Lock()
		qs.cancelAccept = cancel
		qs.Mutex.Unlock()
		qs.acceptLoop(ctx, ln)
	})
	return nil
}

func (qs *QUICServerInterface) acceptLoop(ctx context.Context, ln *quic.Listener) {
	for {
		qs.Mutex.RLock()
		done := qs.done
		qs.Mutex.RUnlock()
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		default:
		}
		conn, err := ln.Accept(ctx)
		if err != nil {
			qs.Mutex.RLock()
			online := qs.Online
			qs.Mutex.RUnlock()
			if !online {
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
			debug.Log(debug.DebugVerbose, "QUIC accept error", "name", qs.Name, "error", err)
			continue
		}
		go qs.handleConn(ctx, conn)
	}
}

func (qs *QUICServerInterface) handleConn(ctx context.Context, conn *quic.Conn) {
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		_ = conn.CloseWithError(0, "accept stream failed")
		return
	}
	sess := &quicSessionConn{stream: stream, conn: conn}
	addr := conn.RemoteAddr().String()
	qs.Mutex.Lock()
	qs.connections[addr] = sess
	qs.Mutex.Unlock()
	defer func() {
		qs.Mutex.Lock()
		delete(qs.connections, addr)
		qs.Mutex.Unlock()
		_ = sess.Close()
	}()
	qs.readHDLCLoop(sess)
}

// SessionCount returns the number of active QUIC sessions.
func (qs *QUICServerInterface) SessionCount() int {
	qs.Mutex.RLock()
	defer qs.Mutex.RUnlock()
	return len(qs.connections)
}

func (qs *QUICServerInterface) readHDLCLoop(conn net.Conn) {
	decoder := newHDLCToggleStreamDecoder(qs.MTU, func(payload []byte) {
		if len(payload) == 0 {
			return
		}
		qs.ProcessIncoming(payload)
	})
	buf := make([]byte, qs.MTU)
	for {
		qs.Mutex.RLock()
		done := qs.done
		qs.Mutex.RUnlock()
		select {
		case <-done:
			return
		default:
		}
		n, err := conn.Read(buf)
		if n > 0 {
			decoder.feed(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// Stop closes the listener and all sessions.
func (qs *QUICServerInterface) Stop() error {
	qs.Mutex.Lock()
	qs.Online = false
	cancel := qs.cancelAccept
	qs.cancelAccept = nil
	ln := qs.listener
	qs.listener = nil
	conns := make([]net.Conn, 0, len(qs.connections))
	for addr, conn := range qs.connections {
		conns = append(conns, conn)
		delete(qs.connections, addr)
	}
	qs.Mutex.Unlock()
	if cancel != nil {
		cancel()
	}
	if ln != nil {
		_ = ln.Close()
	}
	for _, conn := range conns {
		_ = conn.Close()
	}
	qs.stopOnce.Do(func() {
		if qs.done != nil {
			close(qs.done)
		}
	})
	qs.acceptWg.Wait()
	return nil
}

// ProcessOutgoing fans out an HDLC frame to all sessions.
func (qs *QUICServerInterface) ProcessOutgoing(data []byte) error {
	qs.Mutex.RLock()
	online := qs.Online
	qs.Mutex.RUnlock()
	if !online {
		return fmt.Errorf("interface offline")
	}
	qs.txMu.Lock()
	frame := appendFrameHDLC(qs.txFrame[:0], data)
	qs.txFrame = frame
	out := append([]byte(nil), frame...)
	qs.txMu.Unlock()
	qs.Mutex.Lock()
	conns := make([]net.Conn, 0, len(qs.connections))
	for _, c := range qs.connections {
		conns = append(conns, c)
	}
	qs.Mutex.Unlock()
	if len(conns) == 0 {
		return fmt.Errorf("no QUIC sessions")
	}
	var lastErr error
	for _, c := range conns {
		if _, err := c.Write(out); err != nil {
			debug.Log(debug.DebugVerbose, "QUIC server write failed", "address", c.RemoteAddr(), "error", err)
			lastErr = err
		}
	}
	return lastErr
}

// Send applies IFAC then ProcessOutgoing.
func (qs *QUICServerInterface) Send(data []byte, address string) error {
	_ = address
	if err := common.RejectReceiveOnly(qs); err != nil {
		return err
	}
	masked, err := common.ApplyIFACOutbound(qs, data)
	if err != nil {
		return err
	}
	if err := qs.ProcessOutgoing(masked); err != nil {
		return err
	}
	qs.updateBandwidthStats(uint64(len(masked)))
	return nil
}

// ListenAddr returns the bound address after Start (for tests).
func (qs *QUICServerInterface) ListenAddr() net.Addr {
	qs.Mutex.RLock()
	defer qs.Mutex.RUnlock()
	if qs.listener == nil {
		return nil
	}
	return qs.listener.Addr()
}
