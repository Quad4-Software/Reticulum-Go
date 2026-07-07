// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/transport"
)

// Lifecycle coordinates network sleep/wake recovery for the control API.
type Lifecycle interface {
	OnNetworkAvailable() error
	OnNetworkLost() error
	RefreshPaths(dests ...[]byte) error
}

// Server is a localhost JSON control API bound to one Reticulum-Go
// transport. See the package doc comment for the wire protocol.
type Server struct {
	transport *transport.Transport
	lifecycle Lifecycle
	host      string
	port      int
	authKey   []byte
	startedAt time.Time

	httpServer *http.Server

	mu       sync.RWMutex
	sessions map[string]*session

	announceMu   sync.RWMutex
	announceSubs map[*wsClient]struct{}
}

// New builds a control API server bound to t, using cfg.RPCKey as the
// bearer auth token and cfg.ControlAPIHost/Port for the listen address.
// Callers are expected to only call New when cfg.EnableControlAPI is true;
// common.ReticulumConfig.Validate rejects that combination when RPCKey is
// empty.
func New(t *transport.Transport, lifecycle Lifecycle, cfg *common.ReticulumConfig) (*Server, error) {
	if t == nil {
		return nil, errors.New("controlapi: transport is required")
	}
	if cfg == nil || len(cfg.RPCKey) == 0 {
		return nil, errors.New("controlapi: rpc_key must be set")
	}

	host := cfg.ControlAPIHost
	if host == "" {
		host = common.DefaultControlAPIHost
	}
	port := cfg.ControlAPIPort
	if port == 0 {
		port = common.DefaultControlAPIPort
	}

	s := &Server{
		transport:    t,
		lifecycle:    lifecycle,
		host:         host,
		port:         port,
		authKey:      cfg.RPCKey,
		startedAt:    time.Now(),
		sessions:     make(map[string]*session),
		announceSubs: make(map[*wsClient]struct{}),
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)
	s.httpServer = &http.Server{
		Handler:           s.authMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	t.RegisterAnnounceHandler(&announceBridge{server: s})

	return s, nil
}

// Serve binds the configured listen address and blocks, serving requests
// until Close is called. Run it in its own goroutine.
func (s *Server) Serve() error {
	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("controlapi: listen on %s: %w", addr, err)
	}
	debug.Log(debug.DebugInfo, "Control API listening", "addr", addr)

	err = s.httpServer.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Close disconnects every session's WebSocket clients and shuts down the
// HTTP listener.
func (s *Server) Close() error {
	s.mu.Lock()
	sessions := make([]*session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.sessions = make(map[string]*session)
	s.mu.Unlock()

	for _, sess := range sessions {
		sess.close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("GET /v1/status", s.handleStatus)
	mux.HandleFunc("GET /v1/paths", s.handlePaths)
	mux.HandleFunc("POST /v1/sessions", s.handleCreateSession)
	mux.HandleFunc("DELETE /v1/sessions/{id}", s.handleDeleteSession)
	mux.HandleFunc("POST /v1/sessions/{id}/destinations", s.handleRegisterDestination)
	mux.HandleFunc("POST /v1/sessions/{id}/destinations/{hash}/announce", s.handleAnnounce)
	mux.HandleFunc("POST /v1/sessions/{id}/destinations/{hash}/requests", s.handleRegisterRequestHandler)
	mux.HandleFunc("POST /v1/sessions/{id}/path/request", s.handlePathRequest)
	mux.HandleFunc("GET /v1/sessions/{id}/events", s.handleEvents)
	mux.HandleFunc("POST /v1/lifecycle/resume", s.handleLifecycleResume)
	mux.HandleFunc("POST /v1/lifecycle/pause", s.handleLifecyclePause)
	mux.HandleFunc("POST /v1/lifecycle/refresh-paths", s.handleLifecycleRefreshPaths)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:          "ok",
		TransportID:     hex.EncodeToString(s.transport.TransportIdentityHash()),
		TransportUptime: time.Since(s.startedAt).Seconds(),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	stats := s.transport.GetInterfaceStatsRPC()
	resp := statusResponse{
		TransportID: hex.EncodeToString(stats.TransportID),
		Interfaces:  make([]interfaceStatJSON, 0, len(stats.Interfaces)),
	}
	for _, ifc := range stats.Interfaces {
		resp.Interfaces = append(resp.Interfaces, interfaceStatJSON{
			Name:    ifc.Name,
			Type:    ifc.Type,
			Status:  ifc.Status,
			RXBytes: ifc.RXB,
			TXBytes: ifc.TXB,
			Bitrate: ifc.Bitrate,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handlePaths(w http.ResponseWriter, r *http.Request) {
	entries := s.transport.GetPathTable(nil)
	out := make([]pathTableEntryJSON, 0, len(entries))
	for _, e := range entries {
		out = append(out, pathTableEntryJSON{
			Hash:      hex.EncodeToString(e.Hash),
			Via:       hex.EncodeToString(e.Via),
			Hops:      e.Hops,
			Expires:   e.Expires,
			Interface: e.Interface,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	ident, err := loadOrCreateIdentity(req.IdentityPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("identity: %v", err))
		return
	}

	sessionID, err := randomID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to allocate session id")
		return
	}

	sess := newSession(sessionID, ident)
	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, createSessionResponse{
		SessionID:    sessionID,
		IdentityHash: ident.GetHexHash(),
	})
}

// loadOrCreateIdentity loads the identity at path, creates and persists a
// new one there if path is set but does not exist yet, or generates an
// ephemeral in-memory identity when path is empty.
func loadOrCreateIdentity(path string) (*identity.Identity, error) {
	if path == "" {
		return identity.NewIdentity()
	}
	if _, err := os.Stat(path); err == nil {
		return identity.LoadIdentityFile(path, nil)
	}
	ident, err := identity.NewIdentity()
	if err != nil {
		return nil, err
	}
	if err := ident.ToFile(path); err != nil {
		return nil, err
	}
	return ident, nil
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	sess, ok := s.sessions[id]
	if ok {
		delete(s.sessions, id)
	}
	s.mu.Unlock()

	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	sess.close()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePathRequest(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.session(r.PathValue("id")); !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	var req pathRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	destHash, err := hex.DecodeString(req.DestinationHash)
	if err != nil || len(destHash) != 16 {
		writeError(w, http.StatusBadRequest, "destination_hash must be 16 hex-encoded bytes")
		return
	}

	if err := s.transport.RequestPath(destHash, "", nil, false); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("path request: %v", err))
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") || r.Header.Get("Sec-WebSocket-Key") == "" {
		writeError(w, http.StatusBadRequest, "expected websocket upgrade request")
		return
	}

	conn, err := upgradeWebSocket(w, r)
	if err != nil {
		debug.Log(debug.DebugError, "controlapi: websocket upgrade failed", "error", err)
		return
	}

	client := newWSClient(s, sess, conn)
	client.run()
}

func (s *Server) session(id string) (*session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

func (s *Server) subscribeAnnounces(c *wsClient) {
	s.announceMu.Lock()
	defer s.announceMu.Unlock()
	s.announceSubs[c] = struct{}{}
}

func (s *Server) unsubscribeAnnounces(c *wsClient) {
	s.announceMu.Lock()
	defer s.announceMu.Unlock()
	delete(s.announceSubs, c)
}

func (s *Server) broadcastAnnounce(evt announceEvent) {
	s.announceMu.RLock()
	defer s.announceMu.RUnlock()
	for c := range s.announceSubs {
		c.send(evt)
	}
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
