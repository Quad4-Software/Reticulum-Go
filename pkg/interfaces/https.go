// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build !js

package interfaces

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

const (
	httpsDefaultPath     = "/rns"
	httpsDefaultLongPoll = 25 * time.Second
	httpsPeerHeader      = "X-RNS-Peer"
	httpsPeerIDBytes     = 16
	httpsQueueSize       = 64
	httpsMaxBodySlack    = 128
	httpsDialTimeout     = 10 * time.Second
)

// HTTPSClientOptions holds optional TLS and long-poll settings for a client.
type HTTPSClientOptions struct {
	CertFile string
	KeyFile  string
	PeerKey  string
	SNI      string
	Path     string
	LongPoll time.Duration
}

// HTTPSServerOptions holds optional TLS and long-poll settings for a server.
type HTTPSServerOptions struct {
	CertFile string
	KeyFile  string
	PeerKey  string
	Path     string
	LongPoll time.Duration
}

func normalizeHTTPSPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return httpsDefaultPath
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		return httpsDefaultPath
	}
	return path
}

func normalizeHTTPSLongPoll(d time.Duration) time.Duration {
	if d <= 0 {
		return httpsDefaultLongPoll
	}
	return d
}

func buildHTTPSClientTLS(sni string, peerPin []byte, clientCert tls.Certificate) *tls.Config {
	cfg := buildQUICClientTLS(sni, peerPin, clientCert)
	cfg.NextProtos = []string{"http/1.1"}
	return cfg
}

func buildHTTPSServerTLS(cert tls.Certificate, peerPin []byte) *tls.Config {
	cfg := buildQUICServerTLS(cert, peerPin)
	cfg.NextProtos = []string{"http/1.1"}
	return cfg
}

func newHTTPSPeerID() (string, error) {
	b := make([]byte, httpsPeerIDBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

type httpsPeerQueue struct {
	ch chan []byte
}

func newHTTPSPeerQueue() *httpsPeerQueue {
	return &httpsPeerQueue{ch: make(chan []byte, httpsQueueSize)}
}

func (q *httpsPeerQueue) enqueue(pkt []byte) {
	select {
	case q.ch <- pkt:
	default:
		select {
		case <-q.ch:
		default:
		}
		select {
		case q.ch <- pkt:
		default:
		}
	}
}

// HTTPSClientInterface posts outbound packets and long-polls for inbound ones.
type HTTPSClientInterface struct {
	BaseInterface
	host              string
	port              int
	path              string
	longPoll          time.Duration
	peerID            string
	certFile          string
	keyFile           string
	peerKey           string
	sni               string
	peerPin           []byte
	clientCert        tls.Certificate
	maxReconnectTries int
	httpClient        *http.Client
	baseURL           string
	done              chan struct{}
	stopOnce          sync.Once
	pollWg            sync.WaitGroup
	polling           bool
	onDown            func()
	onUp              func()
}

// NewHTTPSClientInterface constructs a client with unlimited reconnect by default.
func NewHTTPSClientInterface(name, host string, port int, enabled bool, opts HTTPSClientOptions) (*HTTPSClientInterface, error) {
	return NewHTTPSClientInterfaceWithRetries(name, host, port, enabled, 0, opts)
}

// NewHTTPSClientInterfaceWithRetries constructs a client with reconnect limit.
func NewHTTPSClientInterfaceWithRetries(name, host string, port int, enabled bool, maxTries int, opts HTTPSClientOptions) (*HTTPSClientInterface, error) {
	pin, err := parsePeerKeyPin(opts.PeerKey)
	if err != nil {
		return nil, err
	}
	cert, err := loadOrGenerateQUICCert(opts.CertFile, opts.KeyFile)
	if err != nil {
		return nil, err
	}
	peerID, err := newHTTPSPeerID()
	if err != nil {
		return nil, err
	}
	longPoll := normalizeHTTPSLongPoll(opts.LongPoll)
	path := normalizeHTTPSPath(opts.Path)
	tlsConf := buildHTTPSClientTLS(opts.SNI, pin, cert)
	hc := &HTTPSClientInterface{
		BaseInterface:     NewBaseInterface(name, common.IFTypeHTTPS, enabled),
		host:              host,
		port:              port,
		path:              path,
		longPoll:          longPoll,
		peerID:            peerID,
		certFile:          opts.CertFile,
		keyFile:           opts.KeyFile,
		peerKey:           opts.PeerKey,
		sni:               opts.SNI,
		peerPin:           pin,
		clientCert:        cert,
		maxReconnectTries: NormalizeMaxReconnectTries(maxTries),
		done:              make(chan struct{}),
		baseURL:           fmt.Sprintf("https://%s%s", net.JoinHostPort(host, fmt.Sprintf("%d", port)), path),
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig:     tlsConf,
				TLSHandshakeTimeout: httpsDialTimeout,
				DisableKeepAlives:   false,
			},
			Timeout: longPoll + httpsDialTimeout + 5*time.Second,
		},
	}
	hc.MTU = DefaultMTU
	if enabled {
		hc.startPollLoop()
	}
	return hc, nil
}

// LeafSPKIPinHex returns the client's leaf SPKI pin for peer_key on the remote side.
func (hc *HTTPSClientInterface) LeafSPKIPinHex() (string, error) {
	leaf, err := leafCertificate(hc.clientCert)
	if err != nil {
		return "", err
	}
	return SPKIPinHex(leaf), nil
}

// SetConnectivityHooks registers up/down callbacks.
func (hc *HTTPSClientInterface) SetConnectivityHooks(onDown, onUp func()) {
	hc.Mutex.Lock()
	hc.onDown = onDown
	hc.onUp = onUp
	hc.Mutex.Unlock()
}

func (hc *HTTPSClientInterface) fireDown() {
	hc.Mutex.RLock()
	fn := hc.onDown
	hc.Mutex.RUnlock()
	if fn != nil {
		fn()
	}
}

func (hc *HTTPSClientInterface) fireUp() {
	hc.Mutex.RLock()
	fn := hc.onUp
	hc.Mutex.RUnlock()
	if fn != nil {
		fn()
	}
}

func (hc *HTTPSClientInterface) startPollLoop() {
	hc.Mutex.Lock()
	if hc.polling {
		hc.Mutex.Unlock()
		return
	}
	hc.polling = true
	hc.Mutex.Unlock()
	hc.pollWg.Add(1)
	go func() {
		defer func() {
			hc.Mutex.Lock()
			hc.polling = false
			hc.Mutex.Unlock()
			hc.pollWg.Done()
		}()
		hc.pollLoop()
	}()
}

func (hc *HTTPSClientInterface) markOnline() {
	hc.Mutex.Lock()
	was := hc.Online
	hc.Online = true
	hc.Mutex.Unlock()
	if !was {
		hc.fireUp()
	}
}

func (hc *HTTPSClientInterface) markOffline() {
	hc.Mutex.Lock()
	was := hc.Online
	hc.Online = false
	hc.Mutex.Unlock()
	if was {
		hc.fireDown()
	}
}

func (hc *HTTPSClientInterface) pollLoop() {
	backoff := InitialBackoff
	retries := 0
	unlimited := hc.maxReconnectTries < 0

	for {
		select {
		case <-hc.done:
			return
		default:
		}

		if err := hc.doRegister(); err != nil {
			hc.markOffline()
			debug.Log(debug.DebugVerbose, "HTTPS client register failed", "name", hc.Name, "error", err)
			if !unlimited {
				retries++
				if retries >= hc.maxReconnectTries {
					debug.Log(debug.DebugError, "HTTPS client reconnect exhausted", "name", hc.Name)
					hc.signalStop()
					return
				}
			}
			select {
			case <-hc.done:
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > MaxBackoff {
				backoff = MaxBackoff
			}
			continue
		}

		backoff = InitialBackoff
		retries = 0
		hc.markOnline()

	poll:
		for {
			select {
			case <-hc.done:
				return
			default:
			}

			status, body, err := hc.doPoll()
			if err != nil {
				hc.markOffline()
				debug.Log(debug.DebugVerbose, "HTTPS client poll failed", "name", hc.Name, "error", err)
				break poll
			}

			switch status {
			case http.StatusOK:
				if len(body) > 0 {
					hc.ProcessIncoming(body)
				}
			case http.StatusNoContent:
				// idle long-poll timeout
			default:
				debug.Log(debug.DebugVerbose, "HTTPS client poll unexpected status",
					"name", hc.Name, "status", status)
				hc.markOffline()
				break poll
			}
		}

		if !unlimited {
			retries++
			if retries >= hc.maxReconnectTries {
				debug.Log(debug.DebugError, "HTTPS client reconnect exhausted", "name", hc.Name)
				hc.signalStop()
				return
			}
		}
		select {
		case <-hc.done:
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > MaxBackoff {
			backoff = MaxBackoff
		}
	}
}

func (hc *HTTPSClientInterface) doRegister() error {
	req, err := http.NewRequest(http.MethodPost, hc.baseURL+"/send", http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set(httpsPeerHeader, hc.peerID)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := hc.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTPS register status %d", resp.StatusCode)
	}
	return nil
}

func (hc *HTTPSClientInterface) doPoll() (int, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, hc.baseURL+"/poll", nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set(httpsPeerHeader, hc.peerID)
	resp, err := hc.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	limit := int64(hc.MTU + httpsMaxBodySlack)
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if int64(len(body)) > limit {
		return resp.StatusCode, nil, fmt.Errorf("poll body exceeds MTU")
	}
	return resp.StatusCode, body, nil
}

// Start begins or resumes the long-poll loop.
func (hc *HTTPSClientInterface) Start() error {
	hc.Mutex.Lock()
	if hc.Detached {
		hc.Mutex.Unlock()
		return fmt.Errorf("interface detached")
	}
	hc.Enabled = true
	select {
	case <-hc.done:
		hc.done = make(chan struct{})
		hc.stopOnce = sync.Once{}
	default:
		if hc.done == nil {
			hc.done = make(chan struct{})
			hc.stopOnce = sync.Once{}
		}
	}
	polling := hc.polling
	hc.Mutex.Unlock()
	if polling {
		return nil
	}
	hc.startPollLoop()
	return nil
}

func (hc *HTTPSClientInterface) signalStop() {
	hc.Mutex.Lock()
	hc.Enabled = false
	hc.Online = false
	hc.Mutex.Unlock()
	hc.stopOnce.Do(func() {
		if hc.done != nil {
			close(hc.done)
		}
	})
}

// Stop ends the long-poll loop.
func (hc *HTTPSClientInterface) Stop() error {
	hc.signalStop()
	hc.pollWg.Wait()
	return nil
}

// ProcessOutgoing POSTs a raw packet to the server /send endpoint.
func (hc *HTTPSClientInterface) ProcessOutgoing(data []byte) error {
	hc.Mutex.RLock()
	online := hc.Online
	hc.Mutex.RUnlock()
	if !online {
		return fmt.Errorf("interface offline")
	}
	if len(data) == 0 {
		return nil
	}
	if len(data) > hc.MTU+httpsMaxBodySlack {
		return fmt.Errorf("packet exceeds MTU")
	}
	req, err := http.NewRequest(http.MethodPost, hc.baseURL+"/send", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set(httpsPeerHeader, hc.peerID)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := hc.httpClient.Do(req)
	if err != nil {
		hc.markOffline()
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		hc.markOffline()
		return fmt.Errorf("HTTPS send status %d", resp.StatusCode)
	}
	return nil
}

// Send applies IFAC then ProcessOutgoing.
func (hc *HTTPSClientInterface) Send(data []byte, address string) error {
	_ = address
	if err := common.RejectReceiveOnly(hc); err != nil {
		return err
	}
	masked, err := common.ApplyIFACOutbound(hc, data)
	if err != nil {
		return err
	}
	if err := hc.ProcessOutgoing(masked); err != nil {
		return err
	}
	hc.updateBandwidthStats(uint64(len(masked)))
	return nil
}

// HTTPSServerInterface serves TLS HTTP long-poll endpoints for RNS packets.
type HTTPSServerInterface struct {
	BaseInterface
	bindAddr   string
	bindPort   int
	path       string
	longPoll   time.Duration
	certFile   string
	keyFile    string
	peerKey    string
	peerPin    []byte
	serverCert tls.Certificate
	peers      map[string]*httpsPeerQueue
	httpServer *http.Server
	listener   net.Listener
	done       chan struct{}
	stopOnce   sync.Once
	serveWg    sync.WaitGroup
}

// NewHTTPSServerInterface constructs an HTTPS long-poll server interface.
func NewHTTPSServerInterface(name, bindAddr string, bindPort int, opts HTTPSServerOptions) (*HTTPSServerInterface, error) {
	pin, err := parsePeerKeyPin(opts.PeerKey)
	if err != nil {
		return nil, err
	}
	cert, err := loadOrGenerateQUICCert(opts.CertFile, opts.KeyFile)
	if err != nil {
		return nil, err
	}
	hs := &HTTPSServerInterface{
		BaseInterface: NewBaseInterface(name, common.IFTypeHTTPS, true),
		bindAddr:      bindAddr,
		bindPort:      bindPort,
		path:          normalizeHTTPSPath(opts.Path),
		longPoll:      normalizeHTTPSLongPoll(opts.LongPoll),
		certFile:      opts.CertFile,
		keyFile:       opts.KeyFile,
		peerKey:       opts.PeerKey,
		peerPin:       pin,
		serverCert:    cert,
		peers:         make(map[string]*httpsPeerQueue),
		done:          make(chan struct{}),
	}
	hs.MTU = DefaultMTU
	return hs, nil
}

// LeafSPKIPinHex returns the server leaf SPKI pin for client peer_key.
func (hs *HTTPSServerInterface) LeafSPKIPinHex() (string, error) {
	leaf, err := leafCertificate(hs.serverCert)
	if err != nil {
		return "", err
	}
	return SPKIPinHex(leaf), nil
}

// PeerCount returns the number of known long-poll peers.
func (hs *HTTPSServerInterface) PeerCount() int {
	hs.Mutex.RLock()
	defer hs.Mutex.RUnlock()
	return len(hs.peers)
}

func (hs *HTTPSServerInterface) ensurePeer(peerID string) *httpsPeerQueue {
	hs.Mutex.Lock()
	defer hs.Mutex.Unlock()
	q := hs.peers[peerID]
	if q == nil {
		q = newHTTPSPeerQueue()
		hs.peers[peerID] = q
	}
	return q
}

// Start listens and serves HTTPS long-poll endpoints.
func (hs *HTTPSServerInterface) Start() error {
	hs.serveWg.Wait()
	hs.Mutex.Lock()
	if hs.httpServer != nil {
		hs.Mutex.Unlock()
		return fmt.Errorf("HTTPS server already started")
	}
	select {
	case <-hs.done:
		hs.done = make(chan struct{})
		hs.stopOnce = sync.Once{}
	default:
		if hs.done == nil {
			hs.done = make(chan struct{})
			hs.stopOnce = sync.Once{}
		}
	}
	hs.Mutex.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc(hs.path+"/send", hs.handleSend)
	mux.HandleFunc(hs.path+"/poll", hs.handlePoll)

	tlsConf := buildHTTPSServerTLS(hs.serverCert, hs.peerPin)
	addr := net.JoinHostPort(hs.bindAddr, fmt.Sprintf("%d", hs.bindPort))
	ln, err := tls.Listen("tcp", addr, tlsConf)
	if err != nil {
		return fmt.Errorf("failed to start HTTPS server: %w", common.WrapListenError(err))
	}
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * hs.longPoll,
	}

	hs.Mutex.Lock()
	hs.listener = ln
	hs.httpServer = srv
	hs.Online = true
	hs.Mutex.Unlock()

	hs.serveWg.Go(func() {
		err := srv.Serve(ln)
		if err != nil && err != http.ErrServerClosed {
			debug.Log(debug.DebugVerbose, "HTTPS serve ended", "name", hs.Name, "error", err)
		}
	})
	return nil
}

func (hs *HTTPSServerInterface) peerIDFromRequest(r *http.Request) (string, error) {
	peerID := strings.TrimSpace(r.Header.Get(httpsPeerHeader))
	if peerID == "" {
		return "", fmt.Errorf("missing %s", httpsPeerHeader)
	}
	if len(peerID) > 128 {
		return "", fmt.Errorf("peer id too long")
	}
	return peerID, nil
}

func (hs *HTTPSServerInterface) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	peerID, err := hs.peerIDFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	hs.ensurePeer(peerID)

	limit := int64(hs.MTU + httpsMaxBodySlack)
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	if int64(len(body)) > limit {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	if len(body) > 0 {
		hs.ProcessIncoming(body)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (hs *HTTPSServerInterface) handlePoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	peerID, err := hs.peerIDFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	q := hs.ensurePeer(peerID)

	hs.Mutex.RLock()
	done := hs.done
	longPoll := hs.longPoll
	hs.Mutex.RUnlock()

	timer := time.NewTimer(longPoll)
	defer timer.Stop()

	select {
	case pkt := <-q.ch:
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pkt)
	case <-timer.C:
		w.WriteHeader(http.StatusNoContent)
	case <-done:
		http.Error(w, "shutting down", http.StatusServiceUnavailable)
	case <-r.Context().Done():
		return
	}
}

// Stop closes the HTTPS server and peer queues.
func (hs *HTTPSServerInterface) Stop() error {
	hs.Mutex.Lock()
	hs.Online = false
	srv := hs.httpServer
	hs.httpServer = nil
	hs.listener = nil
	hs.peers = make(map[string]*httpsPeerQueue)
	hs.Mutex.Unlock()

	if srv != nil {
		_ = srv.Close()
	}
	hs.stopOnce.Do(func() {
		if hs.done != nil {
			close(hs.done)
		}
	})
	hs.serveWg.Wait()
	return nil
}

// ProcessOutgoing fans out a packet to all peer long-poll queues.
func (hs *HTTPSServerInterface) ProcessOutgoing(data []byte) error {
	hs.Mutex.RLock()
	online := hs.Online
	hs.Mutex.RUnlock()
	if !online {
		return fmt.Errorf("interface offline")
	}
	if len(data) == 0 {
		return nil
	}

	hs.Mutex.Lock()
	queues := make([]*httpsPeerQueue, 0, len(hs.peers))
	for _, q := range hs.peers {
		queues = append(queues, q)
	}
	hs.Mutex.Unlock()
	if len(queues) == 0 {
		return fmt.Errorf("no HTTPS peers")
	}

	for _, q := range queues {
		q.enqueue(append([]byte(nil), data...))
	}
	return nil
}

// Send applies IFAC then ProcessOutgoing.
func (hs *HTTPSServerInterface) Send(data []byte, address string) error {
	_ = address
	if err := common.RejectReceiveOnly(hs); err != nil {
		return err
	}
	masked, err := common.ApplyIFACOutbound(hs, data)
	if err != nil {
		return err
	}
	if err := hs.ProcessOutgoing(masked); err != nil {
		return err
	}
	hs.updateBandwidthStats(uint64(len(masked)))
	return nil
}

// ListenAddr returns the bound address after Start (for tests).
func (hs *HTTPSServerInterface) ListenAddr() net.Addr {
	hs.Mutex.RLock()
	defer hs.Mutex.RUnlock()
	if hs.listener == nil {
		return nil
	}
	return hs.listener.Addr()
}
